package tokens

import (
	"math"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A range that starts or ends partway through a day kept summed counts only
// the calls inside it: a day is taken whole only between the range's ends,
// and those two are read call by call, even once the day is summed.
func TestAColdDaysRangeIsExact(t *testing.T) {
	d0 := time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local) // ten days back: summed, not held
	now := d0.AddDate(0, 0, 10).Add(12 * time.Hour)
	s := Open(t.TempDir())
	s.Now = func() time.Time { return now }
	calls := []Call{
		{ID: "a", Time: d0.Add(time.Hour), Agent: "claude", Input: 100, Layer: proto.TokenLayerRecords},
		{ID: "b", Time: d0.Add(2 * time.Hour), Agent: "claude", Input: 200, Layer: proto.TokenLayerRecords},
		{ID: "c", Time: d0.Add(3 * time.Hour), Agent: "claude", Input: 400, Layer: proto.TokenLayerRecords},
	}
	if _, err := s.Add(calls); err != nil {
		t.Fatal(err)
	}
	prices := NewPrices(nil)
	// A range past the day on both ends keeps it summed.
	wide := s.Summarize(proto.TokensQuery{Since: d0.AddDate(0, 0, -1), Until: d0.AddDate(0, 0, 2)}, prices, now)
	if wide.Total.Input != 700 || wide.Total.Calls != 3 {
		t.Fatalf("the day summed: %+v", wide.Total)
	}
	s.mu.Lock()
	dc := s.days[day(d0)]
	summed := dc != nil && dc.calls == nil && len(dc.rows) > 0
	s.mu.Unlock()
	if !summed {
		t.Fatal("the day was kept as calls, not summed")
	}

	whole := s.Summarize(proto.TokensQuery{Since: d0, Until: d0.AddDate(0, 0, 1)}, prices, now)
	if whole.Total.Input != 700 || whole.Total.Calls != 3 {
		t.Fatalf("the whole day: %+v", whole.Total)
	}
	in := s.Summarize(proto.TokensQuery{Since: d0.Add(90 * time.Minute), Until: d0.Add(150 * time.Minute)}, prices, now)
	if in.Total.Input != 200 || in.Total.Calls != 1 {
		t.Fatalf("inside the range: %+v", in.Total)
	}
	after := s.Summarize(proto.TokensQuery{Since: d0.Add(150 * time.Minute), Until: d0.AddDate(0, 0, 1)}, prices, now)
	if after.Total.Input != 400 || after.Total.Calls != 1 {
		t.Fatalf("after the range: %+v", after.Total)
	}
}

// same sums says counts a and b are the same: the figures exactly, the costs to
// the last bit a float of this size can be trusted with, as sums added in a
// different order may land a rounding step apart.
func sameSums(a, b proto.TokenCounts) bool {
	const eps = 1e-9
	return a.Input == b.Input && a.CacheRead == b.CacheRead && a.CacheWrite == b.CacheWrite &&
		a.Output == b.Output && a.Reasoning == b.Reasoning && a.Calls == b.Calls &&
		a.Unpriced == b.Unpriced && a.Estimated == b.Estimated &&
		math.Abs(a.Cost-b.Cost) < eps && math.Abs(a.Equivalent-b.Equivalent) < eps
}

// Days summed once add up to exactly what their calls do, for every split a
// summary offers, for the day rows a host syncs and for a session's badge:
// the summing loses the calls, not a figure.
func TestSummedDaysAddUpToTheirCalls(t *testing.T) {
	now := time.Now().Truncate(time.Hour)
	s := Open(t.TempDir())
	s.Now = func() time.Time { return now }
	prices := NewPrices([]proto.TokenPrice{{Model: "claude-opus-5-5", Input: 15, CacheRead: 1.5, CacheWrite: 18.75, Output: 75}})
	calls := []Call{
		{ID: "a", Time: now.AddDate(0, 0, -12).Add(time.Hour), Agent: "claude", Session: "s1", Model: "claude-opus-5-5", Project: "/p", Config: "work", Account: "me", Input: 1000, CacheRead: 2000, Output: 300, Layer: proto.TokenLayerRecords},
		{ID: "b", Time: now.AddDate(0, 0, -12).Add(2 * time.Hour), Agent: "codex", Session: "s2", Model: "gpt-6-luna", Project: "/q", Input: 4000, CacheWrite: 500, Output: 600, Layer: proto.TokenLayerRecords},
		{ID: "c", Time: now.AddDate(0, 0, -6).Add(3 * time.Hour), Agent: "claude", Session: "s1", Model: "claude-opus-5-5", Project: "/p", Config: "work", Account: "me", Input: 7000, Output: 800, Plan: true, Layer: proto.TokenLayerRecords},
		{ID: "d", Time: now.AddDate(0, 0, -1).Add(4 * time.Hour), Agent: "pi", Session: "s3", Model: "local-model", Project: "/q", Provider: "local", Input: 90, Output: 10, Layer: proto.TokenLayerTraffic},
		{ID: "e", Time: now.Add(-2 * time.Hour), Agent: "claude", Session: "s1", Model: "claude-opus-5-5", Project: "/p", Config: "work", Account: "me", Input: 11, Output: 22, Layer: proto.TokenLayerRecords},
		{ID: "own", Time: now.Add(-3 * time.Hour), Own: proto.TokensOwnJudge, Model: "small-model", Input: 33, Output: 44, Layer: proto.TokenLayerTraffic},
	}
	if _, err := s.Add(calls); err != nil {
		t.Fatal(err)
	}
	since, until := now.AddDate(0, 0, -30), now.Add(time.Minute)

	// What the sums should be, straight from the calls.
	total := proto.TokenCounts{}
	own := proto.TokenCounts{}
	by := map[string]map[string]proto.TokenCounts{}
	days := map[string]proto.TokenCounts{}
	dayRows := map[[6]string]proto.TokenCounts{}
	sessions := map[string]proto.TokenCounts{}
	splits := []string{proto.TokensByAgent, proto.TokensByModel, proto.TokensByProject, proto.TokensBySession, proto.TokensByConfig, proto.TokensByAccount, proto.TokensByLayer}
	addTo := func(m map[string]proto.TokenCounts, key string, n proto.TokenCounts) {
		got := m[key]
		got.Add(n)
		m[key] = got
	}
	for _, c := range s.Calls(since, until) {
		n := c.Counts(prices)
		// The day rows a host syncs keep enowX's own use too, the summary apart.
		k := [6]string{day(c.Time), c.Agent, c.Model, c.Project, c.Config, c.Own}
		got := dayRows[k]
		got.Add(n)
		dayRows[k] = got
		if c.Own != "" {
			own.Add(n)
			continue
		}
		total.Add(n)
		addTo(days, day(c.Time), n)
		if c.Session != "" {
			addTo(sessions, c.Session, n)
		}
		r := rowOf(c, prices)
		for _, split := range splits {
			if by[split] == nil {
				by[split] = map[string]proto.TokenCounts{}
			}
			addTo(by[split], r.key(split), n)
		}
	}

	for _, split := range splits {
		sum := s.Summarize(proto.TokensQuery{Since: since, Until: until, By: split}, prices, now)
		if !sameSums(sum.Total, total) {
			t.Fatalf("%s: total %+v, from the calls %+v", split, sum.Total, total)
		}
		if len(sum.Groups) != len(by[split]) {
			t.Fatalf("%s: %d groups, from the calls %d: %+v", split, len(sum.Groups), len(by[split]), sum.Groups)
		}
		for _, g := range sum.Groups {
			if want := by[split][g.Key]; !sameSums(g.TokenCounts, want) {
				t.Fatalf("%s: %q is %+v, from the calls %+v", split, g.Key, g.TokenCounts, want)
			}
		}
		if len(sum.Days) != len(days) {
			t.Fatalf("%s: %d days, from the calls %d", split, len(sum.Days), len(days))
		}
		for _, d := range sum.Days {
			if want := days[d.Day]; !sameSums(d.TokenCounts, want) {
				t.Fatalf("%s: day %s is %+v, from the calls %+v", split, d.Day, d.TokenCounts, want)
			}
		}
	}
	if o := s.Summarize(proto.TokensQuery{Since: since, Until: until}, prices, now); !sameSums(o.Own, own) {
		t.Fatalf("own use %+v, from the calls %+v", o.Own, own)
	}

	rows := s.DayRows(since, until, prices)
	if len(rows) != len(dayRows) {
		t.Fatalf("%d day rows, from the calls %d: %+v", len(rows), len(dayRows), rows)
	}
	for _, r := range rows {
		k := [6]string{r.Day, r.Agent, r.Model, r.Project, r.Config, r.Own}
		if want := dayRows[k]; !sameSums(r.TokenCounts, want) {
			t.Fatalf("day row %v is %+v, from the calls %+v", k, r.TokenCounts, want)
		}
	}
	totals := s.SessionTotals(since, prices)
	if len(totals) != len(sessions) {
		t.Fatalf("%d sessions, from the calls %d: %+v", len(totals), len(sessions), totals)
	}
	for _, st := range totals {
		if want := sessions[st.Session]; !sameSums(st.TokenCounts, want) {
			t.Fatalf("session %q is %+v, from the calls %+v", st.Session, st.TokenCounts, want)
		}
	}
}
