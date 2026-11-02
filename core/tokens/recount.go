package tokens

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
)

// RecountVersion names how calls are counted. When it rises, what is stored is rebuilt
// from the agents' records once, on a new build's start: each call's final counts in
// place of its first record's, once across days.
const RecountVersion = 1

// recounted is what the last recount left, kept in recount.json: done last, so one cut
// short runs again. Approximate are the days that kept calls whose records are gone.
type recounted struct {
	Version     int      `json:"version"`
	Approximate []string `json:"approximate,omitempty"`
}

func (s *Store) recountFile() string { return filepath.Join(s.dir, "recount.json") }

func (s *Store) readRecounted() recounted {
	var r recounted
	if data, err := os.ReadFile(s.recountFile()); err == nil {
		_ = json.Unmarshal(data, &r)
	}
	return r
}

// NeedsRecount says what is stored was counted by an older build.
func (s *Store) NeedsRecount() bool { return s.readRecounted().Version < RecountVersion }

// Approximate says a day of [since, until) kept calls whose records are gone: its
// numbers are as they were counted then.
func (s *Store) Approximate(since, until time.Time) bool {
	from, to := day(since), day(until)
	return slices.ContainsFunc(s.readRecounted().Approximate, func(d string) bool { return d >= from && d <= to })
}

// backupForRecount copies the store's files aside, once: a recount cut short and run
// again leaves the first copy, the one from before any of it.
func (s *Store) backupForRecount() error {
	dest := filepath.Join(s.dir, fmt.Sprintf("before-recount-%d", RecountVersion))
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	tmp := dest + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !(strings.HasSuffix(name, ".jsonl") || name == "rollups.json") {
			continue
		}
		if err := copyFile(filepath.Join(s.dir, name), filepath.Join(tmp, name)); err != nil {
			return err
		}
	}
	return os.Rename(tmp, dest)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Recount rebuilds the day files from fresh, every call as read from the agents' records
// now, tied to its session. A call that was kept keeps its day and who it was tied to,
// which a closed session can't say again. What fresh doesn't have stays as it was: enowX's
// own use, what was metered or read from a screen, and calls whose records are gone, whose
// days are marked approximate. Rolled-up days aren't touched. The files are copied aside
// first, and the version is written last, so this runs once.
func (s *Store) Recount(fresh []Call) error {
	if err := s.backupForRecount(); err != nil {
		return fmt.Errorf("tokens: back up before the recount: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	oldest := s.Now().AddDate(0, 0, -KeepDays)
	rolled := map[string]bool{}
	var rows []proto.TokenDayRow
	if data, err := os.ReadFile(s.rollupFile()); err == nil {
		_ = json.Unmarshal(data, &rows)
	}
	for _, r := range rows {
		rolled[r.Day] = true
	}
	// What is stored, once per ID, the earliest day's where a call was kept on two.
	var dayNames []string
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		if d, ok := strings.CutSuffix(e.Name(), ".jsonl"); ok && len(d) == len(time.DateOnly) {
			dayNames = append(dayNames, d)
		}
	}
	slices.Sort(dayNames)
	old := map[string]*Call{}
	var order []string
	for _, d := range dayNames {
		for _, c := range s.loadLocked(d).calls {
			if k := old[c.ID]; k != nil {
				k.grow(c)
				continue
			}
			copied := c
			old[c.ID] = &copied
			order = append(order, c.ID)
		}
	}
	// What the records say now, on the stored call's day and with its ties.
	byDay := map[string][]Call{}
	done := map[string]*Call{}
	var fromFresh []*Call
	for _, c := range fresh {
		if c.ID == "" || c.Empty() || c.Time.Before(oldest) {
			continue
		}
		if k := done[c.ID]; k != nil {
			k.grow(c)
			continue
		}
		copied := c
		if o := old[c.ID]; o != nil {
			copied.Time = o.Time
			copied.grow(*o)
			copied.Session, copied.Project = orStr(o.Session, copied.Session), orStr(o.Project, copied.Project)
			copied.Account, copied.Config = orStr(o.Account, copied.Account), orStr(o.Config, copied.Config)
		} else if rolled[day(c.Time)] {
			continue // that day's totals are kept already
		}
		done[c.ID] = &copied
		fromFresh = append(fromFresh, &copied)
	}
	for _, c := range fromFresh {
		byDay[day(c.Time)] = append(byDay[day(c.Time)], *c)
	}
	approximate := map[string]bool{}
	for _, id := range order {
		if done[id] != nil {
			continue
		}
		c := *old[id]
		d := day(c.Time)
		byDay[d] = append(byDay[d], c)
		if c.Own == "" && (c.Layer == "" || c.Layer == proto.TokenLayerRecords) {
			approximate[d] = true
		}
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("tokens: %w", err)
	}
	for _, d := range dayNames {
		if _, kept := byDay[d]; !kept {
			byDay[d] = nil // all it held counts on an earlier day: emptied, not left to count twice
		}
	}
	for d, list := range byDay {
		slices.SortStableFunc(list, func(a, b Call) int { return a.Time.Compare(b.Time) })
		var b strings.Builder
		for _, c := range list {
			line, _ := json.Marshal(c)
			b.Write(line)
			b.WriteByte('\n')
		}
		if err := envsync.WriteFileAtomic(s.file(d), []byte(b.String()), 0o600); err != nil {
			return fmt.Errorf("tokens: write %s: %w", d, err)
		}
	}
	s.days, s.where = map[string]*dayCalls{}, nil
	r := recounted{Version: RecountVersion}
	for d := range approximate {
		r.Approximate = append(r.Approximate, d)
	}
	slices.Sort(r.Approximate)
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return envsync.WriteFileAtomic(s.recountFile(), data, 0o600)
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
