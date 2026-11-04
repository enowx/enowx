package library

import (
	"context"
	"slices"
	"strings"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

// Near matches, for moderators.

const (
	// similarWithin is the most bits of 64 a near match differs by: a resized or
	// recompressed copy stays well inside it, different designs rarely do.
	similarWithin = 10
	// similarShown is how many near matches a moderator sees per design.
	similarShown = 5
)

// fillSimilar tells moderators which designs look close to those in the queue.
// It only informs: nothing here changes an item or decides anything.
func (s *Service) fillSimilar(ctx context.Context, items []api.LibraryQueueItem) error {
	if len(items) == 0 {
		return nil
	}
	hashes, err := s.db.LibraryHashes(ctx)
	if err != nil {
		return err
	}
	for i := range items {
		it := &items[i]
		at := slices.IndexFunc(hashes, func(h store.LibraryHash) bool { return h.ItemID == it.ID && h.Version == it.Version.Version })
		if at < 0 {
			continue
		}
		it.SimilarChecked = true
		it.Similar = closestTo(hashes[at], hashes)
	}
	return nil
}

// closestTo are the near matches of h among all, the closest first, one per
// other item.
func closestTo(h store.LibraryHash, all []store.LibraryHash) []api.LibrarySimilar {
	best := map[string]api.LibrarySimilar{}
	for _, o := range all {
		if o.ItemID == h.ItemID {
			continue
		}
		d, ok := hashDistance(h.Phash, o.Phash)
		if !ok || d > similarWithin {
			continue
		}
		if had, seen := best[o.ItemID]; !seen || d < had.Distance {
			best[o.ItemID] = api.LibrarySimilar{ItemID: o.ItemID, Slug: o.Slug, Title: o.Title, Creator: o.Handle, Status: o.Status, Version: o.Version, SameCreator: o.Owner == h.Owner, Distance: d}
		}
	}
	out := make([]api.LibrarySimilar, 0, len(best))
	for _, m := range best {
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b api.LibrarySimilar) int {
		if a.Distance != b.Distance {
			return a.Distance - b.Distance
		}
		return strings.Compare(a.ItemID, b.ItemID)
	})
	return out[:min(similarShown, len(out))]
}
