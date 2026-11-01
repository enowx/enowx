package transcript

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeTranscriptFollowsTheFile(t *testing.T) {
	home := t.TempDir()
	dir := "/home/me/code/app"
	folder := filepath.Join(home, ".claude", "projects", "-home-me-code-app")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, "s1.jsonl")
	appendTo(t, path, `{"type":"user","cwd":"/home/me/code/app","timestamp":"2026-10-22T06:00:00Z","message":{"role":"user","content":"Fix the login bug"}}
{"type":"user","isMeta":true,"message":{"content":"meta"}}
{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Look at auth.go"},{"type":"text","text":"Reading the code."}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./auth"}}]}}
{"type":"assistant","isSidechain":true,"message":{"content":"a subagent's words"}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"FAIL auth"`)

	if got := ClaudeFiles(home, dir); len(got) != 1 || got[0] != path {
		t.Fatalf("files = %v", got)
	}
	r := NewClaudeReader(path)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	entries, rev := r.Since(0)
	kinds := []string{}
	for _, e := range entries {
		kinds = append(kinds, e.Kind)
	}
	want := []string{proto.TranscriptUser, proto.TranscriptThinking, proto.TranscriptAssistant, proto.TranscriptTool}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	if tool := entries[3]; tool.Text != "Bash" || tool.Detail != "go test ./auth" || tool.Done {
		t.Fatalf("tool = %+v", tool)
	}

	// The agent finishes the half-written line, then goes on.
	appendTo(t, path, `,"is_error":true}]}}
{"type":"user","message":{"content":"<command-name>/clear</command-name>"}}
`)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	changed, _ := r.Since(rev)
	if len(changed) != 2 {
		t.Fatalf("changed = %+v", changed)
	}
	if tool := changed[0]; tool.Index != 3 || !tool.Done || !tool.Failed || tool.Result != "FAIL auth" {
		t.Fatalf("tool after result = %+v", tool)
	}
	if n := changed[1]; n.Kind != proto.TranscriptNotice || n.Text != "Ran /clear" {
		t.Fatalf("notice = %+v", n)
	}
}

func TestClaudeFilesFindsFoldersByTheirRecords(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".claude", "projects", "named-differently")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	appendTo(t, filepath.Join(folder, "s.jsonl"), `{"type":"summary"}`+"\n"+`{"type":"user","cwd":"/srv/my.app","message":{"content":"hi"}}`+"\n")
	if got := ClaudeFiles(home, "/srv/my.app"); len(got) != 1 {
		t.Fatalf("files = %v", got)
	}
	if got := ClaudeFiles(home, "/srv/other"); len(got) != 0 {
		t.Fatalf("files for another folder = %v", got)
	}
}

// A session started through a symlink is found: agents name the folder after
// the real path.
func TestClaudeFilesFollowASymlinkedDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges")
	}
	home, base := t.TempDir(), t.TempDir()
	base, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	real, link := filepath.Join(base, "real"), filepath.Join(base, "link")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(home, ".claude", "projects", ClaudeFolder(real))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "a.jsonl")
	if err := os.WriteFile(file, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if got := ClaudeFiles(home, link); len(got) != 1 || got[0] != file {
		t.Fatalf("found %v", got)
	}
}

func TestClaudeSettingsThoughtsAndEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendTo(t, path, `{"type":"permission-mode","permissionMode":"plan"}
{"type":"assistant","effort":"high","thinkingDurationMs":4200,"message":{"model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"Plan it"},{"type":"tool_use","id":"e1","name":"Edit","input":{"file_path":"a.go","old_string":"a\nb\nc","new_string":"a\nB\nc\nd"}}]}}
{"type":"assistant","message":{"model":"<synthetic>","content":"ok"}}
{"type":"assistant","message":{"model":"claude-opus-5-5","usage":{"input_tokens":2,"cache_read_input_tokens":140126,"cache_creation_input_tokens":367,"output_tokens":920},"content":"done"}}
`)
	r := NewClaudeReader(path)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	if r.Model != "claude-opus-5-5" || r.Mode != "plan" || r.Effort != "high" {
		t.Fatalf("settings = %q %q %q", r.Model, r.Mode, r.Effort)
	}
	if r.Context != 2+140126+367+920 {
		t.Fatalf("context = %d", r.Context)
	}
	entries, _ := r.Since(0)
	if len(entries) < 2 || entries[0].Kind != proto.TranscriptThinking || entries[0].DurationMs != 4200 {
		t.Fatalf("entries = %+v", entries)
	}
	if e := entries[1]; e.Text != "Edit" || e.Added != 2 || e.Removed != 1 || e.Old == "" || e.New == "" {
		t.Fatalf("edit = %+v", e)
	}
}

func TestBeforeStartsAtATurn(t *testing.T) {
	r := &Reader{}
	for _, k := range []string{"user", "assistant", "tool", "user", "assistant", "user", "tool", "assistant"} {
		r.add(proto.TranscriptEntry{Kind: k})
	}
	got, _ := r.Before(r.Len(), 2) // the last two would cut the last turn
	if len(got) != 3 || got[0].Index != 5 {
		t.Fatalf("tail = %+v", got)
	}
	got, _ = r.Before(5, 1)
	if len(got) != 2 || got[0].Index != 3 || got[1].Index != 4 {
		t.Fatalf("before 5 = %+v", got)
	}
	if got, _ := r.Before(0, 10); len(got) != 0 {
		t.Fatalf("before 0 = %+v", got)
	}
}

// A message sent again after a rewind replaces the messages after the
// point it went back to, which stay in the file.
func TestClaudeTranscriptFollowsTheBranchSentLast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendTo(t, path, `{"type":"user","uuid":"u1","message":{"content":"Reply AAA"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}
{"type":"user","uuid":"r1","parentUuid":"a1","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
{"type":"assistant","uuid":"a2","parentUuid":"r1","message":{"content":"AAA"}}
{"type":"system","uuid":"s1","parentUuid":"a2","subtype":"turn_duration"}
{"type":"user","uuid":"u2","parentUuid":"s1","message":{"content":"Reply BBB"}}
{"type":"assistant","uuid":"a3","parentUuid":"u2","message":{"content":"BBB"}}
`)
	r := NewClaudeReader(path)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	if r.Len() != 5 || r.Restarted() {
		t.Fatalf("len %d before the rewind", r.Len())
	}
	appendTo(t, path, `{"type":"user","uuid":"u3","parentUuid":"s1","message":{"content":"Reply XXX"}}
{"type":"assistant","uuid":"a4","parentUuid":"u3","message":{"content":"XXX"}}
`)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Since(0)
	var texts []string
	for _, e := range got {
		texts = append(texts, e.Text)
	}
	if strings.Join(texts, "|") != "Reply AAA|Bash|AAA|Reply XXX|XXX" || !r.Restarted() {
		t.Fatalf("entries = %q", texts)
	}
}

// A question the agent asks comes with its options, still unanswered.
func TestClaudeQuestionsCarryTheirOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendTo(t, path, `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"q1","name":"AskUserQuestion","input":{"questions":[{"question":"Which database?","header":"Database","multiSelect":false,"options":[{"label":"Postgres","description":"What the project runs"},{"label":"SQLite","description":"No server"}]}]}}]}}
`)
	r := NewClaudeReader(path)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	entries, _ := r.Since(0)
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
	}
	e := entries[0]
	if e.Done || e.Detail != "Which database?" || len(e.Questions) != 1 {
		t.Fatalf("question = %+v", e)
	}
	if q := e.Questions[0]; q.Header != "Database" || len(q.Options) != 2 || q.Options[1] != "SQLite" || q.About[0] != "What the project runs" {
		t.Fatalf("question = %+v", q)
	}
}

// A tool call says how long it took, from its call to its result.
func TestClaudeToolsTakeTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendTo(t, path, `{"type":"assistant","timestamp":"2026-10-30T14:02:00.000Z","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}
{"type":"user","timestamp":"2026-10-30T14:02:01.500Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
`)
	r := NewClaudeReader(path)
	if err := r.Update(); err != nil {
		t.Fatal(err)
	}
	entries, _ := r.Since(0)
	if len(entries) != 1 || !entries[0].Done || entries[0].DurationMs != 1500 {
		t.Fatalf("entries = %+v", entries)
	}
}
