package tokens

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// A call written over several records keeps each field's largest, whichever record
// says it, over a reopen too; records that say no more change nothing.
func TestStoreKeepsEachCallsFinalCounts(t *testing.T) {
	dir := t.TempDir()
	at := time.Now().Add(-time.Hour)
	s := Open(dir)
	first := Call{ID: "claude:m1", Time: at, Input: 3, CacheRead: 100, CacheWrite: 10, Output: 1}
	if got, err := s.Add([]Call{first}); err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	grown := first
	grown.Output = 500
	if got, _ := s.Add([]Call{grown, first}); len(got) != 1 || got[0].Output != 500 {
		t.Fatalf("grown: %+v", got)
	}
	smaller := first
	smaller.Output, smaller.CacheRead = 2, 120
	if got, _ := s.Add([]Call{smaller}); len(got) != 1 || got[0].Output != 500 || got[0].CacheRead != 120 {
		t.Fatalf("each field apart: %+v", got)
	}
	if got, _ := s.Add([]Call{grown}); len(got) != 0 {
		t.Fatalf("nothing new: %+v", got)
	}
	again := Open(dir).Calls(at.Add(-time.Minute), at.Add(time.Minute))
	if len(again) != 1 || again[0].Output != 500 || again[0].CacheRead != 120 {
		t.Fatalf("reopened: %+v", again)
	}
}

// Tokens are input, output and cache write: cache read is beside them.
func TestTotalLeavesOutCacheReads(t *testing.T) {
	c := proto.TokenCounts{Input: 1, Output: 2, CacheWrite: 4, CacheRead: 1000}
	if c.Total() != 7 {
		t.Fatalf("%d", c.Total())
	}
}

// A call seen again on another day, as a resumed conversation's copy is, counts once,
// on the day it was first kept, over a reopen too; it can still grow there.
func TestStoreCountsACallOnceAcrossDays(t *testing.T) {
	dir := t.TempDir()
	first := time.Now().Add(-72 * time.Hour)
	later := time.Now().Add(-time.Hour)
	c := Call{ID: "claude:m1", Time: first, Output: 10}
	s := Open(dir)
	if _, err := s.Add([]Call{c}); err != nil {
		t.Fatal(err)
	}
	copied := c
	copied.Time = later
	if got, _ := s.Add([]Call{copied}); len(got) != 0 {
		t.Fatalf("counted again: %+v", got)
	}
	s = Open(dir)
	copied.Output = 40
	if got, _ := s.Add([]Call{copied}); len(got) != 1 || got[0].Output != 40 {
		t.Fatalf("grown: %+v", got)
	}
	all := Open(dir).Calls(first.Add(-time.Hour), time.Now())
	if len(all) != 1 || all[0].Output != 40 || !all[0].Time.Equal(first) {
		t.Fatalf("%+v", all)
	}
}

// Codex in every shape: a record of the running total alone counts its difference from
// the one before, the first being where it starts from; cached never passes input; rate
// limits alone count nothing; a resume says a count again; a fork's copy is its parent's.
func TestCodexRecordsInEveryShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	at := func(s int) string { return time.Date(2026, 10, 20, 10, 0, s, 0, time.UTC).Format(time.RFC3339) }
	count := func(s int, last, total string) string {
		return `{"timestamp":"` + at(s) + `","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":` + last + `,"total_token_usage":` + total + `}}}` + "\n"
	}
	body := `{"timestamp":"` + at(0) + `","type":"session_meta","payload":{"id":"c1","cwd":"/w"}}` + "\n" +
		`{"timestamp":"` + at(1) + `","type":"turn_context","payload":{"model":"gpt-6-luna"}}` + "\n" +
		`{"timestamp":"` + at(2) + `","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"plan_type":"plus"}}}` + "\n" +
		count(3, `{}`, `{"input_tokens":100,"cached_input_tokens":40,"output_tokens":10,"total_tokens":110}`) +
		count(4, `{}`, `{"input_tokens":150,"cached_input_tokens":50,"output_tokens":20,"total_tokens":170}`) +
		count(5, `{}`, `{"input_tokens":150,"cached_input_tokens":50,"output_tokens":20,"total_tokens":170}`) +
		count(6, `{"input_tokens":10,"cached_input_tokens":99,"output_tokens":5,"total_tokens":15}`, `{"input_tokens":160,"cached_input_tokens":149,"output_tokens":25,"total_tokens":185}`)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	calls, err := readCodex(path, &FileState{})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("%+v", calls)
	}
	if d := calls[0]; d.Input != 40 || d.CacheRead != 10 || d.Output != 10 {
		t.Errorf("the difference: %+v", d)
	}
	if c := calls[1]; c.CacheRead != 10 || c.Input != 0 {
		t.Errorf("cached past input: %+v", c)
	}

	fork := `{"timestamp":"` + at(30) + `","type":"session_meta","payload":{"id":"c2","history_base":{"thread_id":"c1"}}}` + "\n" +
		count(6, `{"input_tokens":10,"output_tokens":5,"total_tokens":15}`, `{"input_tokens":160,"output_tokens":25,"total_tokens":185}`) +
		count(31, `{"input_tokens":20,"output_tokens":5,"total_tokens":25}`, `{"input_tokens":180,"output_tokens":30,"total_tokens":210}`)
	if err := os.WriteFile(path, []byte(fork), 0o600); err != nil {
		t.Fatal(err)
	}
	calls, err = readCodex(path, &FileState{})
	if err != nil || len(calls) != 1 || calls[0].Input != 20 {
		t.Fatalf("a fork's copy: %+v %v", calls, err)
	}
}

// A write kept for an hour costs its own rate, a long request the long one for the whole
// of it, and a model taken from an earlier record makes the cost an estimate.
func TestCostsAreExact(t *testing.T) {
	p := NewPrices(nil)
	near := func(got, want float64) bool { return got > want-1e-9 && got < want+1e-9 }
	c := Call{Provider: "anthropic", Model: "claude-opus-5-5", CacheWrite: 1_000_000, CacheWrite1h: 400_000}
	if got, ok := p.Cost(c); !ok || !near(got, 0.6*5+0.4*8) {
		t.Errorf("5m and 1h: %v %v", got, ok)
	}
	short := Call{Provider: "openai", Model: "gpt-6-luna", Input: 100_000, Output: 1000}
	if got, _ := p.Cost(short); !near(got, 0.1*0.1+0.001*0.5) {
		t.Errorf("short: %v", got)
	}
	long := Call{Provider: "openai", Model: "gpt-6-luna", Input: 100_000, CacheRead: 200_000, Output: 1000}
	if got, _ := p.Cost(long); !near(got, 0.1*0.2+0.2*0.02+0.001*0.75) {
		t.Errorf("long: %v", got)
	}
	guessed := Call{Provider: "openai", Model: "gpt-6-luna", Input: 10, ModelInferred: true}
	if n := guessed.Counts(p); n.Estimated != 1 || n.Unpriced != 0 {
		t.Errorf("%+v", n)
	}
	guessed.Cost = 1 // what the agent said it cost
	if guessed.Counts(p).Estimated != 0 {
		t.Error("the agent's own cost isn't an estimate")
	}
}

// A recount rebuilds what is stored from the records: final counts, once across days, its
// day and session kept; what the records no longer have stays and marks its day; enowX's own
// use and rolled-up days are untouched; the files are copied aside; it runs once.
func TestRecount(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	d1 := time.Now().Add(-72 * time.Hour)
	d2 := time.Now().Add(-48 * time.Hour)
	stored := []Call{
		{ID: "claude:m1", Time: d1, Output: 1, Session: "s1", Agent: "claude", Layer: proto.TokenLayerRecords},
		{ID: "claude:gone", Time: d2, Output: 7, Agent: "claude", Layer: proto.TokenLayerRecords},
		{ID: "judge:1", Time: d2, Output: 3, Own: proto.TokensOwnJudge},
	}
	if _, err := s.Add(stored); err != nil {
		t.Fatal(err)
	}
	rollups := []byte(`[{"day":"2020-01-01","agent":"claude","output":5}]`)
	if err := os.WriteFile(filepath.Join(dir, "rollups.json"), rollups, 0o600); err != nil {
		t.Fatal(err)
	}
	if !s.NeedsRecount() {
		t.Fatal("no recount asked for")
	}
	fresh := []Call{
		{ID: "claude:m1", Time: d2, Output: 50, Agent: "claude", Layer: proto.TokenLayerRecords},
		{ID: "claude:m1", Time: d2, Output: 90, Agent: "claude", Layer: proto.TokenLayerRecords},
		{ID: "claude:new", Time: d2, Output: 4, Agent: "claude", Layer: proto.TokenLayerRecords},
	}
	if err := s.Recount(fresh); err != nil {
		t.Fatal(err)
	}
	if s.NeedsRecount() {
		t.Fatal("it would run again")
	}
	byID := map[string]Call{}
	for _, c := range Open(dir).Calls(d1.Add(-time.Hour), time.Now()) {
		if _, twice := byID[c.ID]; twice {
			t.Fatalf("%s twice", c.ID)
		}
		byID[c.ID] = c
	}
	if m := byID["claude:m1"]; m.Output != 90 || m.Session != "s1" || !m.Time.Equal(d1) {
		t.Errorf("recounted: %+v", m)
	}
	if byID["claude:new"].Output != 4 || byID["claude:gone"].Output != 7 || byID["judge:1"].Output != 3 {
		t.Errorf("%+v", byID)
	}
	if !s.Approximate(d2.Add(-time.Hour), d2.Add(time.Hour)) || s.Approximate(d1.Add(-time.Hour), d1.Add(time.Hour)) {
		t.Error("the wrong days approximate")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "rollups.json")); string(got) != string(rollups) {
		t.Errorf("rollups: %s", got)
	}
	backup := filepath.Join(dir, fmt.Sprintf("before-recount-%d", RecountVersion))
	if _, err := os.Stat(filepath.Join(backup, "rollups.json")); err != nil {
		t.Errorf("no copy: %v", err)
	}
	if files, _ := filepath.Glob(filepath.Join(backup, "*.jsonl")); len(files) != 2 {
		t.Errorf("copied %v", files)
	}
}
