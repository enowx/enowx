package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/proto"
)

func appendLines(t *testing.T, path string, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

const (
	cpMeta = `{"type":"meta","time":"2026-11-05T10:00:00Z","conversation":"c1","dir":"/repo","model":"claude-opus-5-5","effort":"high","mode":"default","budget":10,"window":1000000}
`
	cpUser = `{"type":"user","time":"2026-11-05T10:00:01Z","text":"Fix the bug","message":{"role":"user","content":"Fix the bug"}}
`
	cpStream = `{"type":"block_start","time":"2026-11-05T10:00:02Z","msg":"m1","block":0,"kind":"thinking"}
{"type":"delta","time":"2026-11-05T10:00:02Z","msg":"m1","block":0,"kind":"thinking","text":"Looking"}
{"type":"block_start","time":"2026-11-05T10:00:03Z","msg":"m1","block":1,"kind":"text"}
{"type":"delta","time":"2026-11-05T10:00:03Z","msg":"m1","block":1,"text":"I will "}
{"type":"delta","time":"2026-11-05T10:00:03Z","msg":"m1","block":1,"text":"read it"}
`
	cpFinal = `{"type":"assistant","time":"2026-11-05T10:00:04Z","msg":"m1","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Looking closely","signature":"x"},{"type":"text","text":"I will read it."},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"a.go"}}]},"usage":{"input":10,"output":20,"cacheRead":30,"cacheWrite":40},"cost":0.25,"totalCost":1.5}
`
	cpTool = `{"type":"tool_start","time":"2026-11-05T10:00:05Z","tool":{"id":"t1","name":"Read","input":{"file_path":"a.go"},"detail":"a.go"}}
{"type":"tool_end","time":"2026-11-05T10:00:06Z","tool":{"id":"t1","name":"Read","result":"package a","durationMs":12}}
`
)

func texts(r *Reader) []string {
	entries, _ := r.Since(0)
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Kind + ":" + e.Text
	}
	return out
}

func TestPlatformReaderStreamsThenFinalText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c1.jsonl")
	appendLines(t, path, cpMeta+cpUser+cpStream)
	r := NewPlatformReader(path)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(texts(r), "|"); got != "user:Fix the bug|thinking:Looking|assistant:I will read it" {
		t.Fatalf("streaming: %s", got)
	}
	_, rev := r.Since(0)
	// A delta changes an entry's revision: the chat gets what grew.
	appendLines(t, path, `{"type":"delta","time":"2026-11-05T10:00:03Z","msg":"m1","block":1,"text":" now"}`+"\n")
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	changed, _ := r.Since(rev)
	if len(changed) != 1 || changed[0].Text != "I will read it now" || changed[0].Index != 2 {
		t.Fatalf("a delta: %+v", changed)
	}
	// The finished answer replaces what streamed, each where it was.
	appendLines(t, path, cpFinal)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(texts(r), "|"); got != "user:Fix the bug|thinking:Looking closely|assistant:I will read it." {
		t.Fatalf("final: %s", got)
	}
	if r.Model != "claude-opus-5-5" || r.Effort != "high" || r.Mode != "default" || r.Window != 1_000_000 || r.Context != 100 || r.CostUSD != 1.5 || r.BudgetUSD != 10 {
		t.Fatalf("settings: %+v", r)
	}
}

func TestPlatformReaderRecords(t *testing.T) {
	cases := []struct {
		name, lines string
		check       func(t *testing.T, r *Reader, entries []proto.TranscriptEntry)
	}{
		{"an answer with nothing streamed", cpFinal, func(t *testing.T, r *Reader, es []proto.TranscriptEntry) {
			if len(es) != 2 || es[0].Kind != proto.TranscriptThinking || es[1].Kind != proto.TranscriptAssistant {
				t.Fatalf("%+v", es)
			}
		}},
		{"an answer recorded twice", cpFinal + cpFinal, func(t *testing.T, r *Reader, es []proto.TranscriptEntry) {
			if len(es) != 2 {
				t.Fatalf("%+v", es)
			}
		}},
		{"a thought with no text shows nothing", `{"type":"block_start","msg":"m","block":0,"kind":"thinking"}
{"type":"delta","msg":"m","block":0,"kind":"thinking","text":"  "}
{"type":"assistant","msg":"m","message":{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"x"},{"type":"text","text":"Done"}]}}
`, func(t *testing.T, r *Reader, es []proto.TranscriptEntry) {
			if len(es) != 1 || es[0].Text != "Done" {
				t.Fatalf("%+v", es)
			}
		}},
		{"a tool's life", cpTool, func(t *testing.T, r *Reader, es []proto.TranscriptEntry) {
			if len(es) != 1 || es[0].Text != "Read" || es[0].Detail != "a.go" || !es[0].Done || es[0].Failed || es[0].Result != "package a" || es[0].DurationMs != 12 {
				t.Fatalf("%+v", es)
			}
		}},
		{"an edit that failed", `{"type":"tool_start","time":"2026-11-05T10:00:05Z","tool":{"id":"t2","name":"Edit","input":{"file_path":"a.go","old_string":"x","new_string":"y"}}}
{"type":"tool_end","time":"2026-11-05T10:00:07Z","tool":{"id":"t2","name":"Edit","result":"no match","failed":true,"old":"x\ny","new":"z","added":1,"removed":2}}
`, func(t *testing.T, r *Reader, es []proto.TranscriptEntry) {
			e := es[0]
			if !e.Failed || e.Old != "x\ny" || e.New != "z" || e.Added != 1 || e.Removed != 2 || e.DurationMs != 2000 || e.Detail != "a.go" {
				t.Fatalf("%+v", e)
			}
		}},
		{"questions", `{"type":"tool_start","tool":{"id":"q","name":"AskUserQuestion","questions":[{"header":"DB","question":"Which one?","options":["a","b"],"about":["first","second"],"multi":true}]}}
{"type":"tool_wait","asks":"questions","tool":{"id":"q","name":"AskUserQuestion"}}
`, func(t *testing.T, r *Reader, es []proto.TranscriptEntry) {
			q := es[0].Questions
			if len(q) != 1 || q[0].Question != "Which one?" || q[0].Options[1] != "b" || q[0].About[0] != "first" || !q[0].Multi || es[0].Detail != "Which one?" {
				t.Fatalf("%+v", es[0])
			}
		}},
		{"notice and compaction", `{"type":"notice","level":"warning","text":"Retrying in 5s"}
{"type":"compact","before":180000,"messages":[]}
`, func(t *testing.T, r *Reader, es []proto.TranscriptEntry) {
			if len(es) != 2 || es[0].Kind != proto.TranscriptNotice || es[0].Text != "Retrying in 5s" || !strings.Contains(es[1].Text, "compacted") {
				t.Fatalf("%+v", es)
			}
		}},
		{"a subagent folds into its call", `{"type":"tool_start","tool":{"id":"t","name":"Task","detail":"look around"}}
{"type":"user","sidechain":"t","text":"look around"}
{"type":"delta","sidechain":"t","msg":"s","text":"hm"}
{"type":"tool_start","sidechain":"t","tool":{"id":"s1","name":"Grep","detail":"foo"}}
{"type":"tool_end","sidechain":"t","tool":{"id":"s1","name":"Grep","result":"x"}}
{"type":"assistant","sidechain":"t","msg":"s","totalCost":2,"usage":{"input":999,"output":1,"cacheRead":0,"cacheWrite":0},"message":{"role":"assistant","content":[{"type":"text","text":"hidden"}]}}
`, func(t *testing.T, r *Reader, es []proto.TranscriptEntry) {
			if len(es) != 1 || es[0].Result != "Grep foo" || es[0].Done || r.CostUSD != 2 || r.Context != 0 {
				t.Fatalf("%+v, %+v", es, r)
			}
		}},
		{"a call left running ends with its turn", `{"type":"tool_start","time":"2026-11-05T10:00:05Z","tool":{"id":"t","name":"Bash"}}
{"type":"turn_end","time":"2026-11-05T10:00:06Z","stop":"interrupted"}
`, func(t *testing.T, r *Reader, es []proto.TranscriptEntry) {
			if len(es) != 2 || !es[0].Done || !es[0].Failed || es[1].Text != "Interrupted" {
				t.Fatalf("%+v", es)
			}
		}},
		{"a broken line is passed over", "not json\n" + cpUser, func(t *testing.T, r *Reader, es []proto.TranscriptEntry) {
			if len(es) != 1 {
				t.Fatalf("%+v", es)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "c.jsonl")
			appendLines(t, path, tc.lines)
			r := NewPlatformReader(path)
			if err := r.Update(); err != nil {
				t.Fatal(err)
			}
			es, _ := r.Since(0)
			tc.check(t, r, es)
		})
	}
}

// A line the agent hasn't finished waits; a resume appends; a file rewritten is read anew.
func TestPlatformReaderFollowsItsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c1.jsonl")
	line := cpUser
	appendLines(t, path, cpMeta+line[:20])
	r := NewPlatformReader(path)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 0 {
		t.Fatalf("half a line read: %v", texts(r))
	}
	appendLines(t, path, line[20:])
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 1 {
		t.Fatalf("the rest: %v", texts(r))
	}
	// Resumed: another Meta, its settings now; the entries go on.
	appendLines(t, path, cpFinal+`{"type":"meta","conversation":"c1","dir":"/repo","model":"claude-sonnet-5-5","mode":"plan"}`+"\n"+cpUser)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 4 || r.Model != "claude-sonnet-5-5" || r.Mode != "plan" || r.Window != 0 || r.Effort != "high" || r.BudgetUSD != 10 || r.CostUSD != 1.5 {
		t.Fatalf("resumed: %v %+v", texts(r), r)
	}
	// Rewritten shorter: from the start again, with nothing kept.
	if err := os.WriteFile(path, []byte(cpUser), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 1 || r.Model != "" || r.CostUSD != 0 {
		t.Fatalf("rewritten: %v %+v", texts(r), r)
	}
}

func TestPlatformIsRegistered(t *testing.T) {
	home, store := t.TempDir(), t.TempDir()
	t.Setenv("ENOWX_HOME", "")
	if !Supports(record.AgentID) {
		t.Fatal("not supported")
	}
	// Run by hand: its default folder, for the folder it ran in alone.
	sessions := record.SessionsDir(home)
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	here := filepath.Join(sessions, "c1.jsonl")
	appendLines(t, here, cpMeta)
	appendLines(t, filepath.Join(sessions, "other.jsonl"), `{"type":"meta","dir":"/elsewhere"}`+"\n")
	found := FindIn(record.AgentID, home, "/repo", store)
	if len(found) != 1 || found[0].Path != here {
		t.Fatalf("default folder: %+v", found)
	}
	// In a session: its own folder.
	mine := filepath.Join(store, "c2.jsonl")
	appendLines(t, mine, cpMeta+cpFinal)
	found = FindIn(record.AgentID, home, "/repo", store)
	if len(found) != 1 || found[0].Path != mine {
		t.Fatalf("store: %+v", found)
	}
	if id := ConversationID(record.AgentID, mine); id != "c2" {
		t.Fatalf("id = %q", id)
	}
	if model, effort, mode := LastSettings(record.AgentID, mine); model != "claude-opus-5-5" || effort != "high" || mode != "default" {
		t.Fatalf("settings: %s %s %s", model, effort, mode)
	}
	if Compacted(record.AgentID, mine) {
		t.Fatal("not compacted yet")
	}
	appendLines(t, mine, `{"type":"compact","before":5}`+"\n")
	if !Compacted(record.AgentID, mine) {
		t.Fatal("compacted")
	}
}
