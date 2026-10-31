package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/api"
	cloudclient "github.com/enowx/enowx/cloud/client"
	"github.com/enowx/enowx/cloud/hostauth"
	"github.com/enowx/enowx/proto"
)

// cloudLink is this host's sign-in to a team cloud (docs/cloud.md): kept
// by hostauth, a sign-in waiting for its code's approval in between. Its
// mutex guards pending, cancel and err; it is never held while asking the
// cloud.
type cloudLink struct {
	store   *hostauth.Store
	mu      sync.Mutex
	pending *proto.CloudPending
	cancel  context.CancelFunc
	err     string
}

// cloudTimeout bounds a request the daemon makes to the cloud.
const cloudTimeout = 20 * time.Second

func (s *Server) cloudStatus() (proto.CloudStatus, error) {
	st, err := s.cloud.store.Get()
	if err != nil {
		return proto.CloudStatus{}, err
	}
	out := proto.CloudStatus{URL: st.URL}
	if st.SignedIn() {
		out.SignedIn = true
		out.AccountID, out.Account, out.GitHub, out.Avatar = st.AccountID, st.Account, st.GitHub, st.Avatar
		out.DeviceID, out.DeviceName, out.DeviceKind = st.DeviceID, st.DeviceName, st.DeviceKind
	}
	s.cloud.mu.Lock()
	out.Pending, out.Err = s.cloud.pending, s.cloud.err
	s.cloud.mu.Unlock()
	return out, nil
}

func (s *Server) cloudChanged() {
	if st, err := s.cloudStatus(); err == nil {
		s.broadcast(proto.EventCloudChanged, st)
	}
	s.spacesChanged() // signed in or out: the account's teams' spaces
}

// cloudSetURL sets the cloud's address, checking it is one; another
// address signs out of the one before. "" turns the cloud off.
func (s *Server) cloudSetURL(raw string) (proto.CloudStatus, error) {
	raw = strings.TrimSpace(raw)
	st, err := s.cloud.store.Get()
	if err != nil {
		return proto.CloudStatus{}, err
	}
	if raw != "" {
		c, err := cloudclient.New(raw, "")
		if err != nil {
			return proto.CloudStatus{}, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), cloudTimeout)
		defer cancel()
		if _, err := c.Info(ctx); err != nil {
			return proto.CloudStatus{}, fmt.Errorf("cloud: %s doesn't answer as an enowX cloud: %w", c.URL, err)
		}
		raw = c.URL
	}
	if raw != st.URL {
		s.cloudCancel()
		if st.SignedIn() {
			s.cloudRevoke(st)
		}
		st = hostauth.State{URL: raw}
		if err := s.cloud.store.Put(st); err != nil {
			return proto.CloudStatus{}, err
		}
		s.cloudChanged()
	}
	return s.cloudStatus()
}

// cloudSignIn starts signing the host in: a code to approve, and a wait for
// its approval in the background, which ends signed in, or with an error.
func (s *Server) cloudSignIn(p proto.CloudSignInParams) (proto.CloudStatus, error) {
	st, err := s.cloud.store.Get()
	if err != nil {
		return proto.CloudStatus{}, err
	}
	if st.URL == "" {
		// Signing in turns the cloud on: enowX's own, unless one was set.
		on, err := s.cloudSetURL(api.DefaultURL)
		if err != nil {
			return proto.CloudStatus{}, err
		}
		st.URL = on.URL
	}
	kind := p.Kind
	if kind == "" {
		kind = proto.CloudDeviceHost
	}
	if kind != proto.CloudDeviceApp && kind != proto.CloudDeviceHost {
		return proto.CloudStatus{}, fmt.Errorf("cloud: sign in as %s or %s, not %q", proto.CloudDeviceApp, proto.CloudDeviceHost, kind)
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name, _ = os.Hostname()
	}
	key, err := s.cloud.store.DeviceKey()
	if err != nil {
		return proto.CloudStatus{}, err
	}
	c, err := cloudclient.New(st.URL, "")
	if err != nil {
		return proto.CloudStatus{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cloudTimeout)
	dc, err := c.StartSignIn(ctx, api.DeviceCodeParams{Name: name, Kind: kind, PublicKey: key.Public()})
	cancel()
	if err != nil {
		return proto.CloudStatus{}, err
	}
	s.cloudCancel()
	wait, stop := context.WithDeadline(context.Background(), dc.Expires)
	s.cloud.mu.Lock()
	s.cloud.pending = &proto.CloudPending{UserCode: dc.UserCode, VerifyURL: dc.VerifyURL, VerifyURLComplete: dc.VerifyURLComplete, Expires: dc.Expires}
	s.cloud.cancel, s.cloud.err = stop, ""
	s.cloud.mu.Unlock()
	go s.cloudWait(wait, c, dc, st.URL)
	s.cloudChanged()
	return s.cloudStatus()
}

// cloudWait waits for dc's approval and keeps what it signs in as. It runs
// on a goroutine of its own, one at a time (cloudSignIn cancels the last).
func (s *Server) cloudWait(ctx context.Context, c *cloudclient.Client, dc api.DeviceCode, url string) {
	in, err := c.WaitSignIn(ctx, dc)
	if errors.Is(err, context.Canceled) {
		return // cancelled, or another sign-in took its place
	}
	if errors.Is(err, context.DeadlineExceeded) {
		err = errors.New("cloud: the code expired before it was approved: sign in again")
	}
	if err == nil {
		err = s.cloud.store.Put(hostauth.State{URL: url, Token: in.Token,
			AccountID: in.Me.Account.ID, Account: in.Me.Account.Name, GitHub: in.Me.Account.GitHub, Avatar: in.Me.Account.Avatar,
			DeviceID: in.Me.Device.ID, DeviceName: in.Me.Device.Name, DeviceKind: in.Me.Device.Kind})
	}
	s.cloud.mu.Lock()
	s.cloud.pending, s.cloud.cancel = nil, nil
	if err != nil {
		s.cloud.err = err.Error()
	}
	s.cloud.mu.Unlock()
	s.cloudChanged()
}

// cloudCancel drops a sign-in waiting for approval.
func (s *Server) cloudCancel() {
	s.cloud.mu.Lock()
	cancel := s.cloud.cancel
	s.cloud.pending, s.cloud.cancel, s.cloud.err = nil, nil, ""
	s.cloud.mu.Unlock()
	if cancel != nil {
		cancel()
		s.cloudChanged()
	}
}

// cloudSignOut signs the host out: its token stops working on the cloud
// (when it answers) and is forgotten here.
func (s *Server) cloudSignOut() (proto.CloudStatus, error) {
	s.cloudCancel()
	st, err := s.cloud.store.Get()
	if err != nil {
		return proto.CloudStatus{}, err
	}
	if st.SignedIn() {
		s.cloudRevoke(st)
		if err := s.cloud.store.Put(hostauth.State{URL: st.URL}); err != nil {
			return proto.CloudStatus{}, err
		}
		s.cloudChanged()
	}
	return s.cloudStatus()
}

// cloudRevoke asks the cloud to end st's token, best effort: forgetting it
// here signs out regardless.
func (s *Server) cloudRevoke(st hostauth.State) {
	c, err := cloudclient.New(st.URL, st.Token)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cloudTimeout)
	defer cancel()
	if err := c.SignOut(ctx); err != nil {
		s.logger().Warn("cloud: sign out", "err", err)
	}
}

// cloudRequest asks the cloud's API as the host's device.
func (s *Server) cloudRequest(p proto.CloudRequest) (proto.CloudResponse, error) {
	st, err := s.cloud.store.Get()
	if err != nil {
		return proto.CloudResponse{}, err
	}
	if st.URL == "" {
		return proto.CloudResponse{}, errors.New("cloud: no cloud is set up on this host")
	}
	c, err := cloudclient.New(st.URL, st.Token)
	if err != nil {
		return proto.CloudResponse{}, err
	}
	method := strings.ToUpper(p.Method)
	if method == "" {
		method = "GET"
	}
	ctx, cancel := context.WithTimeout(context.Background(), cloudTimeout)
	defer cancel()
	status, body, err := c.Raw(ctx, method, p.Path, p.Body)
	if err != nil {
		return proto.CloudResponse{}, err
	}
	if status == 401 && st.SignedIn() {
		// The token was revoked elsewhere (another device signed it out):
		// this host is signed out too.
		s.cloud.store.Put(hostauth.State{URL: st.URL})
		s.cloudChanged()
	}
	if status/100 == 2 && st.SignedIn() {
		s.learnFrom(st.AccountID, method, p.Path, body)
	}
	out := proto.CloudResponse{Status: status}
	if len(body) > 0 && (body[0] == '{' || body[0] == '[') {
		out.Body = body
	}
	return out, nil
}

// learnFrom keeps the teams an answer of the cloud names, made or listed,
// as spaces of the host's account, and drops the space of one deleted.
func (s *Server) learnFrom(account, method, path string, body []byte) {
	if method == "DELETE" {
		if team := deletedTeamPath(path); team != "" {
			s.dropDeletedHere(team)
		}
		return
	}
	if path != api.PathTeams {
		return
	}
	var teams []api.Team
	switch method {
	case "GET":
		if json.Unmarshal(body, &teams) != nil {
			return
		}
	case "POST":
		var t api.Team
		if json.Unmarshal(body, &t) != nil {
			return
		}
		teams = []api.Team{t}
	default:
		return
	}
	s.learnTeams(account, teams)
}
