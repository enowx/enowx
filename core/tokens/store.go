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

// hotDays is how many of the latest days keep their calls in memory: the ones
// a scan writes to, and the ones a summary reads again and again. Older days
// are kept summed by everything a summary splits by (day, session, agent,
// model, project, config, account, layer, own use), which is a few dozen rows
// a day rather than thousands of calls: a year of them costs megabytes and not
// hundreds (docs/performance.md).
const hotDays = 2

// whereDays is how far back a call is remembered by its ID, so that one kept
// again on another day (a resumed or copied conversation's) counts once. A
// call older than that, written again on a day of its own, is counted once
// more, and grows it to the same figures: the record of every ID a year long
// would cost more than that does.
const whereDays = 40

// Store keeps a host's calls in dir, a JSON lines file a day
// (2006-01-02.jsonl), and the rolled-up days in rollups.json. Safe from
// any goroutine; mu guards the cache of days read.
type Store struct {
	dir string
	// Now is the time calls are kept up to KeepDays before (tests set it).
	Now func() time.Time

	mu sync.Mutex
	// days is what was read of each day: its calls while it is one of the
	// last hotDays, and them summed once it isn't.
	days map[string]*dayCalls
	// where is the day each kept ID is in, by the ID's hash, built from the
	// day files on the first Add. Nil until then.
	where map[uint64]int32
	// pruned is the day where was last cut back to whereDays.
	pruned string
}

// dayCalls is a day as the store keeps it: its calls and where each ID is
// among them while it is hot, and them summed once it isn't.
type dayCalls struct {
	calls []Call
	at    map[string]int
	// rows is the day summed by everything a summary splits by, kept for a
	// day past hotDays in place of its calls.
	rows []row
	// priced is the price table rows were summed with (Prices.Key).
	priced string
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

// hot says day d keeps its calls in memory: one of the last hotDays.
func (s *Store) hot(d string) bool { return d >= day(s.Now().AddDate(0, 0, -hotDays)) }

// idHash is an ID as where keeps it: eight bytes an ID rather than the ID.
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

// loadWhereLocked builds where from the day files of the last whereDays,
// reading each line's ID only. mu is held.
func (s *Store) loadWhereLocked() {
	s.where = map[uint64]int32{}
	oldest := day(s.Now().AddDate(0, 0, -whereDays))
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		d, ok := strings.CutSuffix(e.Name(), ".jsonl")
		if !ok || len(d) != len(time.DateOnly) || d < oldest {
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
				h := idHash(line.ID)
				if _, seen := s.where[h]; !seen {
					s.where[h] = n
				}
			}
		}
		f.Close()
	}
	s.pruned = day(s.Now())
}

// pruneWhereLocked cuts where back to the last whereDays, once a day. mu is
// held.
func (s *Store) pruneWhereLocked(today string) {
	s.pruned = today
	oldest := dayNumber(day(s.Now().AddDate(0, 0, -whereDays)))
	for h, n := range s.where {
		if n < oldest {
			delete(s.where, h)
		}
	}
}

// readDayCalls is the calls kept in file, each ID once, a later line
// correcting an earlier one.
func readDayCalls(file string) *dayCalls {
	dc := &dayCalls{at: map[string]int{}}
	f, err := os.Open(file)
	if err != nil {
		return dc
	}
	defer f.Close()
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
	return dc
}

// dayLocked is day d as the cache has it: its calls while it is hot, its rows
// once it isn't. mu is held.
func (s *Store) dayLocked(d string, prices *Prices) *dayCalls {
	key := prices.Key()
	if dc := s.days[d]; dc != nil && (dc.calls != nil || dc.priced == key) {
		return dc
	}
	dc := readDayCalls(s.file(d))
	dc.priced = key
	if !s.hot(d) {
		dc.rows, dc.calls, dc.at = rowsOf(dc.calls, prices), nil, nil
	}
	s.days[d] = dc
	return dc
}

// rawLocked is day d's calls: from the cache while it is hot, read and let go
// once it isn't. mu is held.
func (s *Store) rawLocked(d string) []Call {
	if dc := s.days[d]; dc != nil && dc.calls != nil {
		return dc.calls
	}
	dc := readDayCalls(s.file(d))
	if s.hot(d) {
		s.days[d] = dc // a hot day is held, as a scan and a summary read it again
	}
	return dc.calls
}

// addLocked is day d as Add needs it: its calls and where each ID is among
// them, read from its file when the cache has only its rows, as a call may be
// corrected. mu is held.
func (s *Store) addLocked(d string) *dayCalls {
	if dc := s.days[d]; dc != nil && dc.calls != nil {
		return dc
	}
	dc := readDayCalls(s.file(d))
	s.days[d] = dc
	return dc
}

// rows is the store's use in [since, until) summed by everything a summary
// splits by. A day either end of the range falls in is read call by call, as
// the range cuts it; the days between are kept summed.
func (s *Store) rows(since, until time.Time, prices *Prices) []row {
	from, to := day(since), day(until)
	var out []row
	for _, d := range s.Days(since, until) {
		s.mu.Lock()
		var calls []Call
		if d == from || d == to {
			calls = s.rawLocked(d) // its file, when a cold day holds only its rows
		} else if dc := s.dayLocked(d, prices); dc.calls == nil {
			out = append(out, dc.rows...)
			s.mu.Unlock()
			continue
		} else {
			calls = dc.calls
		}
		for _, c := range calls {
			if !c.Time.Before(since) && c.Time.Before(until) {
				out = append(out, rowOf(c, prices))
			}
		}
		s.mu.Unlock()
	}
	return out
}

// Add keeps the calls not kept yet (by ID, on whichever day each was kept), corrects
// those a later record grew (a corrected line after the first, which reading merges), and returns
// both. Calls past KeepDays aren't kept.
func (s *Store) Add(calls []Call) ([]Call, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.where == nil {
		s.loadWhereLocked()
	} else if today := day(s.Now()); today != s.pruned {
		s.pruneWhereLocked(today)
	}
	oldest := s.Now().AddDate(0, 0, -KeepDays)
	byDay := map[string][]Call{}
	var order []string
	var added []Call
	queue := func(d string) {
		if _, queued := byDay[d]; !queued {
			order = append(order, d)
		}
	}
	for _, c := range calls {
		if c.ID == "" || c.Empty() || c.Time.Before(oldest) {
			continue
		}
		d := day(c.Time)
		h := idHash(c.ID)
		if n, known := s.where[h]; known {
			// A hash can collide, and a day can be summed: its day must hold the ID.
			if k := dayOfNumber(n); k != d {
				if slices.ContainsFunc(s.rawLocked(k), func(x Call) bool { return x.ID == c.ID }) {
					d = k
				}
			}
		} else {
			s.where[h] = dayNumber(d)
		}
		dc := s.addLocked(d)
		if i, seen := dc.at[c.ID]; seen {
			if dc.calls[i].grow(c) {
				queue(d)
				byDay[d] = append(byDay[d], dc.calls[i])
				added = append(added, dc.calls[i])
			}
			continue
		}
		dc.at[c.ID] = len(dc.calls)
		dc.calls = append(dc.calls, c)
		queue(d)
		byDay[d] = append(byDay[d], c)
		added = append(added, c)
	}
	// A day past hotDays was read to be added to, and isn't held: it goes now,
	// whether it was written or not, and the next read sums it afresh.
	for d := range s.days {
		if !s.hot(d) {
			delete(s.days, d)
		}
	}
	if len(byDay) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return added, fmt.Errorf("tokens: %w", err)
	}
	for _, d := range order {
		if err := appendCalls(s.file(d), byDay[d]); err != nil {
			return added, err
		}
	}
	return added, nil
}

// appendCalls writes calls to file, one JSON line each.
func appendCalls(file string, calls []Call) error {
	var b strings.Builder
	for _, c := range calls {
		line, _ := json.Marshal(c)
		b.Write(line)
		b.WriteByte('\n')
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("tokens: write %s: %w", filepath.Base(file), err)
	}
	_, err = f.WriteString(b.String())
	f.Close()
	if err != nil {
		return fmt.Errorf("tokens: write %s: %w", filepath.Base(file), err)
	}
	return nil
}

// Calls returns the calls of [since, until), oldest day first. A day past
// hotDays is read from its file and let go, not held.
func (s *Store) Calls(since, until time.Time) []Call {
	var out []Call
	for _, d := range s.Days(since, until) {
		s.mu.Lock()
		for _, c := range s.rawLocked(d) {
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

// Forget drops the days whose calls aren't held, to free memory once a long
// range was read.
func (s *Store) Forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for d := range s.days {
		if !s.hot(d) {
			delete(s.days, d)
		}
	}
}
