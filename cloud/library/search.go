package library

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
	"github.com/enowx/enowx/core/designshare"
)

// Searching and filtering, and what the server reads of a design for them.
// Searching costs the database more than browsing, so it has its own limit per
// address and its own time, on top of the few places all public reads share:
// the teams' sync keeps its connections.

const (
	// searchesPerMinute is how many searches an address may make.
	searchesPerMinute = 60
	// searchTime is the longest a search holds a database connection.
	searchTime = 3 * time.Second
)

// addressOf is who to count a request as: the browser the website asks for,
// whose address its secret vouches for, or the request's own.
func (s *Service) addressOf(r *http.Request) string {
	if s.fromWeb(r) {
		if f := r.Header.Get("X-Forwarded-For"); f != "" {
			return strings.TrimSpace(strings.Split(f, ",")[0])
		}
	}
	return s.host.ClientAddr(r)
}

// tooMany says an address is over a limit, and answers.
func tooMany(w http.ResponseWriter, msg string) {
	w.Header().Set("Retry-After", "60")
	fail(w, http.StatusTooManyRequests, msg)
}

// queryOf is a request's filters. Their values go to the database as parameters
// and nothing else, whatever they are; one that isn't known simply finds nothing.
func queryOf(q url.Values) store.LibraryQuery {
	return store.LibraryQuery{Q: q.Get("q"), Category: q.Get("category"), Kind: q.Get("kind"), License: q.Get("license"), Tag: q.Get("tag"), Sort: q.Get("sort"),
		Style: q.Get("style"), Color: q.Get("color"), Theme: q.Get("theme"), Device: q.Get("device"), Motion: q.Get("motion") == "1"}
}

// styleOf is a creator's pick of style, one of the list or none.
func styleOf(v string) string {
	if slices.Contains(api.LibraryStyles, v) {
		return v
	}
	return ""
}

// readTraits keeps what the server itself reads of a version being submitted: a
// design's look from its bundle, its picture's hash. A picture that can't be read
// only leaves moderators without near matches for it.
func (s *Service) readTraits(ctx context.Context, v store.LibraryVersionRow, b designshare.Bundle) error {
	look := designshare.LookOf(b.Frame)
	t := store.LibraryTraits{Theme: look.Theme, Device: look.Device}
	for _, c := range look.Colors {
		t.Colors = append(t.Colors, store.LibraryColor{Color: c.Name, Share: c.Share})
	}
	if picture, err := s.objects.Get(ctx, objectKey(v.ItemID, v.Version, api.PartThumb)); err == nil {
		t.Phash = picturePhash(picture)
	} else {
		s.log.Warn("library: picture for its hash", "item", v.ItemID, "err", err)
	}
	return s.db.SetLibraryTraits(ctx, v.ItemID, v.Version, t)
}
