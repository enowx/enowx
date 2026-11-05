package tokens

import (
	"cmp"
	"slices"
	"time"

	"github.com/enowx/enowx/proto"
)

// DefaultRange is how far back a summary looks when asked for no range.
const DefaultRange = 30 * 24 * time.Hour

// Range is q's range, the defaults filled in.
func Range(q proto.TokensQuery, now time.Time) (time.Time, time.Time) {
	until := q.Until
	if until.IsZero() {
		until = now.Add(time.Minute)
	}
	since := q.Since
	if since.IsZero() {
		since = until.Add(-DefaultRange)
	}
	return since, until
}

// row is a call or a summed day's row: what a summary is made of.
type row struct {
	day, session, agent, model, project, config, account, layer, own string
	counts                                                           proto.TokenCounts
}

// rowOf is call c as a row, priced by prices.
func rowOf(c Call, prices *Prices) row {
	return row{day(c.Time), c.Session, c.Agent, c.Model, c.Project, c.Config, c.Account, c.Layer, c.Own, c.Counts(prices)}
}

// rowsOf is calls summed by everything a summary splits by: one row a day,
// session, agent, model, project, config, account, layer and own use. A day
// past hotDays keeps a few dozen of them in place of thousands of calls, and
// every split stays exact.
func rowsOf(calls []Call, prices *Prices) []row {
	type key struct{ day, session, agent, model, project, config, account, layer, own string }
	sums := map[key]*row{}
	var order []key
	for _, c := range calls {
		k := key{day(c.Time), c.Session, c.Agent, c.Model, c.Project, c.Config, c.Account, c.Layer, c.Own}
		r := sums[k]
		if r == nil {
			one := rowOf(c, prices)
			r = &one
			sums[k] = r
			order = append(order, k)
		} else {
			r.counts.Add(c.Counts(prices))
		}
	}
	out := make([]row, 0, len(order))
	for _, k := range order {
		out = append(out, *sums[k])
	}
	return out
}

// Summarize sums the store's use as q asks, priced by prices. host
// labels the host split.
func (s *Store) Summarize(q proto.TokensQuery, prices *Prices, now time.Time) proto.TokensSummary {
	since, until := Range(q, now)
	rows := s.rows(since, until, prices)
	for _, r := range s.RolledUp(since, until) {
		rows = append(rows, row{day: r.Day, agent: r.Agent, model: r.Model, project: r.Project, config: r.Config, own: r.Own, counts: r.TokenCounts})
	}
	out := proto.TokensSummary{Since: since, Until: until, Approximate: s.Approximate(since, until)}
	sum(&out, rows, q)
	return out
}

// SummarizeRows sums synced day rows (another host's) as q asks.
func SummarizeRows(rows []proto.TokenDayRow, q proto.TokensQuery, now time.Time) proto.TokensSummary {
	since, until := Range(q, now)
	from, to := day(since), day(until)
	var list []row
	for _, r := range rows {
		if r.Day >= from && r.Day <= to {
			list = append(list, row{day: r.Day, agent: r.Agent, model: r.Model, project: r.Project, config: r.Config, own: r.Own, counts: r.TokenCounts})
		}
	}
	out := proto.TokensSummary{Since: since, Until: until}
	sum(&out, list, q)
	return out
}

func sum(out *proto.TokensSummary, rows []row, q proto.TokensQuery) {
	days := map[string]*proto.TokenDay{}
	groups := map[string]*proto.TokenGroup{}
	own := map[string]*proto.TokenGroup{}
	for _, r := range rows {
		if (q.Session != "" && r.session != q.Session) || (q.Project != "" && r.project != q.Project) ||
			(q.Agent != "" && r.agent != q.Agent) || (q.Model != "" && r.model != q.Model) || (q.Config != "" && r.config != q.Config) {
			continue
		}
		if r.own != "" {
			out.Own.Add(r.counts)
			add(own, r.own, r.counts)
			continue
		}
		out.Total.Add(r.counts)
		d := days[r.day]
		if d == nil {
			d = &proto.TokenDay{Day: r.day}
			days[r.day] = d
		}
		d.Add(r.counts)
		if q.By != "" {
			add(groups, r.key(q.By), r.counts)
		}
	}
	for _, d := range days {
		out.Days = append(out.Days, *d)
	}
	slices.SortFunc(out.Days, func(a, b proto.TokenDay) int { return cmp.Compare(a.Day, b.Day) })
	out.Groups = sorted(groups)
	out.OwnGroups = sorted(own)
}

func (r row) key(by string) string {
	switch by {
	case proto.TokensByAgent:
		return r.agent
	case proto.TokensByModel:
		return r.model
	case proto.TokensByProject:
		return r.project
	case proto.TokensBySession:
		return r.session
	case proto.TokensByConfig:
		return r.config
	case proto.TokensByAccount:
		return r.account
	case proto.TokensByLayer:
		return r.layer
	}
	return ""
}

func add(m map[string]*proto.TokenGroup, key string, c proto.TokenCounts) {
	g := m[key]
	if g == nil {
		g = &proto.TokenGroup{Key: key}
		m[key] = g
	}
	g.Add(c)
}

// sorted lists groups, the most tokens first.
func sorted(m map[string]*proto.TokenGroup) []proto.TokenGroup {
	out := make([]proto.TokenGroup, 0, len(m))
	for _, g := range m {
		out = append(out, *g)
	}
	slices.SortFunc(out, func(a, b proto.TokenGroup) int {
		if c := cmp.Compare(b.Total(), a.Total()); c != 0 {
			return c
		}
		return cmp.Compare(a.Key, b.Key)
	})
	return out
}

// SessionTotals sums each session's calls since since.
func (s *Store) SessionTotals(since time.Time, prices *Prices) []proto.TokenSessionTotal {
	sums := map[string]*proto.TokenSessionTotal{}
	for _, r := range s.rows(since, time.Now().Add(time.Minute), prices) {
		if r.session == "" || r.own != "" {
			continue
		}
		t := sums[r.session]
		if t == nil {
			t = &proto.TokenSessionTotal{Session: r.session}
			sums[r.session] = t
		}
		t.Add(r.counts)
	}
	out := make([]proto.TokenSessionTotal, 0, len(sums))
	for _, t := range sums {
		out = append(out, *t)
	}
	slices.SortFunc(out, func(a, b proto.TokenSessionTotal) int { return cmp.Compare(a.Session, b.Session) })
	return out
}
