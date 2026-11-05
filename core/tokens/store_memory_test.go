package tokens

import (
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A day past hotDays is kept summed: its calls aren't held, and what it adds
// up to is the same as when they were.
func TestOldDaysAreSummedNotHeld(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -10)
	s := Open(dir)
	s.Now = func() time.Time { return now }
	calls := []Call{
		{ID: "claude:a", Time: old, Agent: "claude", Project: "/p", Model: "claude-opus-5-5", Input: 1000, Output: 100, Layer: proto.TokenLayerRecords},
		{ID: "claude:b", Time: old.Add(time.Hour), Agent: "claude", Project: "/p", Model: "claude-opus-5-5", Input: 2000, Output: 200, Layer: proto.TokenLayerRecords},
		{ID: "codex:c", Time: old.Add(2 * time.Hour), Agent: "codex", Project: "/p", Model: "gpt-6-luna", Input: 500, Layer: proto.TokenLayerRecords},
	}
	if _, err := s.Add(calls); err != nil {
		t.Fatal(err)
	}
	day := day(old)
	s.mu.Lock()
	dc := s.days[day]
	held := dc != nil && dc.calls != nil
	s.mu.Unlock()
	if held {
		t.Fatal("a day ten days old holds its calls after Add")
	}

	sum := s.Summarize(proto.TokensQuery{By: proto.TokensByAgent, Until: now}, NewPrices(nil), now)
	if sum.Total.Input != 3500 || sum.Total.Output != 300 || sum.Total.Calls != 3 {
		t.Fatalf("total: %+v", sum.Total)
	}
	if len(sum.Groups) != 2 || sum.Groups[0].Key != "claude" || sum.Groups[0].Input != 3000 {
		t.Fatalf("groups: %+v", sum.Groups)
	}
	if len(sum.Days) != 1 || sum.Days[0].Day != day {
		t.Fatalf("days: %+v", sum.Days)
	}

	s.mu.Lock()
	dc = s.days[day]
	held = dc != nil && dc.calls != nil
	summed := dc != nil && len(dc.rows) > 0
	s.mu.Unlock()
	if held {
		t.Fatal("a day ten days old held its calls to be summed")
	}
	if !summed {
		t.Fatal("a day ten days old wasn't summed")
	}

	// What it adds up to doesn't change as the price table or the range does.
	again := Open(dir)
	again.Now = func() time.Time { return now }
	second := again.Summarize(proto.TokensQuery{By: proto.TokensByProject, Until: now}, NewPrices([]proto.TokenPrice{{Model: "claude-opus-5-5", Input: 1, Output: 2}}), now)
	if second.Total.Calls != 3 || second.Total.Input != 3500 {
		t.Fatalf("reopened: %+v", second.Total)
	}
}

// The calls of the last days stay held, where a scan and a badge read them
// again and again.
func TestRecentDaysAreHeld(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	s := Open(t.TempDir())
	s.Now = func() time.Time { return now }
	at := now.Add(-2 * time.Hour)
	if _, err := s.Add([]Call{{ID: "a", Time: at, Agent: "claude", Input: 10, Layer: proto.TokenLayerRecords}}); err != nil {
		t.Fatal(err)
	}
	s.Summarize(proto.TokensQuery{Until: now}, NewPrices(nil), now)
	s.mu.Lock()
	dc := s.days[day(at)]
	held := dc != nil && len(dc.calls) == 1 && dc.rows == nil
	s.mu.Unlock()
	if !held {
		t.Fatal("today's calls aren't held")
	}
}

// A call on a day past hotDays still counts once when its conversation is
// resumed days later and the agent writes it again on today's file.
func TestACallCopiedToTodayStillCountsOnce(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	old := now.AddDate(0, 0, -10)
	s := Open(dir)
	s.Now = func() time.Time { return now }
	first := Call{ID: "claude:m1", Time: old, Agent: "claude", Output: 10, Layer: proto.TokenLayerRecords}
	if _, err := s.Add([]Call{first}); err != nil {
		t.Fatal(err)
	}
	s = Open(dir) // a new run, where nothing is held
	s.Now = func() time.Time { return now }
	copied := first
	copied.Time, copied.Output = now.Add(-time.Hour), 40
	if got, _ := s.Add([]Call{copied}); len(got) != 1 || got[0].Output != 40 {
		t.Fatalf("grown: %+v", got)
	}
	sum := Open(dir).Summarize(proto.TokensQuery{Until: now.Add(time.Hour)}, NewPrices(nil), now)
	if sum.Total.Calls != 1 || sum.Total.Output != 40 {
		t.Fatalf("counted twice: %+v", sum.Total)
	}
}
