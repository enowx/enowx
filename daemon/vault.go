package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
)

// The vault (docs/vault.md): the host keeps its ciphertext and index in
// <daemon dir>/vault, and its record of uses; credentials themselves come
// from the app holding the key, asked each time, and are kept in memory
// for the session they were lent to, until it goes.

// vaultTimeout bounds a request: the user may have to unlock the vault
// and answer whether this host may have a credential.
const vaultTimeout = 2 * time.Minute

// errNoVaultApp is a request's error with no app to answer it.
var errNoVaultApp = errors.New("vault: no enowX app is connected to this host to lend credentials: open the app")

// vaultState is the apps answering requests, the requests waiting and
// the credentials lent to sessions. Safe from any goroutine.
type vaultState struct {
	mu      sync.Mutex
	serving []*conn
	pending map[string]chan proto.VaultReply
	leases  map[string]*vaultLease // by session
	audit   *vault.Audit
	// keyWait are the sessions to continue once an app serves (agentkey.go).
	keyWait map[string]bool
}

// vaultLease is what a session was lent: its variables, the folder of its
// files and helpers, its SSH agent.
type vaultLease struct {
	names   []string
	env     map[string]string
	secrets []string
	dir     string
	agent   func() // stops its SSH agent; nil for none

	// What the session's agent alone runs on (Preset.VaultPlatform). It isn't in env: a
	// session's environment also reaches its wrapper shells, readable in /proc for as long
	// as they live, the login shell after the agent, and its tabs. The agent takes it over
	// the daemon's socket instead, once per start (agentArmed).
	agentVars  map[string]bool // the variables that are the agent's alone
	agentCreds map[string]bool // the credentials those are of
	agentEnv   map[string]string
	agentArmed bool
	// agentID is the agent the lease was made for: another agent's key is laid out
	// differently, so a lease isn't used for one (agentkey.go).
	agentID string
}

func (s *Server) vaultDir() string { return filepath.Join(filepath.Dir(s.WorktreeRoot), "vault") }
func (s *Server) vaultRunDir() string {
	return filepath.Join(filepath.Dir(s.WorktreeRoot), "vault-run")
}

func (v *vaultState) serve(c *conn) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.serving = append(slices.DeleteFunc(v.serving, func(o *conn) bool { return o == c }), c)
}

func (v *vaultState) drop(c *conn) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.serving = slices.DeleteFunc(v.serving, func(o *conn) bool { return o == c })
}

func (v *vaultState) served() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.serving) > 0
}

func (v *vaultState) reply(r proto.VaultReply) {
	v.mu.Lock()
	ch := v.pending[r.ID]
	delete(v.pending, r.ID)
	v.mu.Unlock()
	if ch != nil {
		ch <- r
	}
}

// vaultIndex is the host's index, Present false with no vault.
func (s *Server) vaultIndex() proto.VaultIndex {
	out := proto.VaultIndex{Credentials: []proto.CredentialInfo{}}
	data, err := os.ReadFile(filepath.Join(s.vaultDir(), "index.json"))
	if err != nil || json.Unmarshal(data, &out) != nil {
		return proto.VaultIndex{Credentials: []proto.CredentialInfo{}}
	}
	if out.Credentials == nil {
		out.Credentials = []proto.CredentialInfo{}
	}
	return out
}

// vaultGet is the host's copy.
func (s *Server) vaultGet() (proto.VaultCopy, error) {
	data, err := os.ReadFile(filepath.Join(s.vaultDir(), "vault.json"))
	if errors.Is(err, os.ErrNotExist) {
		return proto.VaultCopy{Index: s.vaultIndex()}, nil
	}
	return proto.VaultCopy{Data: data, Index: s.vaultIndex()}, err
}

// vaultPut replaces the host's copy, ciphertext only: data must read as
// a sealed vault.
func (s *Server) vaultPut(p proto.VaultCopy) (proto.VaultIndex, error) {
	if _, err := vault.Parse(p.Data); err != nil {
		return proto.VaultIndex{}, err
	}
	if err := os.MkdirAll(s.vaultDir(), 0o700); err != nil {
		return proto.VaultIndex{}, err
	}
	p.Index.Present = true
	index, _ := json.MarshalIndent(p.Index, "", "  ")
	if err := writeFileAtomic(filepath.Join(s.vaultDir(), "vault.json"), p.Data); err != nil {
		return proto.VaultIndex{}, err
	}
	if err := writeFileAtomic(filepath.Join(s.vaultDir(), "index.json"), index); err != nil {
		return proto.VaultIndex{}, err
	}
	s.broadcast(proto.EventVaultChanged, p.Index)
	return p.Index, nil
}

// vaultAsk hands a request to the latest app serving the vault and waits
// for its answer, recording the use.
func (s *Server) vaultAsk(a proto.VaultAsk) (proto.VaultLend, error) {
	req := proto.VaultRequest{VaultAsk: a}
	req.Hostname, _ = os.Hostname()
	if sess, err := s.sessions.Get(a.Session); err == nil {
		meta := sess.Meta()
		req.SessionName, req.Agent, req.Project = meta.Name, meta.Agent, meta.Project
		if req.SessionName == "" {
			req.SessionName = sess.Info().Name
		}
	}
	lend, err := s.vaultRequest(req)
	s.vaultAudit(a, lend, err)
	return lend, err
}

func (s *Server) vaultRequest(req proto.VaultRequest) (proto.VaultLend, error) {
	v := &s.vault
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	req.ID = hex.EncodeToString(id)
	ch := make(chan proto.VaultReply, 1)
	v.mu.Lock()
	if len(v.serving) == 0 {
		v.mu.Unlock()
		return proto.VaultLend{}, errNoVaultApp
	}
	app := v.serving[len(v.serving)-1]
	if v.pending == nil {
		v.pending = map[string]chan proto.VaultReply{}
	}
	v.pending[req.ID] = ch
	v.mu.Unlock()
	raw, _ := json.Marshal(req)
	ev, _ := json.Marshal(proto.Event{Name: proto.EventVaultRequest, Data: raw})
	app.send(proto.Frame{Type: proto.TypeEvent, Payload: ev})
	select {
	case r := <-ch:
		if r.Error != "" {
			return r.Lend, errors.New(r.Error)
		}
		return r.Lend, nil
	case <-time.After(vaultTimeout):
		v.mu.Lock()
		delete(v.pending, req.ID)
		v.mu.Unlock()
		return proto.VaultLend{}, errors.New("vault: the app didn't answer in time: is the vault unlocked?")
	case <-s.done:
		return proto.VaultLend{}, errors.New("vault: the daemon is stopping")
	}
}

// vaultAudit records a request: which credential, for what, allowed or
// why not. Never a value.
func (s *Server) vaultAudit(a proto.VaultAsk, lend proto.VaultLend, err error) {
	if s.vault.audit == nil {
		return
	}
	e := proto.VaultAuditEntry{Time: time.Now().UTC(), Action: a.Action, Session: a.Session, Allowed: err == nil}
	names := lend.Credentials
	if len(names) == 0 && a.Name != "" {
		names = []string{a.Name}
	}
	if len(names) > 0 {
		e.Credential = names[0]
		for _, n := range names[1:] {
			e.Credential += ", " + n
		}
	}
	switch {
	case err != nil:
		e.Detail = err.Error()
	case a.Command != "":
		e.Detail = a.Command
	case a.Host != "":
		e.Detail = a.Host
	}
	if err := s.vault.audit.Add(e); err != nil {
		s.logger().Warn("vault: could not record a use", "err", err)
	}
}
