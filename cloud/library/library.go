// Package library is enowX UI's server: the community library of canvas designs
// (docs/cloud.md), under /v1/library. The website at ui.enowx.ai only shows what
// this answers: every decision on who may see or do what is made here.
package library

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/blobstore"
	"github.com/enowx/enowx/cloud/store"
)

// Config is how the library is set up (cmd/enowx-cloud reads it from the
// environment). Without Blobs there is no library.
type Config struct {
	// Blobs keeps designs: a bucket of its own, apart from teams' files.
	Blobs blobstore.Store
	// PublicURL is the website's address, for credits.
	PublicURL string
	// ReturnURL is where a sign-in sends its code. It comes from here alone,
	// never from a request.
	ReturnURL string
	// WebSecret is what the website proves itself with.
	WebSecret string
	// AdminTeam is the team whose owner and admins moderate.
	AdminTeam string
}

// Host is what the library takes from the server around it.
type Host struct {
	// WebAccount is who the browser is signed in to the cloud's pages as.
	WebAccount func(*http.Request) *api.Account
	// SignInURL starts signing in with GitHub, back to next; "" when off.
	SignInURL func(next string) string
	// ClientAddr is where a request comes from.
	ClientAddr func(*http.Request) string
}

// Service answers the library's API. Its handlers run on net/http's goroutines;
// what they share is the database and the limiter under its mutex.
type Service struct {
	cfg  Config
	host Host
	db   *store.DB
	log  *slog.Logger
	// reads holds a place per public read at work, so that a burst of them
	// leaves the database's connections to the teams' sync.
	reads  chan struct{}
	limits perAddress
}

// publicReads is the most public reads at work at once.
const publicReads = 4

// New is a library over db.
func New(cfg Config, host Host, db *store.DB, log *slog.Logger) *Service {
	return &Service{cfg: cfg, host: host, db: db, log: log, reads: make(chan struct{}, publicReads)}
}

// Mount adds the library's routes to mux.
func (s *Service) Mount(mux *http.ServeMux) {
	handle := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, h) }
	handle("GET "+api.PathLibraryItems, s.public(s.listItems))
	handle("GET "+api.PathLibraryItem, s.public(s.getItem))
	handle("GET "+api.PathLibraryVersions, s.public(s.listVersions))
	handle("GET "+api.PathLibraryPrompt, s.public(s.getPrompt))
	handle("POST "+api.PathLibraryEscaped, s.webOnly(s.escaped))

	handle("POST "+api.PathLibraryItems, s.deviceOnly(s.createItem))
	handle("PUT "+api.PathLibraryPart, s.deviceOnly(s.putPart))
	handle("POST "+api.PathLibrarySubmit, s.deviceOnly(s.submit))
	handle("GET "+api.PathLibraryMine, s.deviceOnly(s.mine))

	handle("DELETE "+api.PathLibraryItem, s.signedIn(s.withdraw))
	handle("GET "+api.PathLibraryMe, s.signedIn(s.me))
	handle("POST "+api.PathLibraryReports, s.signedIn(s.report))

	handle("GET "+api.PathLibraryQueue, s.moderator(s.queue))
	handle("POST "+api.PathLibraryItemDecide, s.moderator(s.decideItem))
	handle("GET "+api.PathLibraryAdminPart, s.moderator(s.adminPart))
	handle("POST "+api.PathLibraryRepDecide, s.moderator(s.decideReport))
	handle("POST "+api.PathLibraryBlock, s.moderator(s.block))
	handle("POST "+api.PathLibraryUnblock, s.moderator(s.unblock))
	handle("GET "+api.PathLibraryLog, s.moderator(s.readLog))
	handle("GET "+api.PathLibraryUsage, s.moderator(s.usage))

	handle("GET "+api.PathLibraryAccounts, s.owner(s.accounts))
	handle("GET "+api.PathLibraryModerators, s.owner(s.moderators))
	handle("POST "+api.PathLibraryModerators, s.owner(s.appoint))
	handle("DELETE "+api.PathLibraryModerator, s.owner(s.dismiss))

	handle("GET "+api.PathLibraryAuthorize, s.authorize)
	handle("POST "+api.PathLibraryExchange, s.webOnly(s.exchange))
	handle("POST "+api.PathLibrarySignOut, s.signOut)
}

// Sweep goes until ctx ends: objects past their time go, and a bucket nearing
// its limits is said in the log.
func (s *Service) Sweep(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		s.SweepOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// SweepOnce is one go of Sweep.
func (s *Service) SweepOnce(ctx context.Context) {
	due, err := s.db.LibraryPurgeDue(ctx)
	if err != nil {
		s.log.Warn("library: purge", "err", err)
	}
	for _, p := range due {
		failed := false
		for _, part := range []string{api.PartSource, api.PartDoc, api.PartDocPhone, api.PartThumb, api.PartThumbPhone} {
			if err := s.cfg.Blobs.Delete(ctx, objectKey(p.ItemID, p.Version, part)); err != nil && !errors.Is(err, blobstore.ErrNotFound) {
				s.log.Warn("library: purge", "item", p.ItemID, "err", err)
				failed = true
			}
		}
		if failed {
			continue
		}
		if err := s.db.ForgetLibraryVersion(ctx, p); err != nil {
			s.log.Warn("library: purge", "item", p.ItemID, "err", err)
		}
	}
	if u, err := s.db.LibraryUsage(ctx); err == nil && u.Warn {
		s.log.Warn("library: the bucket is past 70% of a limit", "stored", u.BytesStored, "written", u.BytesWritten)
	}
}

func objectKey(id string, version int, part string) string {
	return "items/" + id + "/v" + strconv.Itoa(version) + "/" + part
}

func gzipped(data []byte) ([]byte, error) {
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func gunzipped(data []byte, limit int64) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(r, limit))
}

// Answers.

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(api.Error{Error: msg})
}

// answer says how a store's error reads: a message of its own, or, for the
// database's, nothing of it.
func (s *Service) answer(w http.ResponseWriter, v any, err error) {
	switch {
	case err == nil && v == nil:
		w.WriteHeader(http.StatusNoContent)
	case err == nil:
		reply(w, v)
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrLibraryQuota):
		fail(w, http.StatusInsufficientStorage, "your library storage is full: 200 MB. Unpublish something first")
	case errors.Unwrap(err) == nil && strings.HasPrefix(err.Error(), "store: "):
		fail(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "store: "))
	default:
		s.log.Error("library", "err", err)
		fail(w, http.StatusInternalServerError, "something went wrong on the server")
	}
}

// readJSON reads a request's body of at most 64 KiB, or answers.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "this request doesn't read")
		return false
	}
	return true
}

// Who asks.

// fromWeb says the request carries the website's secret.
func (s *Service) fromWeb(r *http.Request) bool {
	got := r.Header.Get(api.HeaderLibraryWeb)
	return s.cfg.WebSecret != "" && got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.WebSecret)) == 1
}

type askerKey struct{}

// asking is who a request is from: a device's token or a web session's.
type asking struct {
	account api.Account
	web     bool
	role    string
}

func asker(r *http.Request) asking { return r.Context().Value(askerKey{}).(asking) }

// whoAsks is who the request's token is, ok false (answered) when it opens
// nothing.
func (s *Service) whoAsks(w http.ResponseWriter, r *http.Request) (asking, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		fail(w, http.StatusUnauthorized, "sign in first")
		return asking{}, false
	}
	var a asking
	var err error
	if store.IsLibraryToken(token) {
		a.web = true
		a.account, err = s.db.LibrarySessionAccount(r.Context(), token)
	} else {
		var me api.Me
		me, err = s.db.Me(r.Context(), token)
		a.account = me.Account
	}
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusUnauthorized, "sign in again")
		return asking{}, false
	}
	if err != nil {
		s.answer(w, nil, err)
		return asking{}, false
	}
	if a.role, err = s.roleOf(r.Context(), a.account.ID); err != nil {
		s.answer(w, nil, err)
		return asking{}, false
	}
	return a, true
}

// roleOf is accID's moderating role: the admin team's owner, one of its admins,
// or a moderator its owner appointed.
func (s *Service) roleOf(ctx context.Context, accID string) (string, error) {
	if s.cfg.AdminTeam != "" {
		role, err := s.db.Role(ctx, s.cfg.AdminTeam, accID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return "", fmt.Errorf("library: role: %w", err)
		}
		switch role {
		case api.RoleOwner:
			return api.RoleLibraryOwner, nil
		case api.RoleAdmin:
			return api.RoleLibraryAdmin, nil
		}
	}
	is, err := s.db.IsLibraryModerator(ctx, accID)
	if err != nil {
		return "", err
	}
	if is {
		return api.RoleLibraryModerator, nil
	}
	return "", nil
}

func (s *Service) signedIn(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a, ok := s.whoAsks(w, r); ok {
			h(w, r.WithContext(context.WithValue(r.Context(), askerKey{}, a)))
		}
	}
}

// deviceOnly is for what only the app does, confirmed by its user: a browser's
// session can't publish.
func (s *Service) deviceOnly(h http.HandlerFunc) http.HandlerFunc {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request) {
		if asker(r).web {
			fail(w, http.StatusForbidden, "publishing is done from the enowX app")
			return
		}
		h(w, r)
	})
}

func (s *Service) moderator(h http.HandlerFunc) http.HandlerFunc {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request) {
		if asker(r).role == "" {
			fail(w, http.StatusForbidden, "this is for moderators")
			return
		}
		h(w, r)
	})
}

func (s *Service) owner(h http.HandlerFunc) http.HandlerFunc {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request) {
		if asker(r).role != api.RoleLibraryOwner {
			fail(w, http.StatusForbidden, "this is for the owner")
			return
		}
		h(w, r)
	})
}

func (s *Service) webOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.fromWeb(r) {
			fail(w, http.StatusForbidden, "this is for the library's website")
			return
		}
		h(w, r)
	}
}

// publicPerMinute is how many reads an address may make; the website's own
// requests aren't counted as one address.
const publicPerMinute = 240

// public serves anyone, within the address's limit and the reads at work.
func (s *Service) public(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.fromWeb(r) && !s.limits.allow(s.host.ClientAddr(r), publicPerMinute) {
			w.Header().Set("Retry-After", "60")
			fail(w, http.StatusTooManyRequests, "too many requests from here: wait a minute")
			return
		}
		select {
		case s.reads <- struct{}{}:
			defer func() { <-s.reads }()
		case <-time.After(2 * time.Second):
			w.Header().Set("Retry-After", "5")
			fail(w, http.StatusServiceUnavailable, "the library is busy: try again")
			return
		case <-r.Context().Done():
			return
		}
		h(w, r)
	}
}

// perAddress counts requests per address, a minute at a time, under mu.
type perAddress struct {
	mu     sync.Mutex
	window time.Time
	counts map[string]int
}

func (l *perAddress) allow(addr string, limit int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.counts == nil || now.Sub(l.window) > time.Minute {
		l.window, l.counts = now, map[string]int{}
	}
	l.counts[addr]++
	return l.counts[addr] <= limit
}

// actionsOf says what an item allows anyone, by its license and kind: copies of
// a view-only item aren't made, and a full page's code isn't copied.
func actionsOf(it api.LibraryItem) api.LibraryActions {
	if it.License == api.LicenseViewOnly {
		return api.LibraryActions{}
	}
	return api.LibraryActions{
		CopyPrompt:   true,
		UseComponent: it.Kind == api.LibraryComponent,
		UseReference: it.Kind == api.LibraryPage,
		OpenInEnowX:  true,
	}
}

func shown(it api.LibraryItem) api.LibraryItem {
	it.Actions = actionsOf(it)
	return it
}

func oneOf(list []string, v string) bool { return slices.Contains(list, v) }
