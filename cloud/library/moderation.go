package library

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/blobstore"
)

// Moderating: every action here says why, and is in the log for good.

func (s *Service) queue(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.LibraryReview(r.Context(), 50)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	for i := range items {
		items[i].LibraryItem = shown(items[i].LibraryItem)
	}
	reports, err := s.db.LibraryOpenReports(r.Context(), 50)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryQueue{Items: items, Reports: reports, Role: asker(r).role})
}

// reasonOf is a decision's reason, or answers: the log needs one.
func reasonOf(w http.ResponseWriter, reason string) (string, bool) {
	reason = cleanText(reason, 1000)
	if reason == "" {
		fail(w, http.StatusBadRequest, "say why: it goes in the log")
		return "", false
	}
	return reason, true
}

func (s *Service) decideItem(w http.ResponseWriter, r *http.Request) {
	var d api.LibraryDecision
	if !readJSON(w, r, &d) {
		return
	}
	reason, ok := reasonOf(w, d.Reason)
	if !ok {
		return
	}
	id, actor := r.PathValue("id"), asker(r).account.ID
	switch d.Action {
	case api.DecisionApprove, api.DecisionReject:
		kind := api.NotifyRejected
		if d.Action == api.DecisionApprove {
			kind = api.NotifyApproved
		}
		s.decided(w, r, id, kind, reason, s.db.DecideLibraryVersion(r.Context(), actor, id, d.Version, d.Action == api.DecisionApprove, reason))
	case api.DecisionRemove:
		s.decided(w, r, id, api.NotifyRemoved, reason, s.db.RemoveLibraryItem(r.Context(), actor, id, reason))
	case api.DecisionAnnounce:
		s.announce(w, r, id, reason)
	case api.DecisionFeature, api.DecisionUnfeature:
		s.answer(w, nil, s.db.FeatureLibraryItem(r.Context(), actor, id, d.Action == api.DecisionFeature, reason))
	default:
		fail(w, http.StatusBadRequest, "no such decision")
	}
}

// decided answers a decision on item id, telling its owner once it's made.
func (s *Service) decided(w http.ResponseWriter, r *http.Request, id, kind, reason string, err error) {
	if err == nil {
		if owner, e := s.db.LibraryOwner(r.Context(), id); e == nil {
			s.notify(r.Context(), owner, kind, id, "", reason)
		}
	}
	s.answer(w, nil, err)
}

func (s *Service) decideReport(w http.ResponseWriter, r *http.Request) {
	var d api.LibraryDecision
	if !readJSON(w, r, &d) {
		return
	}
	reason, ok := reasonOf(w, d.Reason)
	if !ok {
		return
	}
	if d.Action != api.DecisionDismiss && d.Action != api.DecisionUphold {
		fail(w, http.StatusBadRequest, "no such decision")
		return
	}
	id := r.PathValue("id")
	target, err := s.db.LibraryReportTarget(r.Context(), id)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if err := s.db.DecideLibraryReport(r.Context(), asker(r).account.ID, id, d.Action == api.DecisionUphold, reason); err != nil {
		s.answer(w, nil, err)
		return
	}
	if d.Action == api.DecisionUphold && target.Account != "" {
		s.notify(r.Context(), target.Account, api.NotifyReportUpheld, target.ItemID, "", reason)
	}
	w.WriteHeader(http.StatusNoContent)
}

// block stops an account publishing and takes what it shows down. Those who
// moderate can't be blocked from here: the owner takes the role away first.
func (s *Service) block(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryReason
	if !readJSON(w, r, &p) {
		return
	}
	reason, ok := reasonOf(w, p.Reason)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if role, err := s.roleOf(r.Context(), id); err != nil {
		s.answer(w, nil, err)
		return
	} else if role != "" {
		fail(w, http.StatusConflict, "they moderate the library: the owner takes that away first")
		return
	}
	s.answer(w, nil, s.db.BlockLibraryAccount(r.Context(), asker(r).account.ID, id, true, reason))
}

func (s *Service) unblock(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryReason
	if !readJSON(w, r, &p) {
		return
	}
	reason, ok := reasonOf(w, p.Reason)
	if !ok {
		return
	}
	s.answer(w, nil, s.db.BlockLibraryAccount(r.Context(), asker(r).account.ID, r.PathValue("id"), false, reason))
}

// adminPart is a version's document or picture for a moderator, under review
// or not, sandboxed even here: nobody's browser runs it as this server.
func (s *Service) adminPart(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		fail(w, http.StatusBadRequest, "a version is a number")
		return
	}
	part := r.PathValue("part")
	h := w.Header()
	switch part {
	case api.PartDoc, api.PartDocPhone:
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Encoding", "gzip")
	case api.PartThumb, api.PartThumbPhone:
		h.Set("Content-Type", "image/webp")
	case api.PartVideo:
		h.Set("Content-Type", "video/mp4")
	default:
		fail(w, http.StatusNotFound, "not found")
		return
	}
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	data, err := s.objects.Get(r.Context(), objectKey(r.PathValue("id"), n, part))
	if err != nil {
		h.Del("Content-Encoding")
		if errors.Is(err, blobstore.ErrNotFound) {
			fail(w, http.StatusNotFound, "not found")
			return
		}
		s.log.Error("library: get", "err", err)
		fail(w, http.StatusBadGateway, "the library's storage didn't answer: try again")
		return
	}
	w.Write(data)
}

func (s *Service) readLog(w http.ResponseWriter, r *http.Request) {
	before := int64(0)
	if c := r.URL.Query().Get("cursor"); c != "" {
		raw, err := base64.RawURLEncoding.DecodeString(c)
		if err == nil {
			before, err = strconv.ParseInt(string(raw), 10, 64)
		}
		if err != nil {
			fail(w, http.StatusBadRequest, "this cursor doesn't read")
			return
		}
	}
	const page = 50
	entries, err := s.db.LibraryLog(r.Context(), before, page)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	out := api.LibraryLog{Entries: entries}
	if len(entries) == page {
		out.Next = base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(entries[page-1].ID, 10)))
	}
	reply(w, out)
}

func (s *Service) usage(w http.ResponseWriter, r *http.Request) {
	u, err := s.db.LibraryUsage(r.Context())
	s.answer(w, u, err)
}

// accounts is who the owner can pick a moderator from.
func (s *Service) accounts(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		reply(w, []api.LibraryAccount{})
		return
	}
	list, err := s.db.LibraryAccounts(r.Context(), q)
	s.answer(w, list, err)
}

func (s *Service) moderators(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.LibraryModerators(r.Context())
	s.answer(w, list, err)
}

func (s *Service) appoint(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryAppoint
	if !readJSON(w, r, &p) {
		return
	}
	reason, ok := reasonOf(w, p.Reason)
	if !ok {
		return
	}
	s.answer(w, nil, s.db.AppointLibraryModerator(r.Context(), asker(r).account.ID, p.AccountID, reason))
}

func (s *Service) dismiss(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryReason
	if !readJSON(w, r, &p) {
		return
	}
	reason, ok := reasonOf(w, p.Reason)
	if !ok {
		return
	}
	s.answer(w, nil, s.db.RemoveLibraryModerator(r.Context(), asker(r).account.ID, r.PathValue("id"), reason))
}
