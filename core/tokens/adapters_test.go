package tokens

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// fixedNow is "today" for the fixtures, written in October 2026.
var fixedNow = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func copyFixture(t *testing.T, name, to string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, to, string(data))
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeHome lays the fixtures out where each agent keeps its records.
func fakeHome(t *testing.T) (Env, string) {
	t.Helper()
	for _, v := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "OPENCLAUDE_CONFIG_DIR", "QODER_CONFIG_DIR", "PI_CODING_AGENT_DIR", "XDG_DATA_HOME",
		"OPENCODE_DB", "KILO_DB", "GEMINI_CLI_HOME", "QWEN_RUNTIME_DIR", "QWEN_HOME", "GOOSE_PATH_ROOT", "CLINE_DIR", "CONTINUE_GLOBAL_DIR",
		"KIMI_SHARE_DIR", "VIBE_HOME", "DSH_HOME", "OPENCLAW_STATE_DIR"} {
		t.Setenv(v, "")
	}
	home, project := t.TempDir(), t.TempDir()
	at := func(parts ...string) string { return filepath.Join(append([]string{home}, parts...)...) }
	claudeFile := at(".claude", "projects", "-work-app", "s-claude-1.jsonl")
	copyFixture(t, "claude.jsonl", claudeFile)
	copyFixture(t, "claude-subagent.jsonl", at(".claude", "projects", "-work-app", "s-claude-1", "subagents", "agent-1.jsonl"))
	copyFixture(t, "claude-subagent.jsonl", at(".openclaude", "projects", "-work-app", "oc.jsonl"))
	copyFixture(t, "codex.jsonl", at(".codex", "sessions", "2026", "10", "21", "rollout-2026-10-21T08-00-00-cx-1.jsonl"))
	copyFixture(t, "pi.jsonl", at(".omp", "agent", "sessions", "--work-web--", "2026-10-22T09-00-00-000Z_pi-sess-1.jsonl"))
	copyFixture(t, "codebuddy.jsonl", at(".codebuddy", "projects", "-work-cli", "cb-1.jsonl"))
	copyFixture(t, "opencode-part.json", at(".local", "share", "opencode", "storage", "part", "msg_legacy1", "prt_legacy1.json"))
	writeFile(t, at(".local", "share", "opencode", "storage", "message", "ses_old", "msg_legacy1.json"),
		`{"id":"msg_legacy1","sessionID":"ses_old","role":"assistant","path":{"cwd":"/work/old"},"modelID":"claude-sonnet-4-5","providerID":"anthropic","time":{"created":1792700000000,"completed":1792700004000}}`)
	openCodeDB(t, at(".local", "share", "opencode", "opencode.db"))
	copyFixture(t, "gemini.jsonl", at(".gemini", "tmp", "work-gem", "chats", "session-2026-10-24T10-00-gem1.jsonl"))
	writeFile(t, at(".gemini", "tmp", "work-gem", ".project_root"), "/work/gem\n")
	copyFixture(t, "qwen.jsonl", at(".qwen", "projects", "-work-qw", "chats", "qw-1.jsonl"))
	gooseDB(t, at(".local", "share", "goose", "sessions", "sessions.db"))
	copilotDB(t, at(".copilot", "session-store.db"))
	devinDB(t, at(".local", "share", "devin", "cli", "sessions.db"))
	grokDB(t, at(".grok", "grok.db"))
	copyFixture(t, "grok-unified.jsonl", at(".grok", "logs", "unified.jsonl"))
	copyFixture(t, "cline-messages.json", at(".cline", "data", "sessions", "cs-1", "cs-1.messages.json"))
	writeFile(t, at(".cline", "data", "sessions", "cs-1", "cs-1.json"), `{"cwd":"/work/cline","model":"claude-sonnet-5-5","provider":"anthropic"}`)
	copyFixture(t, "cline-ui_messages.json", at(".cline", "data", "tasks", "t-1", "ui_messages.json"))
	copyFixture(t, "continue.json", at(".continue", "sessions", "cn-1.json"))
	copyFixture(t, "amp.json", at(".local", "share", "amp", "threads", "T-abc.json"))
	copyFixture(t, "auggie.json", at(".augment", "sessions", "aug-1.json"))
	copyFixture(t, "kiro.json", at(".kiro", "sessions", "cli", "kiro-1.json"))
	copyFixture(t, "jcode.json", at(".jcode", "sessions", "session_jc-1.json"))
	sum := md5.Sum([]byte("/work/kimi"))
	copyFixture(t, "kimi-wire.jsonl", at(".kimi", "sessions", hex.EncodeToString(sum[:]), "ks-1", "wire.jsonl"))
	writeFile(t, at(".kimi", "kimi.json"), `{"work_dirs":[{"path":"/work/kimi"}]}`)
	copyFixture(t, "vibe-journal.jsonl", at(".vibe", "logs", "session", "unified", "v-1", "journal", "0000000000000001.jsonl"))
	copyFixture(t, "muse.jsonl", at(".local", "share", "muse", "sessions", "2026", "10", "26", "mu-1", "session.jsonl"))
	copyFixture(t, "commandcode.jsonl", at(".commandcode", "projects", "-work-cc", "ccs-1.jsonl"))
	zstdFile(t, "dsh.jsonl", at(".dsh", "sessions", "-work-dsh", "dsh-1", "session.jsonl.zstd"))
	openClawDB(t, at(".openclaw", "agents", "main", "agent", "openclaw-agent.sqlite"))
	copyFixture(t, "aider.chat.history.md", filepath.Join(project, ".aider.chat.history.md"))
	return Env{Home: home, Dirs: []string{project}}, claudeFile
}

func sumBy(calls []Call) map[string]proto.TokenCounts {
	out := map[string]proto.TokenCounts{}
	for _, c := range calls {
		t := out[c.Agent]
		t.Add(proto.TokenCounts{Input: c.Input, CacheRead: c.CacheRead, CacheWrite: c.CacheWrite, Output: c.Output, Reasoning: c.Reasoning, Calls: 1})
		out[c.Agent] = t
	}
	return out
}

// The counts each agent's fixture holds, normalized: input without the
// cache, output with the reasoning in.
var wantByAgent = map[string]proto.TokenCounts{
	// msg_A once (two records), msg_B, the subagent's; the synthetic
	// error counted nothing.
	"claude":     {Input: 3015, CacheRead: 62000, CacheWrite: 2000, Output: 550, Reasoning: 120, Calls: 3},
	"openclaude": {Input: 3000, Output: 50, Calls: 1},
	// The count said twice, once.
	"codex": {Input: 8638 + 3613, CacheRead: 10752 + 19200, Output: 104, Reasoning: 30, Calls: 2},
	"omp":   {Input: 1500, CacheRead: 4000, CacheWrite: 500, Output: 120, Calls: 2},
	// Each step, not only the message's last: the older layout's and two
	// of 1.x.
	"opencode":     {Input: 900 + 72013 + 181, CacheRead: 3000 + 71936, CacheWrite: 100, Output: 250 + 42 + 10, Reasoning: 50, Calls: 3},
	"codebuddy":    {Input: 14409, CacheRead: 9600, Output: 3, Reasoning: 1, Calls: 1},
	"aider":        {Input: 9500, CacheRead: 3300, CacheWrite: 1200, Output: 330, Calls: 2},
	"gemini":       {Input: 4000, CacheRead: 8000, Output: 420, Reasoning: 120, Calls: 1},
	"qwen-code":    {Input: 3000, CacheRead: 6000, Output: 200, Calls: 1},
	"goose":        {Input: 1000, CacheRead: 9000, CacheWrite: 1000, Output: 400, Calls: 1},
	"copilot":      {Input: 4000, CacheRead: 16000, Output: 500, Reasoning: 200, Calls: 1},
	"devin":        {Input: 1000, CacheRead: 29000, CacheWrite: 500, Output: 700, Calls: 1},
	"grok":         {Input: 900 + 2000, CacheRead: 4000, Output: 100 + 400, Reasoning: 100, Calls: 2},
	"cline":        {Input: 40 + 1000 + 12, CacheRead: 15000 + 4000 + 9000, CacheWrite: 1000 + 2000, Output: 200 + 100 + 340, Calls: 3},
	"continue":     {Input: 952 + 20, CacheRead: 2048 + 5000, CacheWrite: 100, Output: 230, Calls: 2},
	"amp":          {Input: 100, CacheRead: 7000, CacheWrite: 300, Output: 50, Calls: 1},
	"aug":          {Input: 30, CacheRead: 8000, CacheWrite: 500, Output: 120, Calls: 1},
	"kiro":         {Input: 2000, CacheRead: 1000, Output: 300, Calls: 1},
	"jcode":        {Input: 15, CacheRead: 6000, CacheWrite: 400, Output: 90, Reasoning: 30, Calls: 1},
	"kimi":         {Input: 1200, CacheRead: 8000, Output: 300, Calls: 1},
	"mistral-vibe": {Input: 2000, CacheRead: 3000, Output: 200, Reasoning: 40, Calls: 1},
	"muse":         {Input: 1000, CacheRead: 3000, Output: 100, Reasoning: 20, Calls: 1},
	"command-code": {Input: 80, CacheRead: 2000, CacheWrite: 100, Output: 60, Calls: 1},
	"dsh":          {Input: 2000, CacheRead: 5000, Output: 250, Calls: 1},
	"openclaw":     {Input: 50, CacheRead: 1000, Output: 20, Calls: 1},
}

// wantDir is where the fixtures say each agent ran, for those that say.
var wantDir = map[string]string{"codex": "/work/api", "omp": "/work/web", "gemini": "/work/gem", "qwen-code": "/work/qw", "goose": "/work/goose",
	"copilot": "/work/cp", "devin": "/work/devin", "kimi": "/work/kimi", "dsh": "/work/dsh", "openclaw": "/work/claw", "jcode": "/work/jc", "kiro": "/work/kiro"}

func TestAdaptersCountEachAgentsRecords(t *testing.T) {
	env, _ := fakeHome(t)
	sc := NewScanner(filepath.Join(t.TempDir(), "scan.json"))
	calls, _ := sc.Scan(env, time.Time{}, nil)
	store := Open(t.TempDir())
	store.Now = func() time.Time { return fixedNow }
	added, err := store.Add(calls)
	if err != nil {
		t.Fatal(err)
	}
	got := sumBy(added)
	for agent, w := range wantByAgent {
		if got[agent] != w {
			t.Errorf("%s: got %+v, want %+v", agent, got[agent], w)
		}
	}
	for agent := range got {
		if _, ok := wantByAgent[agent]; !ok {
			t.Errorf("%s counted, unexpected: %+v", agent, got[agent])
		}
	}
	for _, c := range added {
		if c.Time.IsZero() || c.Layer != proto.TokenLayerRecords || c.ID == "" {
			t.Errorf("%s: %+v", c.Agent, c)
		}
		if d, ok := wantDir[c.Agent]; ok && c.Dir != d {
			t.Errorf("%s ran in %q, want %q", c.Agent, c.Dir, d)
		}
	}

	// Read again from the start (a backfill): nothing counts twice.
	sc.Reset()
	again, _ := sc.Scan(env, time.Time{}, nil)
	if len(again) != len(calls) {
		t.Fatalf("read again: %d calls, want %d", len(again), len(calls))
	}
	if more, _ := store.Add(again); len(more) != 0 {
		t.Fatalf("read twice, counted twice: %+v", more)
	}
}

func TestScanReadsOnlyWhatWasAppended(t *testing.T) {
	env, claudeFile := fakeHome(t)
	onlyClaude := func(a string) bool { return a == "claude" }
	sc := NewScanner(filepath.Join(t.TempDir(), "scan.json"))
	if first, _ := sc.Scan(env, time.Time{}, onlyClaude); len(first) != 4 { // two records of msg_A, msg_B, the subagent's
		t.Fatalf("first scan: %d calls", len(first))
	}
	if again, files := sc.Scan(env, time.Time{}, onlyClaude); len(again) != 0 || files != 0 {
		t.Fatalf("nothing changed, yet read %d files, %d calls", files, len(again))
	}
	f, err := os.OpenFile(claudeFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	// A line half written isn't read until it ends.
	f.WriteString(`{"type":"assistant","timestamp":"2026-10-20T10:02:00.000Z","cwd":"/work/app","message":{"id":"msg_D","model":"claude-opus-5-5","usage":{"input_tokens":7,`)
	if part, _ := sc.Scan(env, time.Time{}, onlyClaude); len(part) != 0 {
		t.Fatalf("a partial line was read: %+v", part)
	}
	f.WriteString(`"output_tokens":9}}}` + "\n")
	f.Close()
	next, _ := sc.Scan(env, time.Time{}, onlyClaude)
	if len(next) != 1 || next[0].ID != "claude:msg_D" || next[0].Input != 7 || next[0].Dir != "/work/app" || next[0].Conversation != "s-claude-1" {
		t.Fatalf("appended: %+v", next)
	}
}
