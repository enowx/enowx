package tokens

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
)

// Days past KeepDays are kept as a day's totals per agent, model, project,
// config and own use: what a year's summary needs, without each call.

func (s *Store) rollupFile() string { return filepath.Join(s.dir, "rollups.json") }

// RolledUp returns the rows of the days rolled up, in [since, until).
func (s *Store) RolledUp(since, until time.Time) []proto.TokenDayRow {
	var all []proto.TokenDayRow
	if data, err := os.ReadFile(s.rollupFile()); err == nil {
		_ = json.Unmarshal(data, &all)
	}
	from, to := day(since), day(until)
	return slices.DeleteFunc(all, func(r proto.TokenDayRow) bool { return r.Day < from || r.Day > to })
}

// Rollup rolls up the days past KeepDays before now, priced by prices,
// and deletes their files.
func (s *Store) Rollup(now time.Time, prices *Prices) error {
	old := s.Days(time.Time{}, now.AddDate(0, 0, -KeepDays))
	if len(old) == 0 {
		return nil
	}
	var all []proto.TokenDayRow
	if data, err := os.ReadFile(s.rollupFile()); err == nil {
		_ = json.Unmarshal(data, &all)
	}
	for _, d := range old {
		s.mu.Lock()
		dc := s.loadLocked(d)
		calls := slices.Clone(dc.calls)
		s.mu.Unlock()
		all = append(all, DayRows(calls, prices)...)
	}
	data, err := json.Marshal(all)
	if err != nil {
		return err
	}
	if err := envsync.WriteFileAtomic(s.rollupFile(), data, 0o600); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range old {
		os.Remove(s.file(d))
		delete(s.days, d)
	}
	return nil
}

// DayRows sums calls per day, agent, model, project, config and own use.
func DayRows(calls []Call, prices *Prices) []proto.TokenDayRow {
	type key struct{ day, agent, model, project, config, own string }
	sums := map[key]*proto.TokenDayRow{}
	var order []key
	for _, c := range calls {
		k := key{day(c.Time), c.Agent, c.Model, c.Project, c.Config, c.Own}
		r := sums[k]
		if r == nil {
			r = &proto.TokenDayRow{Day: k.day, Agent: k.agent, Model: k.model, Project: k.project, Config: k.config, Own: k.own}
			sums[k] = r
			order = append(order, k)
		}
		r.Add(c.Counts(prices))
	}
	out := make([]proto.TokenDayRow, 0, len(order))
	for _, k := range order {
		out = append(out, *sums[k])
	}
	slices.SortStableFunc(out, func(a, b proto.TokenDayRow) int {
		if a.Day < b.Day {
			return -1
		}
		if a.Day > b.Day {
			return 1
		}
		return 0
	})
	return out
}
