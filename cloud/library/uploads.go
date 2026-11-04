package library

import (
	"net/http"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// What one account may ask of the bucket and this server when sending designs, so
// that it runs into its own limit long before the month's writes, which everyone
// shares, or the memory of the server teams use. Kept in this server's memory, like
// the other limits here: a restart forgets them, an attacker can't cause one.

const (
	// A design is four or five parts and one submit.
	putsPerMinute    = 40
	submitsPerMinute = 10
	// accountSends is how many of its requests an account has in here at once. It
	// holds its place while a body arrives, so slow bodies cost only their account.
	accountSends = 2
	// sendsAtOnce is how many requests, of anyone, hold a bundle in memory at once:
	// a part being read and zipped, or a design's documents being made.
	sendsAtOnce = 4
	// sendsWait is how long a request waits for one of those.
	sendsWait = 20 * time.Second
	// DefaultAccountWrittenDay is what an account may write to the bucket in a day:
	// twenty 10 MB designs and their documents, and under 2% of a month's.
	DefaultAccountWrittenDay = 512 << 20
)

// sending is what's being sent, per account, under mu.
type sending struct {
	mu      sync.Mutex
	at      map[string]int
	day     string
	written map[string]int64
}

// enter takes a place of accID's, false when it has none left.
func (l *sending) enter(accID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.at[accID] >= accountSends {
		return false
	}
	if l.at == nil {
		l.at = map[string]int{}
	}
	l.at[accID]++
	return true
}

func (l *sending) leave(accID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.at[accID]--; l.at[accID] <= 0 {
		delete(l.at, accID)
	}
}

// reserve counts n bytes written by accID today, false past limit.
func (l *sending) reserve(accID string, n, limit int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if day := time.Now().UTC().Format(time.DateOnly); day != l.day || l.written == nil {
		l.day, l.written = day, map[string]int64{}
	}
	if l.written[accID]+n > limit {
		return false
	}
	l.written[accID] += n
	return true
}

// giveBack takes n bytes that weren't written off accID's day.
func (l *sending) giveBack(accID string, n int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.written[accID] = max(l.written[accID]-n, 0)
}

// sender is what the app sends to the bucket with: its account within its limits,
// and not blocked.
func (s *Service) sender(action string, perMinute int, h http.HandlerFunc) http.HandlerFunc {
	return s.deviceOnly(func(w http.ResponseWriter, r *http.Request) {
		accID := asker(r).account.ID
		if !s.limits.allow(action+"\x00"+accID, perMinute) {
			tooMany(w, "that's a lot sent at once: wait a minute")
			return
		}
		me, err := s.db.LibraryProfile(r.Context(), accID)
		if err != nil {
			s.answer(w, nil, err)
			return
		}
		if me.Trust == api.TrustBlocked {
			fail(w, http.StatusForbidden, "this account can't publish")
			return
		}
		if !s.sends.enter(accID) {
			w.Header().Set("Retry-After", "5")
			fail(w, http.StatusTooManyRequests, "this account is sending enough at once: wait for those first")
			return
		}
		defer s.sends.leave(accID)
		h(w, r)
	})
}

// holdingMemory takes one of the places for what holds a bundle in memory, or
// answers. Its answer false means nothing else is to be said.
func (s *Service) holdingMemory(w http.ResponseWriter, r *http.Request) (release func(), ok bool) {
	timer := time.NewTimer(sendsWait)
	defer timer.Stop()
	select {
	case s.heavySends <- struct{}{}:
		return func() { <-s.heavySends }, true
	case <-timer.C:
		w.Header().Set("Retry-After", "10")
		fail(w, http.StatusServiceUnavailable, "the library is busy: try again")
	case <-r.Context().Done():
	}
	return nil, false
}

// accountWrittenDay is the day's limit of what an account writes.
func (s *Service) accountWrittenDay() int64 {
	if s.cfg.AccountWrittenDay > 0 {
		return s.cfg.AccountWrittenDay
	}
	return DefaultAccountWrittenDay
}
