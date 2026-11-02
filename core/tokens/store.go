package tokens

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
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
	// where is the day each kept ID is in, by the ID's hash, built once from the day
	// files on the first Add and kept small: a call seen again on another day, a
	// resumed or copied conversation's, is the same call. Nil until then.
	where map[uint64]int32
}

// idHash is an ID as where keeps it.
func idHash(id string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(id))
	return h.Sum64()
}

// dayNumber is day d as where keeps it, and back.
func dayNumber(d string) int32 {
	t, err := time.Parse(time.DateOnly, d)
	if err != nil {
		return 0
	}
	return int32(t.Unix() / 86400)
}

func dayOfNumber(n int32) string { return time.Unix(int64(n)*86400, 0).UTC().Format(time.DateOnly) }

// loadWhereLocked builds where from the day files, reading only each line's ID, and
// leaves the days out of the cache. mu is held.
func (s *Store) loadWhereLocked() {
	s.where = map[uint64]int32{}
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		d, ok := strings.CutSuffix(e.Name(), ".jsonl")
		if !ok || len(d) != len(time.DateOnly) {
			continue
		}
		f, err := os.Open(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		n := dayNumber(d)
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var line struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(sc.Bytes(), &line) == nil && line.ID != "" {
				if _, seen := s.where[idHash(line.ID)]; !seen {
					s.where[idHash(line.ID)] = n
				}
			}
		}
		f.Close()
	}
}

// dayCalls is a day's calls as read, and where each ID is among them.
type dayCalls struct {
	calls []Call
	at    map[string]int
}

// grow takes what later record o of call c says it counted: the largest of each
// field, as an agent may write one call over several records whose counts grow as it
// streams. It says whether c changed.
func (c *Call) grow(o Call) bool {
	before := *c
	c.Input, c.CacheRead, c.CacheWrite = max(c.Input, o.Input), max(c.CacheRead, o.CacheRead), max(c.CacheWrite, o.CacheWrite)
	c.Output, c.Reasoning, c.Cost = max(c.Output, o.Output), max(c.Reasoning, o.Reasoning), max(c.Cost, o.Cost)
	c.CacheWrite1h = max(c.CacheWrite1h, o.CacheWrite1h)
	return *c != before
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
	dc := &dayCalls{at: map[string]int{}}
	if f, err := os.Open(s.file(d)); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var c Call
			if json.Unmarshal(sc.Bytes(), &c) != nil || c.ID == "" {
				continue
			}
			if i, seen := dc.at[c.ID]; seen {
				dc.calls[i].grow(c) // a later line corrects an earlier one
				continue
			}
			dc.at[c.ID] = len(dc.calls)
			dc.calls = append(dc.calls, c)
		}
		f.Close()
	}
	s.days[d] = dc
	return dc
}

// Add keeps the calls not kept yet (by ID, on whichever day each was kept), corrects
// those a later record grew (a corrected line after the first, which reading merges), and returns
// both. Calls past KeepDays aren't kept.
func (s *Store) Add(calls []Call) ([]Call, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.where == nil {
		s.loadWhereLocked()
	}
	oldest := s.Now().AddDate(0, 0, -KeepDays)
	byDay := map[string][]Call{}
	var added []Call
	for _, c := range calls {
		if c.ID == "" || c.Empty() || c.Time.Before(oldest) {
			continue
		}
		d := day(c.Time)
		h := idHash(c.ID)
		if n, known := s.where[h]; known {
			// A hash can collide, and a day can be rolled up: its day must hold the ID.
			if k := dayOfNumber(n); k != d {
				if _, here := s.loadLocked(k).at[c.ID]; here {
					d = k
				}
			}
		} else {
			s.where[h] = dayNumber(d)
		}
		dc := s.loadLocked(d)
		if i, seen := dc.at[c.ID]; seen {
			if dc.calls[i].grow(c) {
				byDay[d] = append(byDay[d], dc.calls[i])
				added = append(added, dc.calls[i])
			}
			continue
		}
		dc.at[c.ID] = len(dc.calls)
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
