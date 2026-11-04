package library

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

// Signing in on the library's website with the cloud's GitHub sign-in: the
// browser comes to /library/authorize with a state and a PKCE challenge, signs
// in, and goes back to the address this server was set up with, carrying a code
// good once, for a minute. The website's server trades it for a session token
// that opens /v1/library and nothing else. Signing in here joins no team: this
// path never calls what joins the default team, and starts no session of the
// cloud's own pages.

var (
	validState     = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)
	validChallenge = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

// IsSignIn says next, where a GitHub sign-in goes back to, is this page.
func IsSignIn(next string) bool {
	return next == api.PathLibraryAuthorize || strings.HasPrefix(next, api.PathLibraryAuthorize+"?")
}

// authorize sends the browser back with a code when it's known, or to sign in.
func (s *Service) authorize(w http.ResponseWriter, r *http.Request) {
	state, challenge, ok := s.authorizeParams(w, r)
	if !ok {
		return
	}
	if acc := s.host.WebAccount(r); acc != nil {
		s.sendBack(w, r, acc.ID, state, challenge)
		return
	}
	u := s.host.SignInURL(r.URL.RequestURI())
	if u == "" {
		http.Error(w, "Signing in with GitHub isn't set up on this server.", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, u, http.StatusFound)
}

// authorizeParams are a sign-in's own parameters, checked, or answers.
func (s *Service) authorizeParams(w http.ResponseWriter, r *http.Request) (state, challenge string, ok bool) {
	q := r.URL.Query()
	state, challenge = q.Get("state"), q.Get("code_challenge")
	switch {
	case s.cfg.ReturnURL == "" || s.cfg.WebSecret == "":
		http.Error(w, "The library's website isn't set up on this server.", http.StatusNotFound)
	case !validState.MatchString(state) || !validChallenge.MatchString(challenge) || q.Get("code_challenge_method") != "S256":
		http.Error(w, "This sign-in didn't come from the library's website.", http.StatusBadRequest)
	default:
		return state, challenge, true
	}
	return "", "", false
}

// FinishSignIn is GitHub having signed account accID in on its way back to
// this page, at next.
func (s *Service) FinishSignIn(w http.ResponseWriter, r *http.Request, accID, next string) {
	u, err := url.Parse(next)
	if err != nil {
		http.Error(w, "This sign-in didn't come from the library's website.", http.StatusBadRequest)
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL = u
	state, challenge, ok := s.authorizeParams(w, r2)
	if !ok {
		return
	}
	s.sendBack(w, r, accID, state, challenge)
}

func (s *Service) sendBack(w http.ResponseWriter, r *http.Request, accID, state, challenge string) {
	code, err := s.db.AddLibraryCode(r.Context(), accID, challenge)
	if err != nil {
		s.log.Error("library: code", "err", err)
		http.Error(w, "Something went wrong on the server: try again.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, s.cfg.ReturnURL+"?"+url.Values{"code": {code}, "state": {state}}.Encode(), http.StatusFound)
}

// exchange trades a code and its verifier for a session, for the website's
// server alone. A code is used up by trying it.
func (s *Service) exchange(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryExchange
	if !readJSON(w, r, &p) {
		return
	}
	accID, challenge, err := s.db.TakeLibraryCode(r.Context(), p.Code)
	if errors.Is(err, store.ErrExpired) {
		fail(w, http.StatusBadRequest, "this sign-in expired, or was used: start it again")
		return
	}
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	sum := sha256.Sum256([]byte(p.Verifier))
	if subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) != 1 {
		fail(w, http.StatusBadRequest, "this sign-in expired, or was used: start it again")
		return
	}
	me, err := s.db.LibraryProfile(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if me.Role, err = s.roleOf(r.Context(), accID); err != nil {
		s.answer(w, nil, err)
		return
	}
	token, expires, err := s.db.StartLibrarySession(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibrarySession{Token: token, Expires: expires, Me: me})
}

// signOut ends the session whose token asks: its holder's alone to end.
func (s *Service) signOut(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || !store.IsLibraryToken(token) {
		fail(w, http.StatusUnauthorized, "sign in first")
		return
	}
	s.answer(w, nil, s.db.EndLibrarySession(r.Context(), token))
}
