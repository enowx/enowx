package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/library"
)

// Signing in to the cloud's pages with GitHub (OAuth's web flow): the
// browser goes to GitHub, comes back to the callback with a code, the
// server trades it for a token, asks GitHub who it is, once, and keeps a
// session of its own. GitHub's token isn't kept.

const (
	pathGitHub         = "/v1/auth/github"
	pathGitHubCallback = "/v1/auth/github/callback"
	stateCookie        = "enowx_oauth"
	sessionCookie      = "enowx_session"
	githubScopes       = "read:user user:email"
)

var githubClient = &http.Client{Timeout: 15 * time.Second}

// githubOn says GitHub sign-in is set up: the app's ID and secret, and the
// address GitHub sends the browser back to.
func (s *Server) githubOn() bool {
	return s.cfg.GitHubClientID != "" && s.cfg.GitHubClientSecret != "" && s.cfg.URL != ""
}

func (s *Server) githubWeb() string {
	if s.cfg.GitHubURL != "" {
		return strings.TrimSuffix(s.cfg.GitHubURL, "/")
	}
	return "https://github.com"
}

func (s *Server) githubAPI() string {
	if s.cfg.GitHubAPIURL != "" {
		return strings.TrimSuffix(s.cfg.GitHubAPIURL, "/")
	}
	return "https://api.github.com"
}

func (s *Server) base() string { return strings.TrimSuffix(s.cfg.URL, "/") }

// secure: cookies only over HTTPS when the server is reached over it.
func (s *Server) secure() bool { return strings.HasPrefix(s.cfg.URL, "https://") }

// localPath is next when it is a path of this server, "/" otherwise: a sign
// in never sends the browser elsewhere.
func localPath(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

// githubStart sends the browser to GitHub, to come back to next.
func (s *Server) githubStart(w http.ResponseWriter, r *http.Request) {
	if !s.githubOn() {
		s.page(w, http.StatusNotFound, "Signing in with GitHub isn't set up on this server.", nil)
		return
	}
	var b [24]byte
	rand.Read(b[:])
	state := base64.RawURLEncoding.EncodeToString(b[:])
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: state + "|" + localPath(r.URL.Query().Get("next")),
		Path: pathGitHub, MaxAge: 600, HttpOnly: true, Secure: s.secure(), SameSite: http.SameSiteLaxMode})
	q := url.Values{"client_id": {s.cfg.GitHubClientID}, "redirect_uri": {s.base() + pathGitHubCallback}, "scope": {githubScopes}, "state": {state}}
	http.Redirect(w, r, s.githubWeb()+"/login/oauth/authorize?"+q.Encode(), http.StatusFound)
}

// githubCallback is GitHub sending the browser back: the account is made
// or updated, and the browser signed in.
func (s *Server) githubCallback(w http.ResponseWriter, r *http.Request) {
	if !s.githubOn() {
		s.page(w, http.StatusNotFound, "Signing in with GitHub isn't set up on this server.", nil)
		return
	}
	c, err := r.Cookie(stateCookie)
	state, next, _ := strings.Cut(func() string {
		if err != nil {
			return ""
		}
		return c.Value
	}(), "|")
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Path: pathGitHub, MaxAge: -1, HttpOnly: true, Secure: s.secure(), SameSite: http.SameSiteLaxMode})
	got := r.URL.Query().Get("state")
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(got)) != 1 {
		s.page(w, http.StatusBadRequest, "This sign-in didn't start here, or took too long. Start it again.", nil)
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		s.page(w, http.StatusForbidden, "GitHub didn't sign you in: "+e+".", nil)
		return
	}
	user, err := s.githubUser(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		s.log.Warn("github sign-in", "err", err)
		s.page(w, http.StatusBadGateway, "GitHub couldn't sign you in. Try again.", nil)
		return
	}
	acc, err := s.db.GitHubAccount(r.Context(), user.ID, user.Login, user.Name, user.Email, user.Avatar)
	if err != nil {
		s.pageInternal(w, err)
		return
	}
	// Signing in to the library joins no team, and starts no session here.
	if s.library != nil && library.IsSignIn(next) {
		s.library.FinishSignIn(w, r, acc.ID, next)
		return
	}
	s.joinDefault(r.Context(), acc.ID)
	token, err := s.db.StartWebSession(r.Context(), acc.ID)
	if err != nil {
		s.pageInternal(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: int((12 * time.Hour).Seconds()),
		HttpOnly: true, Secure: s.secure(), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, localPath(next), http.StatusFound)
}

type githubUser struct {
	ID     int64  `json:"id"`
	Login  string `json:"login"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Avatar string `json:"avatar_url"`
}

// githubUser trades the callback's code for a token and asks GitHub whose
// it is, with their primary email when their profile hides it.
func (s *Server) githubUser(ctx context.Context, code string) (githubUser, error) {
	var u githubUser
	if code == "" {
		return u, errors.New("no code")
	}
	token, err := s.githubToken(ctx, code, s.base()+pathGitHubCallback)
	if err != nil {
		return u, err
	}
	get := func(path string, v any) error { return s.githubGet(ctx, token, path, v) }
	if err := get("/user", &u); err != nil {
		return u, fmt.Errorf("user: %w", err)
	}
	if u.ID == 0 {
		return u, errors.New("user: no id")
	}
	if u.Email == "" {
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if get("/user/emails", &emails) == nil {
			for _, e := range emails {
				if e.Primary && e.Verified {
					u.Email = e.Email
				}
			}
		}
	}
	return u, nil
}

// githubToken trades a callback's code for a token, as of the address it came back to.
func (s *Server) githubToken(ctx context.Context, code, redirect string) (string, error) {
	form := url.Values{"client_id": {s.cfg.GitHubClientID}, "client_secret": {s.cfg.GitHubClientSecret}, "code": {code}, "redirect_uri": {redirect}}
	req, _ := http.NewRequestWithContext(ctx, "POST", s.githubWeb()+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := githubJSON(req, &tok); err != nil {
		return "", fmt.Errorf("token: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("token: %s", tok.Error)
	}
	return tok.AccessToken, nil
}

// githubGet reads path of GitHub's API with token.
func (s *Server) githubGet(ctx context.Context, token, path string, v any) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", s.githubAPI()+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	return githubJSON(req, v)
}

func githubJSON(req *http.Request, v any) error {
	req.Header.Set("User-Agent", "enowx-cloud")
	res, err := githubClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub answered %d", res.StatusCode)
	}
	return json.NewDecoder(http.MaxBytesReader(nil, res.Body, 1<<20)).Decode(v)
}
