package library

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

// Creators: their profiles and the open for work list.
//
// A hidden or blocked profile answers as no profile at all, to all but its creator and
// moderators. A hidden creator's designs leave every list (browse, feed, others'
// collections, their profile) but stay at their addresses with their credit: people may
// hold links and copies, whose license needs that credit. Item.Unlisted tells the
// website to keep search engines off them. Unpublishing is how a design goes away.

func (s *Service) mountCreators(mux *http.ServeMux) {
	handle := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, h) }
	handle("GET "+api.PathLibraryCreators, s.public(s.openCreators))
	handle("GET "+api.PathLibraryCreator, s.public(s.getCreator))
	handle("GET "+api.PathLibraryCreatorItems, s.public(s.creatorItems))
	handle("GET "+api.PathLibraryCreatorCollections, s.public(s.creatorCollections))
	handle("PUT "+api.PathLibraryMe, s.signedIn(s.setProfile))
	s.mountSocial(handle)
	s.mountErase(handle)
}

// member is for what any signed-in account does, up to limit a minute each.
func (s *Service) member(action string, limit int, h http.HandlerFunc) http.HandlerFunc {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request) {
		if !s.limits.allow(action+"\x00"+asker(r).account.ID, limit) {
			w.Header().Set("Retry-After", "60")
			fail(w, http.StatusTooManyRequests, "that's a lot at once: wait a minute")
			return
		}
		h(w, r)
	})
}

// active is member for what changes something public or someone else's numbers: a
// blocked account doesn't, though it can always take its data out or delete it.
func (s *Service) active(action string, limit int, h http.HandlerFunc) http.HandlerFunc {
	return s.member(action, limit, func(w http.ResponseWriter, r *http.Request) {
		me, err := s.db.LibraryProfile(r.Context(), asker(r).account.ID)
		if err != nil {
			s.answer(w, nil, err)
			return
		}
		if me.Trust == api.TrustBlocked {
			fail(w, http.StatusForbidden, "this account can't do that")
			return
		}
		h(w, r)
	})
}

// viewer is who a public request's optional token is, "" for nobody, and their role.
func (s *Service) viewer(r *http.Request) (string, string) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return "", ""
	}
	var accID string
	if store.IsLibraryToken(token) {
		acc, err := s.db.LibrarySessionAccount(r.Context(), token)
		if err != nil {
			return "", ""
		}
		accID = acc.ID
	} else {
		me, err := s.db.Me(r.Context(), token)
		if err != nil {
			return "", ""
		}
		accID = me.Account.ID
	}
	role, err := s.roleOf(r.Context(), accID)
	if err != nil {
		return accID, ""
	}
	return accID, role
}

// offsetOf reads a page's cursor, or answers.
func offsetOf(w http.ResponseWriter, r *http.Request) (int, bool) {
	c := r.URL.Query().Get("cursor")
	if c == "" {
		return 0, true
	}
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err == nil {
		var n int
		if n, err = strconv.Atoi(string(raw)); err == nil && n >= 0 && n <= 100000 {
			return n, true
		}
	}
	fail(w, http.StatusBadRequest, "this cursor doesn't read")
	return 0, false
}

func limitOf(r *http.Request) int {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		return 24
	}
	return limit
}

func nextOf(more bool, offset, n int) string {
	if !more {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset + n)))
}

func shownAll(items []api.LibraryItem) []api.LibraryItem {
	for i := range items {
		items[i] = shown(items[i])
	}
	return items
}

func badgesOf(p store.LibraryProfileRow) []string {
	if p.Contributor {
		return []string{api.BadgeContributor}
	}
	return []string{}
}

// publicProfile is what anyone sees of p: what p chose to show, and no more.
func (s *Service) publicProfile(ctx context.Context, p store.LibraryProfileRow) (api.LibraryProfile, error) {
	out := api.LibraryProfile{
		LibraryCreator: api.LibraryCreator{ID: p.AccountID, Handle: p.Handle, Name: p.Name, Avatar: p.Avatar},
		Bio:            p.Bio, Links: p.Links, Badges: badgesOf(p), Since: p.Created,
	}
	if p.ShowGitHub {
		out.GitHub = p.GitHub
	}
	if p.ShowStats {
		stats, err := s.db.LibraryCreatorStats(ctx, p.AccountID)
		if err != nil {
			return out, err
		}
		out.Stats = &stats
	}
	if p.OpenForWork && p.ContactURL != "" {
		out.OpenForWork = &api.LibraryOpenForWork{Kinds: p.WorkKinds, Skills: p.Skills, TimeZone: p.TimeZone, ContactURL: p.ContactURL}
	}
	return out, nil
}

// settingsOf is p as its creator edits it, whatever shows of it.
func settingsOf(p store.LibraryProfileRow) *api.LibraryProfileParams {
	return &api.LibraryProfileParams{Handle: p.Handle, Bio: p.Bio, Links: p.Links, ShowGitHub: p.ShowGitHub, ShowStats: p.ShowStats, Hidden: p.Hidden,
		OpenForWork: p.OpenForWork, WorkKinds: p.WorkKinds, Skills: p.Skills, TimeZone: p.TimeZone, ContactURL: p.ContactURL}
}

// shownCreator is the profile a request names, when it may see it, or answers: one
// that doesn't show answers exactly as one that doesn't exist.
func (s *Service) shownCreator(w http.ResponseWriter, r *http.Request) (store.LibraryProfileRow, string, string, bool) {
	p, err := s.db.LibraryCreator(r.Context(), r.PathValue("handle"))
	if err != nil {
		s.answer(w, nil, err)
		return p, "", "", false
	}
	accID, role := s.viewer(r)
	if (p.Hidden || p.Trust == api.TrustBlocked) && accID != p.AccountID && role == "" {
		s.answer(w, nil, store.ErrNotFound)
		return p, "", "", false
	}
	return p, accID, role, true
}

func (s *Service) getCreator(w http.ResponseWriter, r *http.Request) {
	p, accID, role, ok := s.shownCreator(w, r)
	if !ok {
		return
	}
	out, err := s.publicProfile(r.Context(), p)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	s.addViewer(r.Context(), &out, p, accID, role)
	reply(w, out)
}

// addViewer adds what's for the asker alone: their own settings, their follow, and
// for those who moderate whether it's hidden.
func (s *Service) addViewer(ctx context.Context, out *api.LibraryProfile, p store.LibraryProfileRow, accID, role string) {
	if accID == "" {
		return
	}
	own := accID == p.AccountID
	if own || role != "" {
		out.Hidden = p.Hidden
	}
	out.Viewer = &api.LibraryViewer{Own: own}
	if own {
		out.Settings = settingsOf(p)
	} else if st, err := s.db.LibraryFollowState(ctx, accID, p.AccountID); err == nil {
		out.Viewer.Following = st.Following
	}
}

func (s *Service) creatorItems(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := s.shownCreator(w, r)
	if !ok {
		return
	}
	offset, ok := offsetOf(w, r)
	if !ok {
		return
	}
	items, more, err := s.db.LibraryCreatorItems(r.Context(), p.AccountID, r.URL.Query().Get("sort"), limitOf(r), offset)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryItems{Items: shownAll(items), Next: nextOf(more, offset, len(items))})
}

func (s *Service) creatorCollections(w http.ResponseWriter, r *http.Request) {
	p, accID, _, ok := s.shownCreator(w, r)
	if !ok {
		return
	}
	list, err := s.db.LibraryCollectionsOf(r.Context(), p.AccountID, accID == p.AccountID, "")
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryCollections{Collections: list})
}

// openCreators is the open for work list. It's the only list of people there is, so
// open=1 is a must.
func (s *Service) openCreators(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("open") != "1" {
		fail(w, http.StatusBadRequest, "the only list of creators is ?open=1")
		return
	}
	offset, ok := offsetOf(w, r)
	if !ok {
		return
	}
	query := store.LibraryOpenQuery{Kind: q.Get("kind"), Skill: cleanText(q.Get("skill"), 32), Limit: limitOf(r), Offset: offset}
	if query.Kind != "" && !oneOf(api.LibraryWorkKinds, query.Kind) {
		fail(w, http.StatusBadRequest, "no such kind of work")
		return
	}
	if zone := q.Get("zone"); zone != "" {
		if query.TimeZones, ok = zonesOf(zone); !ok {
			fail(w, http.StatusBadRequest, "no such span of time zones")
			return
		}
	}
	rows, more, err := s.db.LibraryOpenCreators(r.Context(), query)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	ids := make([]string, len(rows))
	for i, p := range rows {
		ids[i] = p.AccountID
	}
	works, err := s.db.LibraryWorksOf(r.Context(), ids, 3)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	skills, err := s.db.LibraryOpenSkills(r.Context(), 16)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	out := api.LibraryOpenCreators{Creators: []api.LibraryOpenCreator{}, Skills: skills, Next: nextOf(more, offset, len(rows))}
	for _, p := range rows {
		line := api.LibraryOpenCreator{
			LibraryCreator: api.LibraryCreator{ID: p.AccountID, Handle: p.Handle, Name: p.Name, Avatar: p.Avatar},
			Badges:         badgesOf(p),
			OpenForWork:    api.LibraryOpenForWork{Kinds: p.WorkKinds, Skills: p.Skills, TimeZone: p.TimeZone, ContactURL: p.ContactURL},
			Works:          works[p.AccountID],
		}
		out.Creators = append(out.Creators, line)
	}
	reply(w, out)
}

// profilePerMinute is how often a profile may change, but for hiding it and turning
// open for work off, which go through always: "any time" means any time.
const profilePerMinute = 20

// setProfile keeps what a creator set.
func (s *Service) setProfile(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryProfileParams
	if !readJSON(w, r, &p) {
		return
	}
	accID := asker(r).account.ID
	was, err := s.db.LibraryProfileOf(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if was.Trust == api.TrustBlocked {
		fail(w, http.StatusForbidden, "this account can't do that")
		return
	}
	withdraws := (p.Hidden && !was.Hidden) || (!p.OpenForWork && was.OpenForWork)
	if !s.limits.allow("profile\x00"+accID, profilePerMinute) && !withdraws {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "that's a lot at once: wait a minute")
		return
	}
	clean, err := cleanProfile(p, was.Handle)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.db.SetLibraryProfile(r.Context(), accID, clean); err != nil {
		s.answer(w, nil, err)
		return
	}
	now, err := s.db.LibraryProfileOf(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	out, err := s.publicProfile(r.Context(), now)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	s.addViewer(r.Context(), &out, now, accID, asker(r).role)
	reply(w, out)
}

// errCreatorOwn is liking or following oneself, which would only move a number.
var errCreatorOwn = errors.New("store: that's your own")
