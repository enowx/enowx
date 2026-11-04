package library

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

// Comments: plain text, sign-in needed, limited per account and per item. Their
// text is other people's data, which every answer holding it says.

const commentsPage = 30

// encodeCursor and decodeCursor hold a place in a list sorted by time and ID.
func encodeCursor(created, id string) string {
	if id == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(created + "|" + id))
}

func decodeCursor(c string) (created, id string, ok bool) {
	if c == "" {
		return "", "", true
	}
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return "", "", false
	}
	return strings.Cut(string(raw), "|")
}

// listComments is an item's comments. Closed by its creator, they're for its
// creator and moderators alone, to be opened again as they were.
func (s *Service) listComments(w http.ResponseWriter, r *http.Request) {
	it, ok := s.publicItem(w, r)
	if !ok {
		return
	}
	created, id, ok := decodeCursor(r.URL.Query().Get("cursor"))
	if !ok {
		fail(w, http.StatusBadRequest, "this cursor doesn't read")
		return
	}
	count, off, err := s.db.LibraryCommentsInfo(r.Context(), it.ID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	out := api.LibraryComments{Comments: []api.LibraryComment{}, Off: off, Count: count, Notice: api.LibraryDataNotice}
	if off && !s.mayPeek(r, it) {
		out.Count = 0
		reply(w, out)
		return
	}
	list, next, err := s.db.LibraryComments(r.Context(), it.ID, store.LibraryCommentCursor{Created: created, ID: id}, commentsPage)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	out.Comments, out.Next = list, encodeCursor(next.Created, next.ID)
	reply(w, out)
}

// addComment writes one, as a signed-in account that isn't blocked, on a live
// design whose comments are open, within the limits.
func (s *Service) addComment(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryCommentParams
	if !readJSON(w, r, &p) {
		return
	}
	body := cleanText(p.Body, api.MaxLibraryComment)
	if body == "" {
		fail(w, http.StatusBadRequest, "write something first")
		return
	}
	accID := asker(r).account.ID
	me, err := s.db.LibraryProfile(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if me.Trust == api.TrustBlocked {
		fail(w, http.StatusForbidden, "this account can't comment")
		return
	}
	it, err := s.db.LibraryItem(r.Context(), r.PathValue("id"))
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if it.Status != api.LibraryLive {
		fail(w, http.StatusNotFound, "not found")
		return
	}
	_, off, err := s.db.LibraryCommentsInfo(r.Context(), it.ID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if off {
		fail(w, http.StatusForbidden, "its creator turned comments off")
		return
	}
	c, err := s.db.LibraryCommentCounts(r.Context(), accID, it.ID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	busy := ""
	switch {
	case c.ByAccount >= api.MaxLibraryCommentsHour:
		busy = "that's many comments for one hour: try again later"
	case c.ByAccountHere >= api.MaxLibraryCommentsHere:
		busy = "that's many comments on this design for one hour: try again later"
	case c.ByItem >= api.MaxLibraryCommentsItem:
		busy = "this design has had many comments this hour: try again later"
	}
	if busy != "" {
		w.Header().Set("Retry-After", "3600")
		fail(w, http.StatusTooManyRequests, busy)
		return
	}
	if _, err := s.db.AddLibraryComment(r.Context(), it.ID, accID, body); err != nil {
		s.answer(w, nil, err)
		return
	}
	if it.Creator.ID != accID {
		s.notify(r.Context(), it.Creator.ID, api.NotifyComment, it.ID, accID, "")
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteComment is an author taking their own comment back.
func (s *Service) deleteComment(w http.ResponseWriter, r *http.Request) {
	s.answer(w, nil, s.db.DeleteLibraryComment(r.Context(), r.PathValue("id"), r.PathValue("comment"), asker(r).account.ID))
}

// setCommentsOff is a design's creator opening or closing its comments.
func (s *Service) setCommentsOff(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryCommentsOffParams
	if !readJSON(w, r, &p) {
		return
	}
	id, accID := r.PathValue("id"), asker(r).account.ID
	owner, err := s.db.LibraryOwner(r.Context(), id)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if owner != accID {
		fail(w, http.StatusForbidden, "only its creator can turn comments on or off")
		return
	}
	s.answer(w, nil, s.db.SetLibraryCommentsOff(r.Context(), accID, id, p.Off))
}

// removeComment is a moderator taking a comment off, with its reason in the log;
// its author is told why.
func (s *Service) removeComment(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryReason
	if !readJSON(w, r, &p) {
		return
	}
	reason, ok := reasonOf(w, p.Reason)
	if !ok {
		return
	}
	c, err := s.db.RemoveLibraryComment(r.Context(), asker(r).account.ID, r.PathValue("id"), reason)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	s.notify(r.Context(), c.AuthorID, api.NotifyRemoved, c.ItemID, "", reason)
	w.WriteHeader(http.StatusNoContent)
}
