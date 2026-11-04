package library

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

// A creator's numbers. A view reaches this server as a number per item, added up
// by the website over a minute: this side never hears of a person, an address or
// a cookie (docs/library.md). Reuses are counted here and written the same way, a
// short transaction now and then, so neither weighs on the teams' sync.

// countsEvery is how often counts held in memory are written.
const countsEvery = time.Minute

// Most a website's report says at once, past which it's refused or cut.
const (
	maxViewsItems = 1000
	maxViewsEach  = 100000
)

// tally holds what's counted until written, under its mutex; a restart loses
// what's held, at most a minute of counting.
type tally struct {
	mu   sync.Mutex
	held map[string]store.LibraryCount
}

func (t *tally) add(id string, reuses int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.held == nil {
		t.held = map[string]store.LibraryCount{}
	}
	c := t.held[id]
	c.Reuses += reuses
	t.held[id] = c
}

func (t *tally) take() map[string]store.LibraryCount {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.held
	t.held = nil
	return out
}

// countReuse counts someone taking a copy of item id's design: its prompt, its
// code, its source or its skill. Anything serving a copy calls this.
func (s *Service) countReuse(id string) { s.counts.add(id, 1) }

// writeCounts writes what's held, keeping it for next time if that fails.
func (s *Service) writeCounts(ctx context.Context) {
	held := s.counts.take()
	if err := s.db.AddLibraryCounts(ctx, held); err != nil {
		s.log.Warn("library: counts", "err", err)
		for id, c := range held {
			s.counts.add(id, c.Reuses)
		}
	}
}

// keepWritingCounts goes until ctx ends, and writes once more on its way out.
func (s *Service) keepWritingCounts(ctx context.Context) {
	tick := time.NewTicker(countsEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			last, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.writeCounts(last)
			return
		case <-tick.C:
			s.writeCounts(ctx)
		}
	}
}

// addViews takes the website's count of views. It holds a place among the reads
// at work, so that it can't take the connections the teams' sync needs: refused,
// the website keeps its numbers for the next try.
func (s *Service) addViews(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryViews
	if !readJSON(w, r, &p) {
		return
	}
	if len(p.Views) > maxViewsItems {
		fail(w, http.StatusBadRequest, "that's more items than one report takes")
		return
	}
	select {
	case s.reads <- struct{}{}:
		defer func() { <-s.reads }()
	case <-time.After(2 * time.Second):
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusServiceUnavailable, "the library is busy: try again")
		return
	case <-r.Context().Done():
		return
	}
	counts := make(map[string]store.LibraryCount, len(p.Views))
	for id, n := range p.Views {
		if n > 0 {
			counts[id] = store.LibraryCount{Views: int64(min(n, maxViewsEach))}
		}
	}
	s.answer(w, nil, s.db.AddLibraryCounts(r.Context(), counts))
}

// stats are the asker's own items' numbers: nobody else's, moderators' included.
func (s *Service) stats(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.LibraryStats(r.Context(), asker(r).account.ID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibraryStats{Items: items})
}
