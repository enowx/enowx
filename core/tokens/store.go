package tokens

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// KeepDays is how long each call is kept; older days are rolled up into
// a day's totals per agent, model, project and config.
const KeepDays = 366

// Store keeps a host's calls in dir, a JSON lines file a day
// (2006-01-02.jsonl), and the rolled-up days in rollups.json. Safe from
// any goroutine; mu guards the cache of days read.
type Store struct {
	dir string
	// Now is the time calls are kept up to KeepDays before (tests set it).
	Now func() time.Time

	mu   sync.Mutex
	days map[string]*dayCalls
}

// dayCalls is a day's calls as read, and their IDs.
type dayCalls struct {
	calls []Call
	ids   map[string]bool
}

// Open opens the store in dir, made when needed.
func Open(dir string) *Store { return &Store{dir: dir, Now: time.Now, days: map[string]*dayCalls{}} }

// Dir is where the store keeps its files.
func (s *Store) Dir() string { return s.dir }

func (s *Store) file(d string) string { return filepath.Join(s.dir, d+".jsonl") }

// loadLocked reads day d into the cache. mu is held.
func (s *Store) loadLocked(d string) *dayCalls {
	if dc := s.days[d]; dc != nil {
		return dc
	}
	dc := &dayCalls{ids: map[string]bool{}}
	if f, err := os.Open(s.file(d)); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var c Call
			if json.Unmarshal(sc.Bytes(), &c) == nil && c.ID != "" && !dc.ids[c.ID] {
				dc.ids[c.ID] = true
				dc.calls = append(dc.calls, c)
			}
		}
		f.Close()
	}
	s.days[d] = dc
	return dc
}

// Add keeps the calls not kept yet (by ID, in their day) and returns
// them. Calls past KeepDays aren't kept.
func (s *Store) Add(calls []Call) ([]Call, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldest := s.Now().AddDate(0, 0, -KeepDays)
	byDay := map[string][]Call{}
	var added []Call
	for _, c := range calls {
		if c.ID == "" || c.Empty() || c.Time.Before(oldest) {
			continue
		}
		d := day(c.Time)
		dc := s.loadLocked(d)
		if dc.ids[c.ID] {
			continue
		}
		dc.ids[c.ID] = true
		dc.calls = append(dc.calls, c)
		byDay[d] = append(byDay[d], c)
		added = append(added, c)
	}
	if len(byDay) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return added, fmt.Errorf("tokens: %w", err)
	}
	for d, list := range byDay {
		var b strings.Builder
		for _, c := range list {
			line, _ := json.Marshal(c)
			b.Write(line)
			b.WriteByte('\n')
		}
		f, err := os.OpenFile(s.file(d), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return added, fmt.Errorf("tokens: write %s: %w", d, err)
		}
		_, err = f.WriteString(b.String())
		f.Close()
		if err != nil {
			return added, fmt.Errorf("tokens: write %s: %w", d, err)
		}
	}
	return added, nil
}

// Calls returns the calls of [since, until), oldest day first.
func (s *Store) Calls(since, until time.Time) []Call {
	var out []Call
	for _, d := range s.Days(since, until) {
		s.mu.Lock()
		dc := s.loadLocked(d)
		for _, c := range dc.calls {
			if !c.Time.Before(since) && c.Time.Before(until) {
				out = append(out, c)
			}
		}
		s.mu.Unlock()
	}
	return out
}

// Days lists the days with a file in [since, until).
func (s *Store) Days(since, until time.Time) []string {
	from, to := day(since), day(until)
	entries, _ := os.ReadDir(s.dir)
	var out []string
	for _, e := range entries {
		d, ok := strings.CutSuffix(e.Name(), ".jsonl")
		if ok && len(d) == len(time.DateOnly) && d >= from && d <= to {
			out = append(out, d)
		}
	}
	slices.Sort(out)
	return out
}

// Forget drops the cached days other than the last few, to free memory
// once a long range was read.
func (s *Store) Forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := day(time.Now().AddDate(0, 0, -40))
	for d := range s.days {
		if d < keep {
			delete(s.days, d)
		}
	}
}
