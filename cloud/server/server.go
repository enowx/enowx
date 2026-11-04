// Package server is the cloud's HTTP server: its API (cloud/api) over the
// database (cloud/store). It speaks plain HTTP behind a proxy that ends TLS.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/blobstore"
	"github.com/enowx/enowx/cloud/library"
	"github.com/enowx/enowx/cloud/store"
	"github.com/enowx/enowx/version"
)

// LibraryConfig is how enowX UI is set up.
type LibraryConfig = library.Config

// Config is how a server is set up (cmd/enowx-cloud reads it from the
// environment).
type Config struct {
	// URL is the address members reach it at.
	URL string
	// GitHubClientID and GitHubClientSecret are the OAuth app members sign
	// in with; empty, GitHub sign-in is off.
	GitHubClientID, GitHubClientSecret string
	// SMTPURL sends invites by email; empty, invites are links only.
	SMTPURL string
	// GitHubURL and GitHubAPIURL are GitHub's, github.com's when empty
	// (tests give their own).
	GitHubURL, GitHubAPIURL string
	// DefaultTeam is the team everyone joins as they sign in, with
	// DefaultRole (a viewer when empty); empty, there's none.
	DefaultTeam, DefaultRole string
	// DefaultTeamRepo is a GitHub repository, owner/name, whose
	// contributors join the default team as members (contributors.go).
	DefaultTeamRepo string
	// Blobs keeps teams' files (a chat's images); nil, none are kept.
	Blobs blobstore.Store
	// Contributor is the Contributor role on Discord; empty, it's off.
	Contributor ContributorConfig
	// Library is enowX UI's setup; nil or without its bucket, there's none.
	Library *library.Config
}

// Server answers the cloud's API. Its handlers run on net/http's
// goroutines; the database is safe for that.
type Server struct {
	cfg Config
	db  *store.DB
	log *slog.Logger
	mux *http.ServeMux
	// recovery are the open recovery challenges (recoverychallenge.go).
	recovery challenges
	// limits counts requests to the doors open without a token (limits.go).
	limits limiter
	// contribs is the default team's repository's contributors.
	contribs contributors
	// present is who of each team is here now (presence.go).
	present presences
	// contributor is the Contributor role on Discord, nil when off (contributorrole.go).
	contributor *contributorRole
	// library is enowX UI, nil when it's off.
	library *library.Service
}

// New is a server over db.
func New(cfg Config, db *store.DB, log *slog.Logger) *Server {
	s := &Server{cfg: cfg, db: db, log: log, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET "+api.PathHealth, s.health)
	s.mux.HandleFunc("GET "+api.PathInfo, s.info)
	s.mux.HandleFunc("GET "+api.PathMe, s.signedIn(s.me))
	s.mux.HandleFunc("GET "+api.PathDevices, s.signedIn(s.devices))
	s.mux.HandleFunc("DELETE "+api.PathDevice, s.signedIn(s.removeDevice))
	s.mux.HandleFunc("POST "+api.PathSignOut, s.signedIn(s.signOut))
	s.mux.HandleFunc("POST "+api.PathDeviceCode, s.deviceCode)
	s.mux.HandleFunc("POST "+api.PathDeviceToken, s.deviceToken)
	s.mux.HandleFunc("GET "+api.PathDevicePending, s.signedIn(s.pendingDevice))
	s.mux.HandleFunc("POST "+api.PathDeviceApprove, s.signedIn(s.approveDevice))
	s.mux.HandleFunc("POST "+api.PathTeams, s.signedIn(s.createTeam))
	s.mux.HandleFunc("GET "+api.PathTeams, s.signedIn(s.teams))
	s.mux.HandleFunc("GET "+api.PathTeam, s.signedIn(s.team))
	s.mux.HandleFunc("PATCH "+api.PathTeam, s.signedIn(s.updateTeam))
	s.mux.HandleFunc("DELETE "+api.PathTeam, s.signedIn(s.deleteTeam))
	s.mux.HandleFunc("PATCH "+api.PathMember, s.signedIn(s.setRole))
	s.mux.HandleFunc("DELETE "+api.PathMember, s.signedIn(s.removeMember))
	s.mux.HandleFunc("POST "+api.PathInvites, s.signedIn(s.createInvite))
	s.mux.HandleFunc("GET "+api.PathInvites, s.signedIn(s.invites))
	s.mux.HandleFunc("DELETE "+api.PathInvite, s.signedIn(s.deleteInvite))
	s.mux.HandleFunc("POST "+api.PathJoin, s.signedIn(s.join))
	s.mux.HandleFunc("POST "+api.PathInvitePeek, s.peekInvite)
	s.mux.HandleFunc("GET "+api.PathTeamKeys, s.signedIn(s.teamKeys))
	s.mux.HandleFunc("PUT "+api.PathTeamKey, s.signedIn(s.shareKey))
	s.mux.HandleFunc("DELETE "+api.PathTeamKey, s.signedIn(s.dropKey))
	s.mux.HandleFunc("GET "+api.PathTeamDevices, s.signedIn(s.teamDevices))
	s.mux.HandleFunc("POST "+api.PathTeamPresence, s.signedIn(s.presence))
	s.mux.HandleFunc("PUT "+api.PathTeamBlob, s.signedIn(s.putBlob))
	s.mux.HandleFunc("GET "+api.PathTeamBlob, s.signedIn(s.getBlob))
	s.mux.HandleFunc("DELETE "+api.PathTeamBlob, s.signedIn(s.deleteBlob))
	s.mux.HandleFunc("GET "+api.PathItems, s.signedIn(s.items))
	s.mux.HandleFunc("PUT "+api.PathItem, s.signedIn(s.putItem))
	s.mux.HandleFunc("POST "+api.PathAudit, s.signedIn(s.recordAudit))
	s.mux.HandleFunc("GET "+api.PathAudit, s.signedIn(s.audit))
	s.mux.HandleFunc("POST "+api.PathHostTickets, s.signedIn(s.issueTicket))
	s.mux.HandleFunc("POST "+api.PathTicketRedeem, s.signedIn(s.redeemTicket))
	s.mux.HandleFunc("GET "+api.PathRecoveryChallenge, s.signedIn(s.recoveryChallenge))
	s.mux.HandleFunc("POST "+api.PathRecovery, s.signedIn(s.setRecovery))
	s.mux.HandleFunc("GET "+api.PathTeamRecovery, s.signedIn(s.recoveryKeys))
	s.mux.HandleFunc("POST "+api.PathTeamRecovery, s.signedIn(s.recover))
	s.mux.HandleFunc("GET "+pathGitHub, s.githubStart)
	s.mux.HandleFunc("GET "+pathGitHubCallback, s.githubCallback)
	s.mux.HandleFunc("GET /{$}", s.home)
	s.mux.HandleFunc("GET /logo.svg", s.logo)
	s.mux.HandleFunc("GET /device", s.device)
	s.mux.HandleFunc("GET /invite", s.invitePage)
	s.mux.HandleFunc("POST /invite", s.inviteOpen)
	s.mux.HandleFunc("POST /invite/accept", s.inviteAccept)
	s.mux.HandleFunc("POST /device", s.approve)
	s.mux.HandleFunc("POST /signout", s.webSignOut)
	if cfg.Library != nil && cfg.Library.Blobs != nil {
		s.library = library.New(*cfg.Library, library.Host{WebAccount: s.webAccount, SignInURL: s.signInURL, ClientAddr: clientAddr}, db, log)
		s.library.Mount(s.mux)
	}
	s.contributorRoutes()
	return s
}

// ServeHTTP serves the API, each request logged.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	if s.guard(rw, r) {
		s.mux.ServeHTTP(rw, r)
	}
	if r.URL.Path != api.PathHealth {
		s.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rw.status, "ms", time.Since(start).Milliseconds())
	}
}

// health is up when the database answers.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		s.log.Error("health", "err", err)
		s.fail(w, http.StatusServiceUnavailable, "the database doesn't answer")
		return
	}
	w.Write([]byte("ok\n"))
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	in := api.Info{Name: "enowx-cloud", Version: version.String, API: api.Version, URL: s.cfg.URL, SignIn: []string{}}
	if s.githubOn() {
		in.SignIn = append(in.SignIn, api.SignInGitHub)
	}
	if s.library != nil {
		in.Features = append(in.Features, api.FeatureLibrary)
		in.LibraryURL = s.cfg.Library.PublicURL
	}
	s.reply(w, in)
}

// RunLibrary sweeps the library's objects until ctx ends; nothing when it's off.
func (s *Server) RunLibrary(ctx context.Context) {
	if s.library != nil {
		s.library.Sweep(ctx)
	}
}

func (s *Server) reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Warn("reply", "err", err)
	}
}

// internal answers 500 for an error of the server's own, logged, not
// shown: a database's message says too much.
func (s *Server) internal(w http.ResponseWriter, err error) {
	s.log.Error("internal", "err", err)
	s.fail(w, http.StatusInternalServerError, "something went wrong on the server")
}

// pageInternal is internal for the cloud's pages.
func (s *Server) pageInternal(w http.ResponseWriter, err error) {
	s.log.Error("internal", "err", err)
	s.page(w, http.StatusInternalServerError, "Something went wrong on the server: try again.", nil)
}

func (s *Server) fail(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(api.Error{Error: msg})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
