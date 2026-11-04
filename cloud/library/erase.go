package library

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// Taking one's data out of the library, and deleting it.

func (s *Service) mountErase(handle func(string, http.HandlerFunc)) {
	handle("GET "+api.PathLibraryExport, s.member("export", 2, s.export))
	handle("DELETE "+api.PathLibraryMe, s.signedIn(s.deleteMe))
}

// export answers what the library keeps of the asker, as a file. A blocked account
// may too: it's their data.
func (s *Service) export(w http.ResponseWriter, r *http.Request) {
	accID := asker(r).account.ID
	p, err := s.db.LibraryProfileOf(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	out, err := s.db.LibraryExport(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if out.Profile, err = s.publicProfile(r.Context(), p); err != nil {
		s.answer(w, nil, err)
		return
	}
	out.Profile.Settings = settingsOf(p)
	out.Profile.Hidden = p.Hidden
	out.Trust = p.Trust
	w.Header().Set("Content-Disposition", `attachment; filename="enowx-ui-`+p.Handle+`.json"`)
	reply(w, out)
}

// purgeWait is how long deleting waits for its objects before leaving the rest to
// the hourly sweep.
const purgeWait = 20 * time.Second

// deleteMe deletes what the library keeps of the asker, once they typed their handle.
// The Cloud account, its devices and its teams stay; signing in to the library again
// starts an empty profile. Those who moderate give that up first.
func (s *Service) deleteMe(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryDeleteMe
	if !readJSON(w, r, &p) {
		return
	}
	a := asker(r)
	if !s.limits.allow("delete\x00"+a.account.ID, 5) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "that's a lot at once: wait a minute")
		return
	}
	if a.role != "" {
		fail(w, http.StatusConflict, "you moderate the library: the owner takes that away first")
		return
	}
	me, err := s.db.LibraryProfile(r.Context(), a.account.ID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if strings.TrimSpace(p.Confirm) != me.Account.Handle {
		fail(w, http.StatusBadRequest, "type your handle to confirm")
		return
	}
	if err := s.db.DeleteLibraryAccount(r.Context(), a.account.ID); err != nil {
		s.answer(w, nil, err)
		return
	}
	// By the existing purge: what doesn't finish now goes on the next sweep.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), purgeWait)
	defer cancel()
	s.SweepOnce(ctx)
	w.WriteHeader(http.StatusNoContent)
}
