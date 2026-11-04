package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/enowx/enowx/proto"
)

func TestCodexTranscriptFollowsTheRollout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", "")
	day := filepath.Join(home, ".codex", "sessions", "2026", "10", "23")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(day, "rollout-2026-10-23T09-00-00-b.jsonl")
	appendTo(t, other, `{"timestamp":"2026-10-23T09:00:00Z","type":"session_meta","payload":{"id":"b","cwd":"/code/other"}}`+"\n")
	path := filepath.Join(day, "rollout-2026-10-23T10-00-00-a.jsonl")
	appendTo(t, path, `{"timestamp":"2026-10-23T10:00:00Z","type":"session_meta","payload":{"id":"a","cwd":"/code/app"}}
{"timestamp":"2026-10-23T10:00:01Z","type":"turn_context","payload":{"cwd":"/code/app","model":"gpt-5.5-codex","effort":"high"}}
{"timestamp":"2026-10-23T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>...</environment_context>"}]}}
{"timestamp":"2026-10-23T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"Fix the bug"}}
{"timestamp":"2026-10-23T10:00:02Z","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"Look at a.go"}],"encrypted_content":"x"}}
{"timestamp":"2026-10-23T10:00:03Z","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"command\":[\"bash\",\"-lc\",\"go test ./...\"]}","call_id":"c1"}}
{"timestamp":"2026-10-23T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"Exit code: 1\nWall time: 1.2 seconds\nOutput:\nFAIL a"}}
{"timestamp":"2026-10-23T10:00:05Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","call_id":"c2","input":"*** Begin Patch\n*** Update File: a.go\n@@\n-a\n+b\n+c\n*** End Patch"}}
{"timestamp":"2026-10-23T10:00:06Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":900,"output_tokens":100,"total_tokens":1000},"model_context_window":272000}}}
`)

	found := CodexFiles(home, "/code/app")
	if len(found) != 1 || found[0].Path != path {
		t.Fatalf("found %v", found)
	}
	r := NewCodexReader(path)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	got, rev := r.Since(0)
	if len(got) != 4 || got[0].Kind != proto.TranscriptUser || got[0].Text != "Fix the bug" ||
		got[1].Kind != proto.TranscriptThinking ||
		got[2].Text != "Bash" || got[2].Detail != "go test ./..." || !got[2].Failed || got[2].Result != "FAIL a" ||
		got[3].Text != "Edit" || got[3].Detail != "a.go" || got[3].Added != 2 || got[3].Removed != 1 || got[3].Done {
		t.Fatalf("entries = %+v", got)
	}
	if r.Model != "gpt-5.5-codex" || r.Effort != "high" || r.Context != 1000 || r.Window != 272000 {
		t.Fatalf("model %q effort %q context %d of %d", r.Model, r.Effort, r.Context, r.Window)
	}

	appendTo(t, path, `{"timestamp":"2026-10-23T10:00:07Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c2","output":"Success. Updated the following files:\nM a.go"}}
{"timestamp":"2026-10-23T10:00:08Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Fixed."}]}}
`)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Since(rev)
	if len(got) != 2 || got[0].Index != 3 || !got[0].Done || got[0].Failed || got[1].Text != "Fixed." {
		t.Fatalf("news = %+v", got)
	}
}

// Codex 0.164 records items as it shows them; the model's calls are
// scripts calling the real tools, so the response items aren't read.
func TestCodexTranscriptReadsRecordedItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	appendTo(t, path, `{"timestamp":"2026-10-23T14:24:46Z","type":"session_meta","payload":{"id":"a","cwd":"/code/app"}}
{"timestamp":"2026-10-23T14:24:46Z","type":"turn_context","payload":{"cwd":"/code/app","model":"gpt-6-luna","summary":"auto","collaboration_mode":{"mode":"default"}}}
{"timestamp":"2026-10-23T14:24:46Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Run it"}]}}
{"timestamp":"2026-10-23T14:24:47Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u1","content":[{"type":"text","text":"Run it"}]}}}
{"timestamp":"2026-10-23T14:24:48Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"m1","content":[{"type":"Text","text":"Running."}],"phase":"commentary"}}}
{"timestamp":"2026-10-23T14:24:48Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Running."}]}}
{"timestamp":"2026-10-23T14:24:49Z","type":"response_item","payload":{"type":"custom_tool_call","name":"exec","call_id":"c1","input":"await tools.exec_command({cmd:\"cat x\"})"}}
{"timestamp":"2026-10-23T14:24:49Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"e1","command":["/bin/zsh","-lc","cat x"],"parsed_cmd":[{"type":"unknown","cmd":"cat x"}],"status":"failed","aggregated_output":"cat: x: No such file\n","exit_code":1}}}
{"timestamp":"2026-10-23T14:24:50Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"FileChange","id":"e2","changes":{"/code/app/notes.txt":{"type":"update","unified_diff":"@@ -1 +1 @@\n-two\n\\ No newline at end of file\n+three\n"}},"status":"completed","stdout":"Success.","stderr":""}}}
`)
	r := NewCodexReader(path)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Since(0)
	if len(got) != 4 || got[0].Text != "Run it" || got[1].Text != "Running." ||
		got[2].Text != "Bash" || got[2].Detail != "cat x" || !got[2].Failed || got[2].Result != "cat: x: No such file" ||
		got[3].Text != "Edit" || got[3].Detail != "/code/app/notes.txt" || got[3].Added != 1 || got[3].Removed != 1 || got[3].Old != "two" {
		t.Fatalf("entries = %+v", got)
	}
	if r.Model != "gpt-6-luna" {
		t.Fatalf("model %q", r.Model)
	}
}

// Going back forks the session into a new rollout that keeps the start of
// the one before, by its byte offset.
func TestCodexTranscriptReadsTheRolloutAForkContinues(t *testing.T) {
	day := filepath.Join(t.TempDir(), "sessions", "2026", "10", "23")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	kept := `{"timestamp":"2026-10-23T15:07:32Z","type":"session_meta","payload":{"id":"th","cwd":"/code/app"}}
{"timestamp":"2026-10-23T15:07:33Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u1","content":[{"type":"text","text":"AAA?"}]}}}
{"timestamp":"2026-10-23T15:07:34Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"m1","content":[{"type":"Text","text":"AAA"}]}}}
`
	base := filepath.Join(day, "rollout-2026-10-23T22-07-32-th.jsonl")
	appendTo(t, base, kept+`{"timestamp":"2026-10-23T15:07:40Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u2","content":[{"type":"text","text":"BBB?"}]}}}
`)
	fork := filepath.Join(day, "rollout-2026-10-23T22-08-11-th_f1.jsonl")
	appendTo(t, fork, fmt.Sprintf(`{"timestamp":"2026-10-23T15:08:11Z","type":"session_meta","payload":{"id":"th","cwd":"/code/app","history_base":{"thread_id":"th","end_byte_offset":%d}}}
{"timestamp":"2026-10-23T15:08:12Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u3","content":[{"type":"text","text":"XXX?"}]}}}
`, len(kept)))
	r := NewCodexReader(fork)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Since(0)
	if len(got) != 3 || got[0].Text != "AAA?" || got[1].Text != "AAA" || got[2].Text != "XXX?" {
		t.Fatalf("entries = %+v", got)
	}
}

// A Codex conversation gives the id it resumes by, or waking it would start a new one.
func TestCodexConversationID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	line := `{"timestamp":"2026-10-23T14:24:47Z","type":"session_meta","payload":{"id":"019a-conversation","cwd":"/work"}}` + "\n"
	if err := os.WriteFile(path, []byte(line+`{"type":"event_msg"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ConversationID("codex", path); got != "019a-conversation" {
		t.Fatalf("got %q", got)
	}
	if got := ConversationID("codex", filepath.Join(t.TempDir(), "none.jsonl")); got != "" {
		t.Fatalf("no file: %q", got)
	}
}
