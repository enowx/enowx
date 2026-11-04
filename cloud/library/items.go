package library

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/blobstore"
	"github.com/enowx/enowx/cloud/store"
)

// What anyone can read, and what an owner can do with their own.

func (s *Service) listItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit == 0 {
		limit = 24
	}
	offset := 0
	if c := q.Get("cursor"); c != "" {
		raw, err := base64.RawURLEncoding.DecodeString(c)
		if err != nil {
			fail(w, http.StatusBadRequest, "this cursor doesn't read")
			return
		}
		if offset, err = strconv.Atoi(string(raw)); err != nil || offset < 0 || offset > 100000 {
			fail(w, http.StatusBadRequest, "this cursor doesn't read")
			return
		}
	}
	items, more, err := s.db.LibraryItems(r.Context(), store.LibraryQuery{Q: q.Get("q"), Category: q.Get("category"), Kind: q.Get("kind"),
		License: q.Get("license"), Tag: q.Get("tag"), Sort: q.Get("sort"), Limit: limit, Offset: offset})
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	for i := range items {
		items[i] = shown(items[i])
	}
	out := api.LibraryItems{Items: items}
	if more {
		out.Next = base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset + len(items))))
	}
	reply(w, out)
}

// gone says an item taken back or down answers 410 forever.
func gone(status string) bool {
	return status == api.LibraryWithdrawn || status == api.LibraryUnpublished || status == api.LibraryRemoved
}

// mayPeek says an optional token in the request belongs to its owner or a
// moderator, who see what isn't public yet.
func (s *Service) mayPeek(r *http.Request, it api.LibraryItem) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return false
	}
	var accID string
	if store.IsLibraryToken(token) {
		acc, err := s.db.LibrarySessionAccount(r.Context(), token)
		if err != nil {
			return false
		}
		accID = acc.ID
	} else {
		me, err := s.db.Me(r.Context(), token)
		if err != nil {
			return false
		}
		accID = me.Account.ID
	}
	if accID == it.Creator.ID {
		return true
	}
	role, err := s.roleOf(r.Context(), accID)
	return err == nil && role != ""
}

// publicItem is item id when the request may see it, or answers.
func (s *Service) publicItem(w http.ResponseWriter, r *http.Request) (api.LibraryItem, bool) {
	it, err := s.db.LibraryItem(r.Context(), r.PathValue("id"))
	switch {
	case err != nil:
		s.answer(w, nil, err)
	case gone(it.Status):
		fail(w, http.StatusGone, "this design was taken down or unpublished")
	case it.Status != api.LibraryLive && !s.mayPeek(r, it):
		fail(w, http.StatusNotFound, "not found")
	default:
		return shown(it), true
	}
	return it, false
}

func (s *Service) getItem(w http.ResponseWriter, r *http.Request) {
	if it, ok := s.publicItem(w, r); ok {
		reply(w, it)
	}
}

// listVersions are the versions anyone may look at: what shows now, and what
// showed before it. Nothing under review, nor anyone's prompt or scan.
func (s *Service) listVersions(w http.ResponseWriter, r *http.Request) {
	it, ok := s.publicItem(w, r)
	if !ok {
		return
	}
	rows, err := s.db.LibraryVersions(r.Context(), it.ID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	out := []api.LibraryVersion{}
	for _, v := range rows {
		if v.Status == api.LibraryLive || v.Status == api.LibrarySuperseded {
			a := v.API()
			a.Scan, a.Prompt = nil, ""
			out = append(out, a)
		}
	}
	reply(w, out)
}

// getPrompt is what Copy prompt copies. A view-only item answers 403 here, not
// only by having no button.
func (s *Service) getPrompt(w http.ResponseWriter, r *http.Request) {
	it, ok := s.publicItem(w, r)
	if !ok {
		return
	}
	if !it.Actions.CopyPrompt {
		fail(w, http.StatusForbidden, "this design is view only")
		return
	}
	v, err := s.db.LibraryVersion(r.Context(), it.ID, it.LiveVersion)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if v.Prompt == "" || v.PromptApproved == "" {
		fail(w, http.StatusNotFound, "this design has no prompt")
		return
	}
	reply(w, api.LibraryPrompt{Prompt: v.Prompt + "\n\n" + s.credit(it)})
}

// credit says where a copy comes from, who made it and under which license.
func (s *Service) credit(it api.LibraryItem) string {
	address := strings.TrimSuffix(s.cfg.PublicURL, "/") + "/d/" + it.ID + "/" + it.Slug
	return "Design: " + it.Title + " by " + it.Creator.Handle + "\n" + address + "\nLicense: " + it.License
}

// object is a live version's document or picture, for the website alone: nobody's
// browser gets other people's HTML from this server's address, and the bucket's
// token, which reads teams' files too, stays here. The bytes come as they're
// kept (a document gzipped), and never change for a version, so the website keeps
// them; whether they may still show is what it asks about the item.
func (s *Service) object(w http.ResponseWriter, r *http.Request) {
	part := r.PathValue("part")
	if part != api.PartDoc && part != api.PartDocPhone && part != api.PartThumb && part != api.PartThumbPhone {
		fail(w, http.StatusNotFound, "not found")
		return
	}
	n, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		fail(w, http.StatusBadRequest, "a version is a number")
		return
	}
	it, err := s.db.LibraryItem(r.Context(), r.PathValue("id"))
	switch {
	case err != nil:
		s.answer(w, nil, err)
		return
	case gone(it.Status):
		fail(w, http.StatusGone, "this design was taken down or unpublished")
		return
	case it.Status != api.LibraryLive || it.LiveVersion != n:
		fail(w, http.StatusNotFound, "not found")
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
	data, err := s.objects.Get(r.Context(), objectKey(it.ID, n, part))
	if errors.Is(err, blobstore.ErrNotFound) {
		fail(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.log.Error("library: get", "item", it.ID, "err", err)
		fail(w, http.StatusBadGateway, "the library's storage didn't answer: try again")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Write(data)
}

// withdraw is an owner taking their item back, from the app or the website.
func (s *Service) withdraw(w http.ResponseWriter, r *http.Request) {
	id, a := r.PathValue("id"), asker(r)
	owner, err := s.db.LibraryOwner(r.Context(), id)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if owner != a.account.ID {
		fail(w, http.StatusForbidden, "only its creator can unpublish a design")
		return
	}
	s.answer(w, nil, s.db.WithdrawLibraryItem(r.Context(), a.account.ID, id, "by its creator"))
}

func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	a := asker(r)
	me, err := s.db.LibraryProfile(r.Context(), a.account.ID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	me.Role = a.role
	reply(w, me)
}

func (s *Service) mine(w http.ResponseWriter, r *http.Request) {
	accID := asker(r).account.ID
	me, err := s.db.LibraryProfile(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	items, err := s.db.LibraryMine(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	c, err := s.db.LibraryCounts(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	for i := range items {
		items[i].LibraryItem = shown(items[i].LibraryItem)
	}
	reply(w, api.LibraryMine{Items: items, Used: c.Used, Limit: api.MaxLibraryAccount, Trust: me.Trust})
}

// reportsPerDay is how many reports an account may make.
const reportsPerDay = 20

// report keeps a report: who made it is for moderators only.
func (s *Service) report(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryReportParams
	if !readJSON(w, r, &p) {
		return
	}
	accID := asker(r).account.ID
	p.Detail = cleanText(p.Detail, 2000)
	switch {
	case (p.ItemID == "") == (p.AccountID == ""):
		fail(w, http.StatusBadRequest, "report a design or an account")
		return
	case !oneOf(api.LibraryReasons, p.Reason):
		fail(w, http.StatusBadRequest, "pick a reason")
		return
	case p.Reason == "copyright" && p.Detail == "":
		fail(w, http.StatusBadRequest, "a copyright report says what is yours, and where it is")
		return
	}
	if p.ItemID != "" {
		it, err := s.db.LibraryItem(r.Context(), p.ItemID)
		if err != nil {
			s.answer(w, nil, err)
			return
		}
		if it.Status != api.LibraryLive {
			fail(w, http.StatusNotFound, "not found")
			return
		}
	} else if _, err := s.db.Account(r.Context(), p.AccountID); err != nil {
		s.answer(w, nil, err)
		return
	}
	n, err := s.db.LibraryReportsToday(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if n >= reportsPerDay {
		w.Header().Set("Retry-After", "3600")
		fail(w, http.StatusTooManyRequests, "that's many reports for one day: try again tomorrow")
		return
	}
	s.answer(w, nil, s.db.AddLibraryReport(r.Context(), accID, p))
}

// escaped is the website saying a preview tried to leave its frame: the item
// goes to moderators, once while the report is open.
func (s *Service) escaped(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.db.LibraryItem(r.Context(), id); err != nil {
		s.answer(w, nil, err)
		return
	}
	s.answer(w, nil, s.db.AddLibraryReport(r.Context(), "", api.LibraryReportParams{ItemID: id, Reason: "escaped-preview", Detail: "A preview tried to leave its frame."}))
}

// cleanText is s as text the library keeps and shows: control and
// direction-changing characters out, at most max characters.
func cleanText(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if n == max {
			break
		}
		if (unicode.IsControl(r) && r != '\n') || unicode.Is(unicode.Bidi_Control, r) || r == '​' {
			continue
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

var errRefused = errors.New("library: refused")
