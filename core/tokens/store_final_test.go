package tokens

import (
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
