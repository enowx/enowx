package library

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/blobstore"
	"github.com/enowx/enowx/cloud/store"
	"github.com/enowx/enowx/core/designshare"
)

// Taking something from the library: a component's code, a copy, a reference, a
// skill. The gates are here, not only in what a page shows: a view-only version
// answers 403, and what a version's kind doesn't have isn't there at all.

// mountReuse adds these routes.
func (s *Service) mountReuse(handle func(string, http.HandlerFunc)) {
	handle("GET "+api.PathLibraryCode, s.heavy(s.getCode))
	handle("GET "+api.PathLibrarySkill, s.heavy(s.getSkill))
	handle("GET "+api.PathLibrarySource, s.deviceOnly(s.heavy(s.getSource)))
	handle("GET "+api.PathLibraryReference, s.deviceOnly(s.heavy(s.getReference)))
}

// heavy is for what reads an object from the store: within the address's limit
// and the objects on their way at once, and off the places public reads hold for
// the database.
func (s *Service) heavy(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.fromWeb(r) && !s.limits.allow(s.host.ClientAddr(r), publicPerMinute) {
			w.Header().Set("Retry-After", "60")
			fail(w, http.StatusTooManyRequests, "too many requests from here: wait a minute")
			return
		}
		select {
		case s.objectReads <- struct{}{}:
			defer func() { <-s.objectReads }()
		case <-time.After(5 * time.Second):
			w.Header().Set("Retry-After", "5")
			fail(w, http.StatusServiceUnavailable, "the library is busy: try again")
			return
		case <-r.Context().Done():
			return
		}
		h(w, r)
	}
}

// versionAsked is the version a request asks of it: ?version=N, or what shows.
// Only what shows or showed before can be asked for.
func (s *Service) versionAsked(w http.ResponseWriter, r *http.Request, it api.LibraryItem) (store.LibraryVersionRow, bool) {
	n := it.LiveVersion
	if q := r.URL.Query().Get("version"); q != "" {
		var err error
		if n, err = strconv.Atoi(q); err != nil || n < 1 {
			fail(w, http.StatusBadRequest, "a version is a number")
			return store.LibraryVersionRow{}, false
		}
	}
	v, err := s.db.LibraryVersion(r.Context(), it.ID, n)
	if err != nil {
		s.answer(w, nil, err)
		return v, false
	}
	if v.Status != api.LibraryLive && v.Status != api.LibrarySuperseded {
		fail(w, http.StatusNotFound, "not found")
		return v, false
	}
	return v, true
}

// asVersion is it as version v says of itself: a license change applies from
// its version on, so an older version keeps the license and kind it had.
func asVersion(it api.LibraryItem, v store.LibraryVersionRow) api.LibraryItem {
	it.Kind, it.License = v.Kind, v.License
	var m store.LibraryMeta
	if json.Unmarshal([]byte(v.Meta), &m) == nil && m.Title != "" {
		it.Title = m.Title
	}
	return shown(it)
}

// usable is the item and version a request asks for when allow lets their license
// and kind have it, or answers: 403 for view only, 404 when there's none of it.
func (s *Service) usable(w http.ResponseWriter, r *http.Request, allow func(api.LibraryActions) bool) (api.LibraryItem, store.LibraryVersionRow, bool) {
	it, ok := s.publicItem(w, r)
	if !ok {
		return it, store.LibraryVersionRow{}, false
	}
	v, ok := s.versionAsked(w, r, it)
	if !ok {
		return it, v, false
	}
	it = asVersion(it, v)
	if it.License == api.LicenseViewOnly {
		fail(w, http.StatusForbidden, "this design is view only")
		return it, v, false
	}
	if !allow(it.Actions) {
		fail(w, http.StatusNotFound, "not found")
		return it, v, false
	}
	return it, v, true
}

// bundleOf is a version's bundle as its bytes, or answers.
func (s *Service) bundleOf(w http.ResponseWriter, r *http.Request, it api.LibraryItem, v store.LibraryVersionRow) ([]byte, bool) {
	raw, ok := s.objectOf(w, r, it, v, api.PartSource)
	if !ok {
		return nil, false
	}
	data, err := gunzipped(raw, api.MaxLibraryBundle+1)
	if err != nil || len(data) > api.MaxLibraryBundle {
		s.log.Error("library: a bundle doesn't read", "item", it.ID, "version", v.Version)
		fail(w, http.StatusBadGateway, "this design's copy doesn't read")
		return nil, false
	}
	return data, true
}

func (s *Service) objectOf(w http.ResponseWriter, r *http.Request, it api.LibraryItem, v store.LibraryVersionRow, part string) ([]byte, bool) {
	data, err := s.objects.Get(r.Context(), objectKey(it.ID, v.Version, part))
	if errors.Is(err, blobstore.ErrNotFound) {
		fail(w, http.StatusNotFound, "not found")
		return nil, false
	}
	if err != nil {
		s.log.Error("library: get", "item", it.ID, "err", err)
		fail(w, http.StatusBadGateway, "the library's storage didn't answer: try again")
		return nil, false
	}
	return data, true
}

func (s *Service) address(it api.LibraryItem) string {
	return strings.TrimSuffix(s.cfg.PublicURL, "/") + "/d/" + it.ID + "/" + it.Slug
}

func (s *Service) getCode(w http.ResponseWriter, r *http.Request) {
	it, v, ok := s.usable(w, r, func(a api.LibraryActions) bool { return a.UseComponent })
	if !ok {
		return
	}
	data, ok := s.bundleOf(w, r, it, v)
	if !ok {
		return
	}
	b, err := designshare.Parse(data)
	if err != nil {
		fail(w, http.StatusBadGateway, "this design's copy doesn't read")
		return
	}
	code, err := designshare.ComponentCode(b, designshare.Notice(it.Title, it.Creator.Handle, s.address(it), it.License))
	if errors.Is(err, designshare.ErrNoCode) {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryCode{Version: v.Version, License: it.License, HTML: code.HTML, CSS: code.CSS, Assets: code.Assets})
}

func (s *Service) getSource(w http.ResponseWriter, r *http.Request) {
	it, v, ok := s.usable(w, r, func(a api.LibraryActions) bool { return a.OpenInEnowX })
	if !ok {
		return
	}
	data, ok := s.bundleOf(w, r, it, v)
	if !ok {
		return
	}
	reply(w, api.LibrarySource{ItemID: it.ID, Version: v.Version, Slug: it.Slug, Title: it.Title, Handle: it.Creator.Handle, License: it.License, Address: s.address(it), Bundle: data})
}

func (s *Service) getReference(w http.ResponseWriter, r *http.Request) {
	it, v, ok := s.usable(w, r, func(a api.LibraryActions) bool { return a.UseReference })
	if !ok {
		return
	}
	data, ok := s.objectOf(w, r, it, v, api.PartThumb)
	if !ok {
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/webp")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	w.Write(data)
}

func (s *Service) getSkill(w http.ResponseWriter, r *http.Request) {
	it, v, ok := s.usable(w, r, func(a api.LibraryActions) bool { return a.UseReference })
	if !ok {
		return
	}
	data, ok := s.bundleOf(w, r, it, v)
	if !ok {
		return
	}
	b, err := designshare.Parse(data)
	if err != nil {
		fail(w, http.StatusBadGateway, "this design's copy doesn't read")
		return
	}
	prompt := ""
	if v.PromptApproved != "" {
		prompt = v.Prompt
	}
	h := w.Header()
	h.Set("Content-Type", "text/markdown; charset=utf-8")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(designshare.Skill(designshare.SkillInput{Slug: it.Slug, Title: it.Title, Handle: it.Creator.Handle, Address: s.address(it), License: it.License, Version: v.Version, Prompt: prompt, Frame: b.Frame})))
}
