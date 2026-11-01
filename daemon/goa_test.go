package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/rag"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// goaBoardOf is srv's GOA board.
func goaBoardOf(t *testing.T, c *client.Client) proto.GOABoard {
	t.Helper()
	var b proto.GOABoard
	if err := c.Call(proto.MethodGOABoard, nil, &b); err != nil {
		t.Fatal(err)
	}
	return b
}

// GOA on host A hands a task to a new session on host B, in a project it
// opens there, through the app; controls that session (renames it, types
// into it, reads it); when the app tells A that B's worker finished, the
// board says done, with its result, and GOA is told in its own
// conversation, not by polling.
func TestGOAControlsAnotherHostEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fakeAgents(t, "claude")
	a, b := startServer(t), startServer(t)
	standInApp(t, a, map[string]*client.Client{b.daemonID: connect(t, b, nil), a.daemonID: connect(t, a, nil)}, func(proto.HostCallParams) bool { return true })
	ca := connect(t, a, nil)

	g, err := ca.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	if !g.GOA || g.Project != "" || g.Dir != a.goaDir() {
		t.Fatalf("GOA's session: goa %v, project %q, dir %q (want %q)", g.GOA, g.Project, g.Dir, a.goaDir())
	}
	if _, err := ca.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true}); err == nil || !strings.Contains(err.Error(), "already runs") {
		t.Fatalf("a second GOA: %v", err)
	}

	proj := t.TempDir()
	var task proto.GOATask
	if err := ca.Call(proto.MethodGOADelegate, proto.GOADelegateParams{From: g.ID, Host: b.daemonID, Agent: "claude", Project: proj,
		Title: "Totals", Prompt: "Fix the rounding of the totals", NoRoute: true}, &task); err != nil {
		t.Fatal(err)
	}
	worker, err := b.sessions.Get(task.Session)
	if err != nil {
		t.Fatalf("the worker isn't on B: %v", err)
	}
	if _, err := a.sessions.Get(task.Session); err == nil {
		t.Fatal("the worker is on A")
	}
	if info := worker.Info(); info.Project != proj || info.GOAWorker != g.ID {
		t.Fatalf("the worker: project %q, works for %q", info.Project, info.GOAWorker)
	}
	if !slices.ContainsFunc(b.projects.List(), func(p proto.ProjectInfo) bool { return p.Path == proj }) {
		t.Fatal("the project wasn't opened on B")
	}
	if board := goaBoardOf(t, ca); board.Session != g.ID || len(board.Tasks) != 1 || board.Tasks[0].Status != proto.GOATaskWorking {
		t.Fatalf("the board: %+v", board)
	}

	there, err := ca.OnHost(b.daemonID, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := there.RenameSession(task.Session, "totals"); err != nil {
		t.Fatal(err)
	}
	if err := there.Input(task.Session, "and add a test", false, true); err != nil {
		t.Fatal(err)
	}
	waitScreen(t, there, task.Session, "heard: and add a test")
	if worker.Info().Name != "totals" {
		t.Fatal("not renamed")
	}

	// The worker's turn ends on B; the app tells A.
	if err := b.sessions.SetState(worker, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	if err := ca.Call(proto.MethodGOAWorker, proto.GOAWorkerParams{Host: b.daemonID, Session: worker.Info()}, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the task done, with its result", func() bool {
		tasks := goaBoardOf(t, ca).Tasks
		return len(tasks) == 1 && tasks[0].Status == proto.GOATaskDone && strings.Contains(tasks[0].Result, "heard: and add a test")
	})
	waitScreen(t, ca, g.ID, "[enowX GOA] Task "+task.ID)

	// Settled by GOA; the log on A has what it did on B.
	var settled proto.GOATask
	if err := ca.Call(proto.MethodGOATask, proto.GOATaskParams{ID: task.ID, Status: proto.GOATaskDone, Result: "rounding fixed"}, &settled); err != nil || settled.Result != "rounding fixed" {
		t.Fatalf("settled %+v %v", settled, err)
	}
	log, _ := ca.ControlLog()
	methods := map[string]bool{}
	for _, e := range log {
		if e.Host == b.daemonID && e.From == g.ID && e.Err == "" {
			methods[e.Method] = true
		}
	}
	for _, m := range []string{proto.MethodProjectAdd, proto.MethodSessionCreate, proto.MethodSessionRename, proto.MethodSessionTranscript} {
		if !methods[m] {
			t.Errorf("GOA's %s on B isn't in the log: %v", m, methods)
		}
	}
}

// GOA's own tool calls: enowX's MCP tools, "$ENOWX_BIN" commands and its
// tools for files run; a shell command of its own, a subagent are refused
// before they run, by its agent's hooks. Reading is allowed unasked, unless
// of a secret; an edit is left to its prompt.
func TestGOARefusesDoingTheWorkItself(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	g, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: t.TempDir(), Cols: 80, Rows: 24,
		Meta: session.Meta{Agent: "claude", GOA: true}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(session.EnvSession, g.ID())
	refused := []string{
		`{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}`,
		`{"tool_name":"Bash","tool_input":{"command":"\"$ENOWX_BIN\" ls; sed -i s/a/b/ totals.go"}}`,
		`{"tool_name":"Task","tool_input":{"prompt":"fix it"}}`,
	}
	for _, in := range refused {
		code, _, reason := hookOf(in, "--event", proto.HookToolBefore)
		if code == 0 || !strings.Contains(reason, "GOD of Agents") {
			t.Errorf("%s: exit %d, %q", in, code, reason)
		}
	}
	allowed := []string{
		`{"tool_name":"mcp__enowx__goa_delegate","tool_input":{"prompt":"Fix the totals"}}`,
		`{"tool_name":"Bash","tool_input":{"command":"\"$ENOWX_BIN\" goa delegate --agent codex \"Fix the totals; add a test\""}}`,
	}
	for _, in := range allowed {
		if code, _, reason := hookOf(in, "--event", proto.HookToolBefore); code != 0 {
			t.Errorf("%s: refused: %q", in, reason)
		}
	}
	for in, unasked := range map[string]bool{
		`{"tool_name":"Read","tool_input":{"file_path":"/src/shop/totals.go"}}`:                                   true,
		`{"tool_name":"Read","tool_input":{"file_path":"/src/shop/.env"}}`:                                        false,
		`{"tool_name":"Edit","tool_input":{"file_path":"/src/shop/totals.go","old_string":"a","new_string":"b"}}`: false,
		`{"tool_name":"Write","tool_input":{"file_path":"/src/shop/plan.md","content":"# Plan"}}`:                 false,
	} {
		code, out, reason := hookOf(in, "--event", proto.HookToolBefore)
		if code != 0 || strings.Contains(out, `"permissionDecision":"allow"`) != unasked {
			t.Errorf("%s: exit %d, %q, %q; unasked: %v", in, code, out, reason, unasked)
		}
	}
	// A worker's own edits aren't GOA's: the same hook lets them by.
	w, _ := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: t.TempDir(), Meta: session.Meta{Agent: "claude"}})
	t.Setenv(session.EnvSession, w.ID())
	if code, _, reason := hookOf(`{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}`, "--event", proto.HookToolBefore); code != 0 {
		t.Fatalf("a worker's command refused: %q", reason)
	}
}

// The judge picks each delegated prompt's model and effort: a typo goes
// to the cheapest model on low effort, a migration to the strongest on
// high; near a token budget, a step cheaper; a model chosen by hand wins.
// The worker starts on what was picked.
func TestGOARoutesEachPrompt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fakeAgents(t, "claude")
	srv := startServer(t)
	judgeLater(t, srv)
	c := connect(t, srv, nil)
	delegate := func(p proto.GOADelegateParams) proto.GOATask {
		t.Helper()
		p.Agent, p.Project = "claude", t.TempDir()
		var task proto.GOATask
		if err := c.Call(proto.MethodGOADelegate, p, &task); err != nil {
			t.Fatal(err)
		}
		return task
	}
	easy := delegate(proto.GOADelegateParams{Prompt: "fix the typo in the README"})
	hard := delegate(proto.GOADelegateParams{Prompt: "migrate the orders table to the new schema"})
	if easy.Model != "claude-haiku-5-5" || easy.Effort != "low" || easy.Difficulty != proto.VerdictEasy || easy.Why == "" {
		t.Fatalf("easy: %+v", easy)
	}
	if hard.Model != "claude-opus-5-5" || hard.Effort != "high" || hard.Difficulty != proto.VerdictHard {
		t.Fatalf("hard: %+v", hard)
	}
	sess, _ := srv.sessions.Get(easy.Session)
	if cmd := strings.Join(sess.Info().Command, " "); !strings.Contains(cmd, "claude-haiku-5-5") {
		t.Fatalf("the easy task's session doesn't start on its model: %s", cmd)
	}
	srv.judge.Cheaper = func() bool { return true }
	near := delegate(proto.GOADelegateParams{Prompt: "migrate the orders table to the new schema"})
	if near.Model != "claude-sonnet-5-5" || near.Effort != "medium" || !strings.Contains(near.Why, "budget") {
		t.Fatalf("near a budget: %+v", near)
	}
	own := delegate(proto.GOADelegateParams{Prompt: "migrate the orders table", Model: "claude-fable-5-1", Effort: "max"})
	if own.Model != "claude-fable-5-1" || !own.Override {
		t.Fatalf("chosen by hand: %+v", own)
	}
}

// What GOA keeps goes to its own collection, never a project's; it may
// search a project, read-only, which another session may not.
func TestGOARAGCollectionIsItsOwn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fakeAgents(t, "claude")
	srv := startServer(t)
	c := connect(t, srv, nil)
	proj := t.TempDir()
	if _, err := c.AddProject(proj); err != nil {
		t.Fatal(err)
	}
	g, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true})
	if err != nil {
		t.Fatal(err)
	}
	var m proto.RAGMemory
	if err := c.Call(proto.MethodGOARemember, proto.GOARememberParams{Session: g.ID, Kind: proto.GOAKindPreference, Text: "The user wants small pull requests"}, &m); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(proto.MethodRAGRemember, proto.RAGRememberParams{RAGRef: proto.RAGRef{Session: g.ID}, Text: "Task t1 went to codex on the VPS"}, nil); err != nil {
		t.Fatal(err)
	}
	own := srv.ragSettings.Memories(rag.ProjectID(srv.goaDir()))
	if len(own) != 2 || !strings.HasPrefix(own[0].Text, "[preference] ") {
		t.Fatalf("GOA's collection: %+v", own)
	}
	if got := srv.ragSettings.Memories(rag.ProjectID(proj)); len(got) != 0 {
		t.Fatalf("a project got GOA's memories: %+v", got)
	}
	if err := c.Call(proto.MethodRAGRemember, proto.RAGRememberParams{RAGRef: proto.RAGRef{Session: g.ID, Project: proj}, Text: "x"}, nil); err == nil {
		t.Fatal("GOA wrote to a project's collection")
	}
	var hits []proto.RAGHit
	if err := c.Call(proto.MethodGOARecall, proto.GOARecallParams{Session: g.ID, Query: "small pull requests"}, &hits); err != nil || len(hits) == 0 || !strings.Contains(hits[0].Snippet, "small pull requests") {
		t.Fatalf("recall: %+v %v", hits, err)
	}
	// Reading a project: GOA may, another session may not.
	err = c.Call(proto.MethodRAGSearch, proto.RAGSearchParams{RAGRef: proto.RAGRef{Session: g.ID, Project: proj}, Query: "totals"}, &hits)
	if err != nil && strings.Contains(err.Error(), "own project only") {
		t.Fatalf("GOA reading a project: %v", err)
	}
	other, _ := srv.sessions.Create(session.Spec{Command: []string{"cat"}, Dir: t.TempDir(), Meta: session.Meta{Agent: "claude"}})
	err = c.Call(proto.MethodRAGSearch, proto.RAGSearchParams{RAGRef: proto.RAGRef{Session: other.ID(), Project: proj}, Query: "totals"}, &hits)
	if err == nil || !strings.Contains(err.Error(), "own project only") {
		t.Fatalf("another session reading a project: %v", err)
	}
	if err := c.Call(proto.MethodGOARemember, proto.GOARememberParams{Session: other.ID(), Text: "x"}, nil); err == nil {
		t.Fatal("another session wrote to GOA's collection")
	}
}

// A worker's command the judge is sure of runs unasked; one that
// publishes is the user's: a sound and a banner, its ask marked urgent,
// and GOA can't answer it for them.
func TestGOAWorkersPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	on := proto.DecideUse{On: true}
	if err := srv.judge.Settings().Set(proto.DecideSettings{Policy: proto.DecideLocalOnly,
		Own:  []proto.Judge{{Kind: proto.JudgeLocal, URL: judgeServer(t), Model: "judge"}},
		Uses: map[string]proto.DecideUse{proto.DecideApprove: on}}); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var banners []proto.NotifySoundParams
	c := connect(t, srv, func(ev proto.Event) {
		if ev.Name == proto.EventNotifySound {
			var p proto.NotifySoundParams
			json.Unmarshal(ev.Data, &p)
			mu.Lock()
			banners = append(banners, p)
			mu.Unlock()
		}
	})
	w, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: t.TempDir(), Cols: 80, Rows: 24,
		Meta: session.Meta{Agent: "claude", Name: "totals", GOAWorker: "goa-session"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(session.EnvSession, w.ID())

	code, out, _ := hookOf(`{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}`, "--event", proto.HookToolBefore)
	if code != 0 || !strings.Contains(out, `"permissionDecision":"allow"`) {
		t.Fatalf("the judge's yes: %d %q", code, out)
	}
	code, _, reason := hookOf(`{"tool_name":"Bash","tool_input":{"command":"rm -rf ~"}}`, "--event", proto.HookToolBefore)
	if code == 0 || !strings.Contains(reason, "judge refused") {
		t.Fatalf("the judge's no: %d %q", code, reason)
	}
	code, out, _ = hookOf(`{"tool_name":"Bash","tool_input":{"command":"git push origin main"}}`, "--event", proto.HookToolBefore)
	if code != 0 || strings.Contains(out, "allow") {
		t.Fatalf("a push: left to its prompt, not allowed: %d %q", code, out)
	}
	waitFor(t, "a banner for the user", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.ContainsFunc(banners, func(p proto.NotifySoundParams) bool {
			return p.Session == w.ID() && p.Kind == proto.NotifyInput && strings.Contains(p.Text, "Urgent") && strings.Contains(p.Text, "git push")
		})
	})
	if err := srv.sessions.SetState(w, proto.StateWaiting); err != nil {
		t.Fatal(err)
	}
	ask, err := c.SessionAsk(w.ID())
	if err != nil || !ask.Waiting || ask.Urgent == "" {
		t.Fatalf("the ask: %+v %v", ask, err)
	}
	if err := c.HostCall("", "goa-session", proto.MethodSessionAnswer, proto.SessionAnswerParams{ID: w.ID(), Answer: "allow"}, nil); err == nil || !strings.Contains(err.Error(), "user's to answer") {
		t.Fatalf("GOA answering the user's: %v", err)
	}
}

// GOA runs on Claude Code and on Codex, each in GOA's folder with its
// guide: Claude Code with only its tools for files and enowX's MCP server,
// its hooks refusing the rest; Codex's sandbox writing, enowX's MCP server
// approved, its hooks set. Both are enforced on the board.
func TestGOAOnClaudeCodeAndCodex(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n\\036\\n' \"$a\"; done > \"$0.args\"\nenv > \"$0.env\"\nwhile read -r line; do echo \"heard: $line\"; done\n"
	for _, n := range []string{"claude", "codex"} {
		os.WriteFile(filepath.Join(bin, n), []byte(script), 0o755)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	launched := func(name string) ([]string, string) {
		t.Helper()
		var data, env []byte
		waitFor(t, name+" started", func() bool {
			data, _ = os.ReadFile(filepath.Join(bin, name+".args"))
			env, _ = os.ReadFile(filepath.Join(bin, name+".env"))
			return len(data) > 0 && len(env) > 0
		})
		args := strings.Split(strings.TrimSuffix(string(data), "\n\x1e\n"), "\n\x1e\n")
		return args, string(env)
	}
	after := func(args []string, flag string) string {
		if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
			return args[i+1]
		}
		return "<none>"
	}

	cl, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true})
	if err != nil {
		t.Fatal(err)
	}
	args, env := launched("claude")
	if after(args, "--tools") != agent.GOAClaudeTools || !slices.Contains(args, "--strict-mcp-config") || after(args, "--allowedTools") != "mcp__enowx" {
		t.Errorf("Claude Code's tools aren't only those for files: %q", args)
	}
	if !strings.Contains(after(args, "--mcp-config"), `"ENOWX_GOA":"1"`) || !strings.Contains(after(args, "--settings"), "PreToolUse") {
		t.Errorf("Claude Code: no enowX MCP server or hooks: %q", args)
	}
	if !strings.Contains(after(args, "--append-system-prompt"), "You are GOD of Agents") {
		t.Errorf("Claude Code: no GOA guide")
	}
	if !strings.Contains(env, "ENABLE_TOOL_SEARCH=false") || !strings.Contains(env, "PWD="+srv.goaDir()) && cl.Dir != srv.goaDir() {
		t.Errorf("Claude Code's environment or folder: %s", cl.Dir)
	}
	if b := goaBoardOf(t, c); b.Session != cl.ID || b.Agent != "claude" || b.Level != proto.GOAEnforced {
		t.Errorf("the board, on Claude Code: %+v", b)
	}
	if err := c.KillSession(cl.ID); err != nil {
		t.Fatal(err)
	}

	cx, err := c.CreateSession(proto.SessionCreateParams{Agent: "codex", GOA: true})
	if err != nil {
		t.Fatal(err)
	}
	args, _ = launched("codex")
	joined := strings.Join(args, " ")
	if after(args, "--sandbox") != "workspace-write" || !strings.Contains(joined, "mcp_servers.enowx.command=") ||
		!strings.Contains(joined, `default_tools_approval_mode="approve"`) || !strings.Contains(joined, "hooks.PreToolUse=") {
		t.Errorf("Codex as GOA: %q", args)
	}
	if !strings.Contains(joined, "developer_instructions=") || !strings.Contains(joined, "You are GOD of Agents") {
		t.Errorf("Codex: no GOA guide")
	}
	if cx.Dir != srv.goaDir() {
		t.Errorf("Codex's folder %q", cx.Dir)
	}
	if b := goaBoardOf(t, c); b.Session != cx.ID || b.Agent != "codex" || b.Level != proto.GOAEnforced {
		t.Errorf("the board, on Codex: %+v", b)
	}
	var agents []proto.GOAAgent
	if err := c.Call(proto.MethodGOAAgents, nil, &agents); err != nil {
		t.Fatal(err)
	}
	levels := map[string]string{}
	for _, a := range agents {
		levels[a.Agent] = a.Level
	}
	if levels["claude"] != proto.GOAEnforced || levels["codex"] != proto.GOAEnforced || levels["aider"] != proto.GOAWatched {
		t.Errorf("levels: %v", levels)
	}
}

// GOA can't hand a task to itself: its prompt would land in its own
// conversation, its model switched, the task never settled.
func TestGOADelegatesToAnotherSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	c := connect(t, srv, nil)
	g, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: t.TempDir(), Cols: 80, Rows: 24, Meta: session.Meta{Agent: "codex", GOA: true}})
	if err != nil {
		t.Fatal(err)
	}
	var task proto.GOATask
	err = c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{From: g.ID(), Session: g.ID(), Prompt: "Write the plan", NoRoute: true}, &task)
	if err == nil || !strings.Contains(err.Error(), "not GOD of Agents' own") {
		t.Fatalf("delegated to itself: %+v %v", task, err)
	}
	if tasks := srv.goa.board.List(); len(tasks) != 0 {
		t.Fatalf("on the board: %+v", tasks)
	}
}
