package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
)

const goaSwitchConvo = "11111111-2222-3333-4444-555555555555"

// goaSwitchRig is a host whose GOA runs on stand-ins that record how each start went.
type goaSwitchRig struct {
	t         *testing.T
	home, out string
	srv       *Server
	c         *client.Client
	goa       proto.SessionInfo
}

// goaSwitchStandIns puts programs named names on the PATH that say a version, end at once
// with GOA_FAIL set, else write what they started with to out and wait.
func goaSwitchStandIns(t *testing.T, out string, names ...string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = --version ] && { echo 2.0.0; exit 0; }\n[ -n \"$GOA_FAIL\" ] && { echo 'No conversation found'; exit 3; }\n" +
		"printf '%s\\n' \"$@\" > \"" + out + "/$ENOWX_SESSION.args.tmp\"\nenv > \"" + out + "/$ENOWX_SESSION.env.tmp\"\n" +
		"mv \"" + out + "/$ENOWX_SESSION.args.tmp\" \"" + out + "/$ENOWX_SESSION.args\"\nmv \"" + out + "/$ENOWX_SESSION.env.tmp\" \"" + out + "/$ENOWX_SESSION.env\"\nexec sleep 300\n"
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func newGOASwitchRig(t *testing.T, agentID string) *goaSwitchRig {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home, out := tokensHome(t), t.TempDir()
	goaSwitchStandIns(t, out, "claude", "codex", "opencode")
	old := goaSwitchSettle
	goaSwitchSettle = time.Second
	t.Cleanup(func() { goaSwitchSettle = old })
	srv := startServer(t)
	c := connect(t, srv, nil)
	g, err := c.CreateSession(proto.SessionCreateParams{Agent: agentID, GOA: true, Cols: 100, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	started(t, out, g.ID)
	return &goaSwitchRig{t: t, home: home, out: out, srv: srv, c: c, goa: g}
}

// said gives GOA's Claude Code a conversation on disk.
func (r *goaSwitchRig) said() string {
	r.t.Helper()
	folder := filepath.Join(r.home, ".claude", "projects", transcript.ClaudeFolder(r.goa.Dir))
	if err := os.MkdirAll(folder, 0o700); err != nil {
		r.t.Fatal(err)
	}
	path := filepath.Join(folder, goaSwitchConvo+".jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"user","message":{"role":"user","content":"hello"},"cwd":"`+r.goa.Dir+`"}`+"\n"), 0o600); err != nil {
		r.t.Fatal(err)
	}
	return path
}

func (r *goaSwitchRig) account(name string) proto.Account {
	r.t.Helper()
	acc, err := r.c.AddAccount("claude", name)
	if err != nil {
		r.t.Fatal(err)
	}
	return acc
}

// change asks for p, then what the agent started with: nil unless it restarted.
func (r *goaSwitchRig) change(p proto.GOASwitchParams) (proto.GOASwitchResult, []string, map[string]string, error) {
	r.t.Helper()
	for _, ext := range []string{".args", ".env"} {
		_ = os.Remove(filepath.Join(r.out, r.goa.ID+ext))
	}
	var res proto.GOASwitchResult
	if err := r.c.Call(proto.MethodGOASwitch, p, &res); err != nil {
		return res, nil, nil, err
	}
	if !res.Restarted {
		return res, nil, nil, nil
	}
	args, env := started(r.t, r.out, r.goa.ID)
	return res, args, env, nil
}

// untouched says GOA wasn't restarted by what was just refused.
func (r *goaSwitchRig) untouched(want proto.SessionInfo) {
	r.t.Helper()
	now := r.info()
	if now.Run != want.Run || now.Exited || now.Agent != want.Agent || now.Account != want.Account {
		r.t.Fatalf("GOA changed: run %d → %d, exited %v, %s/%q → %s/%q", want.Run, now.Run, now.Exited, want.Agent, want.Account, now.Agent, now.Account)
	}
}

func (r *goaSwitchRig) info() proto.SessionInfo {
	r.t.Helper()
	g, ok := r.srv.goaSession()
	if !ok {
		r.t.Fatal("GOA is gone")
	}
	return g
}

func goaHeldOnClaude(t *testing.T, srv *Server, g proto.SessionInfo, args []string, env map[string]string) {
	t.Helper()
	if !slices.Contains(args, "--strict-mcp-config") || !slices.Contains(args, agent.GOAClaudeTools) || env[proto.EnvGOA] != "1" || env["ENABLE_TOOL_SEARCH"] != "false" {
		t.Fatalf("not held: %q, ENOWX_GOA %q, tool search %q", args, env[proto.EnvGOA], env["ENABLE_TOOL_SEARCH"])
	}
	d, _ := srv.goaHook(g, proto.HookEvent{Event: proto.HookToolBefore, Tool: "Bash", Command: "go version"})
	if !d.Block {
		t.Fatal("its hook lets Bash through")
	}
}

// The owner's rule: another config goes on with the same conversation. The board, its
// waiting lines and its workers don't notice.
func TestGOAConfigChangeContinuesTheConversation(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	r.said()
	acc := r.account("Work")
	worker, err := r.c.CreateSession(proto.SessionCreateParams{Agent: "codex", Dir: t.TempDir(), GOAWorker: r.goa.ID, Cols: 80, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	task, err := r.srv.goa.board.Add(proto.GOATask{Title: "the task", Session: worker.ID, Status: proto.GOATaskWorking})
	if err != nil {
		t.Fatal(err)
	}
	later := goaLine{Text: "[enowX GOA] held", Session: worker.ID, HoldUntil: time.Now().Add(time.Hour)}
	r.srv.goa.mu.Lock()
	r.srv.goa.tell = append(r.srv.goa.tell, later)
	r.srv.goa.mu.Unlock()
	r.srv.saveGOATell()
	before := r.info()

	check, _, _, err := r.change(proto.GOASwitchParams{Config: "Work", Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if check.Restarted || check.Conversation != proto.GOAConversationSame || check.Config != "Work" || check.Level != proto.GOAEnforced {
		t.Fatalf("check: %+v", check)
	}
	r.untouched(before)

	res, args, env, err := r.change(proto.GOASwitchParams{Config: "Work"})
	if err != nil {
		t.Fatal(err)
	}
	g := r.info()
	if g.ID != r.goa.ID || g.Account != acc.ID || !res.Restarted || res.Conversation != proto.GOAConversationSame {
		t.Fatalf("after: %+v, %+v", g, res)
	}
	if i := slices.Index(args, "--resume"); i < 0 || args[i+1] != goaSwitchConvo {
		t.Fatalf("not on its conversation: %q", args)
	}
	if env["CLAUDE_CONFIG_DIR"] != r.srv.accounts.Home(acc.ID) {
		t.Fatalf("not on the config: %q", env["CLAUDE_CONFIG_DIR"])
	}
	goaHeldOnClaude(t, r.srv, g, args, env)

	w, err := r.srv.sessions.Get(worker.ID)
	if err != nil || w.Info().Exited || w.Info().Run != worker.Run {
		t.Fatalf("the worker noticed: %v %+v", err, w.Info())
	}
	if !r.srv.goa.board.Has(worker.ID) || r.srv.goaBoard().Tasks[0].ID != task.ID {
		t.Fatal("the board lost its task")
	}
	r.srv.goa.mu.Lock()
	waiting := slices.ContainsFunc(r.srv.goa.tell, func(l goaLine) bool { return l.Text == later.Text })
	switching := r.srv.goa.switching
	r.srv.goa.mu.Unlock()
	if !waiting || switching {
		t.Fatalf("the waiting line: kept %v, still held %v", waiting, switching)
	}
	if data, err := os.ReadFile(r.srv.goa.tellFile); err != nil || !strings.Contains(string(data), later.Text) {
		t.Fatalf("goa-tell.json lost it: %v", err)
	}

	// And back to the official one, the conversation still the same.
	res, args, env, err = r.change(proto.GOASwitchParams{Config: proto.GOAConfigOfficial})
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.Index(args, "--resume"); i < 0 || args[i+1] != goaSwitchConvo || env["CLAUDE_CONFIG_DIR"] != "" || r.info().Account != "" || res.Config != "" {
		t.Fatalf("back on the official: %q, %q, %+v", args, env["CLAUDE_CONFIG_DIR"], res)
	}
}

// A config that keeps conversations of its own can't go on: GOA goes on as it was.
func TestGOAConfigThatCantResumeLeavesItRunning(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	r.said()
	acc := r.account("Apart")
	if err := os.MkdirAll(filepath.Join(r.srv.accounts.Home(acc.ID), "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	before := r.info()
	for _, check := range []bool{true, false} {
		if _, _, _, err := r.change(proto.GOASwitchParams{Config: acc.Name, Check: check}); err == nil || !strings.Contains(err.Error(), "keeps running as it was") {
			t.Fatalf("check %v: %v", check, err)
		}
		r.untouched(before)
	}
}

// What starting shows only by ending puts it back as it was, on its conversation.
func TestGOAConfigThatEndsComesBackAsItWas(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	r.said()
	acc := r.account("Broken")
	acc.Env = []string{"GOA_FAIL=1"}
	if _, err := r.c.SaveConfig(acc); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := r.change(proto.GOASwitchParams{Config: acc.Name})
	if err == nil || !strings.Contains(err.Error(), "back as it was") {
		t.Fatalf("got %v", err)
	}
	args, env := started(t, r.out, r.goa.ID)
	if i := slices.Index(args, "--resume"); i < 0 || args[i+1] != goaSwitchConvo || env["GOA_FAIL"] != "" || env["CLAUDE_CONFIG_DIR"] != "" {
		t.Fatalf("came back as: %q", args)
	}
	g := r.info()
	if g.Exited || g.Account != "" {
		t.Fatalf("came back: %+v", g)
	}
	goaHeldOnClaude(t, r.srv, g, args, env)
}

// Another agent needs a yes, as the conversation doesn't carry over; then it starts new,
// held as GOA, with the board as it was, and a later return to the first agent doesn't
// pick up the old conversation.
func TestGOAAgentChangeStartsANewConversation(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	path := r.said()
	before := r.info()
	res, _, _, err := r.change(proto.GOASwitchParams{Agent: "codex"})
	if err != nil || res.Restarted || res.Conversation != proto.GOAConversationNew {
		t.Fatalf("without a yes: %+v, %v", res, err)
	}
	r.untouched(before)

	res, args, env, err := r.change(proto.GOASwitchParams{Agent: "codex", NewConversation: true})
	if err != nil {
		t.Fatal(err)
	}
	g := r.info()
	if g.ID != r.goa.ID || g.Agent != "codex" || !g.GOA || res.Level != proto.GOAEnforced {
		t.Fatalf("on codex: %+v %+v", g, res)
	}
	if slices.Contains(args, "resume") || !slices.Contains(args, "--sandbox") || env[proto.EnvGOA] != "1" {
		t.Fatalf("not new, or not held: %q", args)
	}
	if b := r.srv.goaBoard(); b.Agent != "codex" || b.Session != r.goa.ID {
		t.Fatalf("board: %+v", b)
	}

	// Old enough for the look to tell it apart.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, args, _, err = r.change(proto.GOASwitchParams{Agent: "claude", NewConversation: true}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(args, "--resume") || slices.Contains(args, goaSwitchConvo) {
		t.Fatalf("back on Claude Code it took the old conversation: %q", args)
	}
	if got := r.srv.transcriptPath(r.goa.ID); got != "" {
		t.Fatalf("its chat follows the old conversation: %s", got)
	}
}

// A config can't undo what holds GOA on OpenCode, which is only its environment.
func TestGOAOnOpenCodeKeepsItsHoldOnAConfig(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	cfg, err := r.c.SaveConfig(proto.Account{Agent: "opencode", Name: "Loose", Kind: proto.ConfigKindAPI, Env: []string{"OPENCODE_CONFIG_CONTENT={}", "OPENCODE_PERMISSION={}", "KEEP_ME=1"},
		Args: []string{"--agent", "build"},
		API:  &proto.ConfigAPI{BaseURL: "https://example.com/v1", Models: []proto.AgentAPIModel{{ID: "m"}}}})
	if err != nil {
		t.Fatal(err)
	}
	res, args, env, err := r.change(proto.GOASwitchParams{Agent: "opencode", Config: cfg.Name, NewConversation: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env["OPENCODE_CONFIG_CONTENT"], `"default_agent":"goa"`) || env["OPENCODE_PERMISSION"] != "" || env["KEEP_ME"] != "1" || env["OPENCODE_CONFIG"] == "" {
		t.Fatalf("its environment: content %q, permission %q, keep %q, config %q", env["OPENCODE_CONFIG_CONTENT"], env["OPENCODE_PERMISSION"], env["KEEP_ME"], env["OPENCODE_CONFIG"])
	}
	if slices.Contains(args, "build") || res.Level != proto.GOAEnforced || res.Note == "" {
		t.Fatalf("flags %q, level %s, note %q", args, res.Level, res.Note)
	}
	// Coming back by way of another config: the same.
	_, _, env, err = r.change(proto.GOASwitchParams{Config: proto.GOAConfigOfficial})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env["OPENCODE_CONFIG_CONTENT"], `"default_agent":"goa"`) {
		t.Fatalf("lost: %q", env["OPENCODE_CONFIG_CONTENT"])
	}
}

// What can't be: an agent not proven, a config of another agent, GOA changing itself, a
// fallback to another agent. GOA at work needs a yes.
func TestGOAChangesRefused(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	r.said()
	r.account("Work")
	cdx, err := r.c.SaveConfig(proto.Account{Agent: "codex", Name: "Codex API", Kind: proto.ConfigKindAPI, API: &proto.ConfigAPI{BaseURL: "https://example.com/v1", Models: []proto.AgentAPIModel{{ID: "m"}}}})
	if err != nil {
		t.Fatal(err)
	}
	before := r.info()
	for name, p := range map[string]proto.GOASwitchParams{
		"not proven":      {Agent: "gemini", NewConversation: true},
		"another's":       {Config: cdx.Name},
		"unknown":         {Config: "nope"},
		"its own session": {Config: "Work", From: r.goa.ID},
	} {
		if _, _, _, err := r.change(p); err == nil {
			t.Errorf("%s: no refusal", name)
		}
		r.untouched(before)
	}
	if _, err := r.srv.switchAccount(proto.AccountSwitchParams{Session: r.goa.ID, Agent: "codex"}); err == nil {
		t.Error("a fallback took GOA to another agent")
	}
	r.untouched(before)

	sess, err := r.srv.sessions.Get(r.goa.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.srv.sessions.SetState(sess, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	before = r.info()
	res, _, _, err := r.change(proto.GOASwitchParams{Config: "Work"})
	if err != nil || res.Restarted || !res.AtWork {
		t.Fatalf("at work, no yes: %+v, %v", res, err)
	}
	r.untouched(before)
	if res, _, _, err = r.change(proto.GOASwitchParams{Config: "Work", Force: true}); err != nil || !res.Restarted {
		t.Fatalf("at work, with a yes: %+v, %v", res, err)
	}
}

// The hook waiting to wake GOA ends with nothing, so no line goes to a process about to go.
func TestGOAChangeTakesNoLineToTheOldAgent(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	r.said()
	r.account("Work")
	got := make(chan proto.GOAWakeReply, 1)
	go func() { got <- r.srv.goaWake(r.goa.ID, make(chan struct{})) }()
	waitFor(t, "the hook", func() bool {
		r.srv.goa.mu.Lock()
		defer r.srv.goa.mu.Unlock()
		return r.srv.goa.waker != nil
	})
	if !r.srv.goaHoldLines() {
		t.Fatal("not held")
	}
	if reply := <-got; reply.Text != "" {
		t.Fatalf("the hook took %q", reply.Text)
	}
	r.srv.goa.mu.Lock()
	r.srv.goa.tell = append(r.srv.goa.tell, goaLine{Text: "while it changes"})
	r.srv.goa.mu.Unlock()
	if said := r.srv.goaLinesToSay(); len(said) != 0 {
		t.Fatalf("given while changing: %+v", said)
	}
	if r.srv.goaHoldLines() {
		t.Fatal("two changes at once")
	}
	r.srv.goaReleaseLines()
}

// A mode the user gave GOA stays over a change of config, which decides its model alone.
func TestGOAConfigKeepsItsMode(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	r.said()
	r.account("Work")
	sess, err := r.srv.sessions.Get(r.goa.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.srv.sessions.SetMode(sess, "acceptEdits")
	res, args, _, err := r.change(proto.GOASwitchParams{Config: "Work"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != "acceptEdits" || !slices.Contains(args, "acceptEdits") {
		t.Fatalf("mode %q, args %q", res.Mode, args)
	}
}
