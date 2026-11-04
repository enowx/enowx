package library

import (
	"errors"
	"net/http"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

// Likes, collections and follows. A like list, a follow list and a private collection
// answer their owner alone, moderators not excepted: to anyone else a private
// collection isn't there, as if it never was.

func (s *Service) mountSocial(handle func(string, http.HandlerFunc)) {
	handle("GET "+api.PathLibrarySaves, s.member("read", 240, s.listSaves))
	handle("GET "+api.PathLibrarySave, s.member("read", 240, s.saveState))
	handle("PUT "+api.PathLibrarySave, s.active("save", 60, s.save))
	handle("DELETE "+api.PathLibrarySave, s.member("save", 60, s.unsave))

	handle("PUT "+api.PathLibraryFollow, s.active("follow", 30, s.follow))
	handle("DELETE "+api.PathLibraryFollow, s.member("follow", 30, s.unfollow))
	handle("GET "+api.PathLibraryFollowing, s.member("read", 240, s.following))
	handle("GET "+api.PathLibraryFeed, s.member("read", 240, s.feed))

	handle("GET "+api.PathLibraryCollections, s.member("read", 240, s.myCollections))
	handle("POST "+api.PathLibraryCollections, s.active("collection", 60, s.addCollection))
	handle("GET "+api.PathLibraryCollection, s.public(s.getCollection))
	handle("PUT "+api.PathLibraryCollection, s.active("collection", 60, s.setCollection))
	handle("DELETE "+api.PathLibraryCollection, s.member("collection", 60, s.deleteCollection))
	handle("PUT "+api.PathLibraryCollectionItem, s.active("collection", 60, s.collect))
	handle("DELETE "+api.PathLibraryCollectionItem, s.member("collection", 60, s.uncollect))
}

// liveItem is the item a request names when it's live: nothing else can be liked or
// collected.
func (s *Service) liveItem(w http.ResponseWriter, r *http.Request, id string) (api.LibraryItem, bool) {
	it, err := s.db.LibraryItem(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && it.Status != api.LibraryLive) {
		fail(w, http.StatusNotFound, "not found")
		return it, false
	}
	if err != nil {
		s.answer(w, nil, err)
		return it, false
	}
	return it, true
}

func (s *Service) save(w http.ResponseWriter, r *http.Request) {
	id, accID := r.PathValue("id"), asker(r).account.ID
	it, ok := s.liveItem(w, r, id)
	if !ok {
		return
	}
	if it.Creator.ID == accID {
		s.answer(w, nil, errCreatorOwn)
		return
	}
	n, err := s.db.LibrarySaveCount(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if n >= api.MaxLibrarySaves {
		fail(w, http.StatusBadRequest, "that's as many designs as an account can like")
		return
	}
	out, err := s.db.SaveLibraryItem(r.Context(), accID, id)
	s.answer(w, out, err)
}

func (s *Service) unsave(w http.ResponseWriter, r *http.Request) {
	out, err := s.db.UnsaveLibraryItem(r.Context(), asker(r).account.ID, r.PathValue("id"))
	s.answer(w, out, err)
}

func (s *Service) saveState(w http.ResponseWriter, r *http.Request) {
	out, err := s.db.LibrarySaveState(r.Context(), asker(r).account.ID, r.PathValue("id"))
	s.answer(w, out, err)
}

func (s *Service) listSaves(w http.ResponseWriter, r *http.Request) {
	offset, ok := offsetOf(w, r)
	if !ok {
		return
	}
	items, more, err := s.db.LibrarySavedItems(r.Context(), asker(r).account.ID, limitOf(r), offset)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryItems{Items: shownAll(items), Next: nextOf(more, offset, len(items))})
}

func (s *Service) follow(w http.ResponseWriter, r *http.Request) {
	accID := asker(r).account.ID
	p, _, _, ok := s.shownCreator(w, r)
	if !ok {
		return
	}
	// One moderating may look at a hidden profile; nobody follows one.
	if p.Hidden || p.Trust == api.TrustBlocked {
		s.answer(w, nil, store.ErrNotFound)
		return
	}
	if p.AccountID == accID {
		s.answer(w, nil, errCreatorOwn)
		return
	}
	n, err := s.db.LibraryFollowCount(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if n >= api.MaxLibraryFollows {
		fail(w, http.StatusBadRequest, "that's as many creators as an account can follow")
		return
	}
	if err := s.db.FollowLibraryCreator(r.Context(), accID, p.AccountID); err != nil {
		s.answer(w, nil, err)
		return
	}
	out, err := s.db.LibraryFollowState(r.Context(), accID, p.AccountID)
	s.answer(w, out, err)
}

// unfollow works whatever the creator shows now: leaving is never refused.
func (s *Service) unfollow(w http.ResponseWriter, r *http.Request) {
	accID := asker(r).account.ID
	p, err := s.db.LibraryCreator(r.Context(), r.PathValue("handle"))
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if err := s.db.UnfollowLibraryCreator(r.Context(), accID, p.AccountID); err != nil {
		s.answer(w, nil, err)
		return
	}
	out, err := s.db.LibraryFollowState(r.Context(), accID, p.AccountID)
	s.answer(w, out, err)
}

func (s *Service) following(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.LibraryFollowing(r.Context(), asker(r).account.ID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryFollowing{Creators: list})
}

func (s *Service) feed(w http.ResponseWriter, r *http.Request) {
	offset, ok := offsetOf(w, r)
	if !ok {
		return
	}
	items, more, err := s.db.LibraryFeed(r.Context(), asker(r).account.ID, limitOf(r), offset)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryItems{Items: shownAll(items), Next: nextOf(more, offset, len(items))})
}

// collectionParams reads a collection's params, or answers. It's public text if its
// owner says, so it keeps to what a bio does.
func collectionParams(w http.ResponseWriter, r *http.Request) (api.LibraryCollectionParams, bool) {
	var p api.LibraryCollectionParams
	if !readJSON(w, r, &p) {
		return p, false
	}
	p.Title, p.Description = cleanText(p.Title, 80), cleanText(p.Description, 500)
	switch {
	case p.Title == "":
		fail(w, http.StatusBadRequest, "a collection needs a name")
		return p, false
	case emailShape.MatchString(p.Title) || emailShape.MatchString(p.Description):
		fail(w, http.StatusBadRequest, "take the email address out: enowX UI never shows one")
		return p, false
	}
	return p, true
}

func (s *Service) myCollections(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.LibraryCollectionsOf(r.Context(), asker(r).account.ID, true, r.URL.Query().Get("item"))
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryCollections{Collections: list})
}

func (s *Service) addCollection(w http.ResponseWriter, r *http.Request) {
	p, ok := collectionParams(w, r)
	if !ok {
		return
	}
	id, err := s.db.AddLibraryCollection(r.Context(), asker(r).account.ID, p)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	c, err := s.db.LibraryCollectionRow(r.Context(), id)
	s.answer(w, c, err)
}

// getCollection answers a collection to whom it's for: its owner always; anyone else
// only when it's public and its owner shows. A private one is no one else's business,
// a moderator's included.
func (s *Service) getCollection(w http.ResponseWriter, r *http.Request) {
	c, err := s.db.LibraryCollectionRow(r.Context(), r.PathValue("id"))
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	accID, _ := s.viewer(r)
	own := accID != "" && accID == c.Owner.ID
	if !own {
		unlisted, err := s.db.LibraryOwnerUnlisted(r.Context(), c.Owner.ID)
		if err != nil {
			s.answer(w, nil, err)
			return
		}
		if !c.Public || unlisted {
			s.answer(w, nil, store.ErrNotFound)
			return
		}
	}
	offset, ok := offsetOf(w, r)
	if !ok {
		return
	}
	items, more, err := s.db.LibraryCollectionItems(r.Context(), c.ID, own, limitOf(r), offset)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if c.Items, err = s.db.LibraryCollectionSize(r.Context(), c.ID, own); err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryCollectionPage{Collection: c, Own: own, Items: shownAll(items), Next: nextOf(more, offset, len(items))})
}

func (s *Service) setCollection(w http.ResponseWriter, r *http.Request) {
	p, ok := collectionParams(w, r)
	if !ok {
		return
	}
	s.answer(w, nil, s.db.SetLibraryCollection(r.Context(), asker(r).account.ID, r.PathValue("id"), p))
}

func (s *Service) deleteCollection(w http.ResponseWriter, r *http.Request) {
	s.answer(w, nil, s.db.DeleteLibraryCollection(r.Context(), asker(r).account.ID, r.PathValue("id")))
}

func (s *Service) collect(w http.ResponseWriter, r *http.Request) {
	item := r.PathValue("item")
	if _, ok := s.liveItem(w, r, item); !ok {
		return
	}
	s.answer(w, nil, s.db.SetLibraryCollectionItem(r.Context(), asker(r).account.ID, r.PathValue("id"), item, true))
}

func (s *Service) uncollect(w http.ResponseWriter, r *http.Request) {
	s.answer(w, nil, s.db.SetLibraryCollectionItem(r.Context(), asker(r).account.ID, r.PathValue("id"), r.PathValue("item"), false))
}
