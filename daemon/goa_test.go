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
	"time"

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

	// The worker's turn ends on B; the app tells A. Just after a task is given, a turn's end
	// from another host is taken for the turn before's.
	time.Sleep(goaTurnLeast)
	if err := b.sessions.SetState(worker, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	if err := ca.Call(proto.MethodGOAWorker, proto.GOAWorkerParams{Host: b.daemonID, Session: worker.Info()}, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the turn ended, no outcome yet", func() bool {
		tasks := goaBoardOf(t, ca).Tasks
		return len(tasks) == 1 && tasks[0].Status == proto.GOATaskTurnEnded && tasks[0].Result == "" && strings.Contains(tasks[0].Said, "heard: and add a test")
	})
	// This GOA's screen shows no composer to type into, so the line waits for its hooks.
	waitFor(t, "GOA told", func() bool {
		for _, line := range a.goaWaitingLines() {
			if strings.Contains(line, "[enowX GOA] Task "+task.ID) {
				return true
			}
		}
		return false
	})

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
		`{"tool_name":"Edit","tool_input":{"file_path":"/src/shop/totals.go","old_string":"a","new_string":"b"}}`,
		`{"tool_name":"Write","tool_input":{"file_path":"/src/shop/plan.md","content":"# Plan"}}`,
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
		`{"tool_name":"Read","tool_input":{"file_path":"/src/shop/totals.go"}}`:                         true,
		`{"tool_name":"Read","tool_input":{"file_path":"/src/shop/.env"}}`:                              false,
		`{"tool_name":"Edit","tool_input":{"file_path":"totals.go","old_string":"a","new_string":"b"}}`: false,
		`{"tool_name":"Write","tool_input":{"file_path":"plan.md","content":"# Plan"}}`:                 false,
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
	hookOf(`{"tool_name":"Bash"}`, "--event", proto.HookPermission) // it really asks
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

// A worker's banner says it really asks: not for a tool call its agent runs
// unasked, in bypass, whether the judge was unsure or the command urgent; and
// for one left to the user only once its prompt shows, not before.
func TestGOAWorkerBannerOnlyWhenItReallyAsks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	if err := srv.judge.Settings().Set(proto.DecideSettings{Policy: proto.DecideLocalOnly,
		Own:  []proto.Judge{{Kind: proto.JudgeLocal, URL: judgeServer(t), Model: "judge"}},
		Uses: map[string]proto.DecideUse{proto.DecideApprove: {On: true, MinConfidence: 0.99}}}); err != nil { // the test judge says 0.95: never sure
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
	urgent := func() int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, p := range banners {
			if p.Session == w.ID() && strings.Contains(p.Text, "Urgent") {
				n++
			}
		}
		return n
	}

	for _, command := range []string{"go test ./...", "git push origin main"} {
		hookOf(`{"tool_name":"Bash","tool_input":{"command":"`+command+`"},"permission_mode":"bypassPermissions"}`, "--event", proto.HookToolBefore)
		if why := srv.goaUrgentOf(w.ID()); why != "" {
			t.Fatalf("%s in bypass is marked the user's: %q", command, why)
		}
	}
	// A call answered after them: what was broadcast before it has arrived.
	if _, err := c.SessionAsk(w.ID()); err != nil {
		t.Fatal(err)
	}
	if n := urgent(); n != 0 {
		t.Fatalf("%d banners for a worker in bypass", n)
	}

	hookOf(`{"tool_name":"Bash","tool_input":{"command":"go test ./..."},"permission_mode":"default"}`, "--event", proto.HookToolBefore)
	if srv.goaUrgentOf(w.ID()) == "" {
		t.Fatal("unsure, out of bypass: GOA may answer it")
	}
	if _, err := c.SessionAsk(w.ID()); err != nil {
		t.Fatal(err)
	}
	if n := urgent(); n != 0 {
		t.Fatalf("%d banners before it asks", n)
	}
	hookOf(`{"tool_name":"Bash","permission_mode":"default"}`, "--event", proto.HookPermission)
	waitFor(t, "the banner once it asks", func() bool { return urgent() == 1 })
	hookOf(`{"tool_name":"Bash","permission_mode":"default"}`, "--event", proto.HookPermission)
	if _, err := c.SessionAsk(w.ID()); err != nil {
		t.Fatal(err)
	}
	if n := urgent(); n != 1 {
		t.Fatalf("%d banners for one call", n)
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
	held := func(flag string) string {
		t.Helper()
		data, err := os.ReadFile(after(args, flag))
		if err != nil {
			t.Fatalf("%s: %v", flag, err)
		}
		return string(data)
	}
	if !strings.Contains(held("--mcp-config"), `"ENOWX_GOA":"1"`) || !strings.Contains(held("--settings"), "PreToolUse") {
		t.Errorf("Claude Code: no enowX MCP server or hooks: %q", args)
	}
	if !strings.Contains(held("--append-system-prompt-file"), "You are GOD of Agents") {
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

// A prompt goes to a stopped session only once it is back: the task on the
// board then means the prompt arrived. One that can't come back fails, with
// no task. With a project and no session, its own session takes the task, a
// new one only if it has none free.
func TestGOABringsSessionsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	// Up after a moment, as an agent resuming its conversation is.
	script := "#!/bin/sh\nsleep 1\necho up\nwhile read -r line; do echo \"heard: $line\"; done\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	a, b := startServer(t), startServer(t)
	ca, cb := connect(t, a, nil), connect(t, b, nil)
	standInApp(t, a, map[string]*client.Client{b.daemonID: cb}, func(proto.HostCallParams) bool { return true })
	delegate := func(p proto.GOADelegateParams) (proto.GOATask, error) {
		var task proto.GOATask
		p.NoRoute = true
		err := ca.Call(proto.MethodGOADelegate, p, &task)
		return task, err
	}
	stopped := func(c *client.Client, project string) proto.SessionInfo {
		t.Helper()
		info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Project: project, Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		waitScreen(t, c, info.ID, "up")
		if err := c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: info.ID, State: proto.StateDone}, nil); err != nil {
			t.Fatal(err)
		}
		if res, err := c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{info.ID}}); err != nil || len(res.Hibernated) != 1 {
			t.Fatalf("hibernating: %+v %v", res, err)
		}
		return info
	}
	project := func(c *client.Client) string {
		t.Helper()
		dir := gitRepo(t)
		if _, err := c.AddProject(dir); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	// Named, on this host.
	dir := project(ca)
	info := stopped(ca, dir)
	task, err := delegate(proto.GOADelegateParams{Session: info.ID, Prompt: "first task"})
	if err != nil || !task.Woken || task.Reused || task.Status != proto.GOATaskWorking {
		t.Fatalf("named: %+v %v", task, err)
	}
	waitScreen(t, ca, info.ID, "heard: first task")
	if back, _ := a.sessions.Get(info.ID); back.Info().Exited {
		t.Fatal("not brought back")
	}

	// Not named: the project's, its open task making this one wait in line.
	again, err := delegate(proto.GOADelegateParams{Project: dir, Agent: "claude", Prompt: "second task"})
	if err != nil || again.Session != info.ID || !again.Reused || again.Status != proto.GOATaskQueued {
		t.Fatalf("with its session on an open task: %+v %v", again, err)
	}
	for _, id := range []string{task.ID, again.ID} {
		if err := ca.Call(proto.MethodGOATask, proto.GOATaskParams{ID: id, Status: proto.GOATaskDone}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := ca.Call(proto.MethodSessionState, proto.SessionStateParams{ID: info.ID, State: proto.StateDone}, nil); err != nil {
		t.Fatal(err)
	}
	reused, err := delegate(proto.GOADelegateParams{Project: dir, Agent: "claude", Prompt: "third task"})
	if err != nil || !reused.Reused || reused.Woken || reused.Session != info.ID || reused.Status == proto.GOATaskQueued {
		t.Fatalf("an idle one: %+v %v", reused, err)
	}
	waitScreen(t, ca, info.ID, "heard: third task")
	// One working still takes the next, in line.
	if err := ca.Call(proto.MethodSessionState, proto.SessionStateParams{ID: reused.Session, State: proto.StateWorking}, nil); err != nil {
		t.Fatal(err)
	}
	other, err := delegate(proto.GOADelegateParams{Project: dir, Agent: "claude", Prompt: "fourth task"})
	if err != nil || other.Session != info.ID || other.Status != proto.GOATaskQueued {
		t.Fatalf("one working: %+v %v", other, err)
	}

	// On another host, through the app: named and not.
	there := project(cb)
	far := stopped(cb, there)
	remote, err := delegate(proto.GOADelegateParams{Host: b.daemonID, Project: there, Agent: "claude", Prompt: "far task"})
	if err != nil || remote.Session != far.ID || !remote.Reused || !remote.Woken {
		t.Fatalf("on another host: %+v %v", remote, err)
	}
	waitScreen(t, cb, far.ID, "heard: far task")

	// One that can't come back: an error, and nothing on the board.
	before := len(a.goa.board.List())
	plain, err := a.sessions.Create(session.Spec{Command: []string{"sh", "-c", "exit 0"}, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "it ended", func() bool { return plain.Info().Exited })
	if task, err := delegate(proto.GOADelegateParams{Session: plain.ID(), Prompt: "lost task"}); err == nil || !strings.Contains(err.Error(), "can't be brought back") {
		t.Fatalf("a session that can't come back: %+v %v", task, err)
	}
	if n := len(a.goa.board.List()); n != before {
		t.Fatalf("on the board all the same: %d, was %d", n, before)
	}
}

// What GOA hands out and how it truly ended is in its collection with no one
// saving it: found by words with RAG off, on this host's tasks and another's,
// after a restart. A turn ending isn't an outcome; a secret isn't kept; no
// project's collection is touched; and an entry can be deleted.
func TestGOARemembersItsTasks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fakeAgents(t, "claude")
	dataDir := t.TempDir()
	a, err := NewServer(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	b := startServer(t)
	ca, cb := connect(t, a, nil), connect(t, b, nil)
	standInApp(t, a, map[string]*client.Client{b.daemonID: cb}, func(proto.HostCallParams) bool { return true })
	g, err := a.sessions.Create(session.Spec{Command: []string{"cat"}, Dir: a.goaDir(), Cols: 80, Rows: 24, Meta: session.Meta{Agent: "claude", GOA: true}})
	if err != nil {
		t.Fatal(err)
	}
	here, there := gitRepo(t), gitRepo(t)
	delegate := func(p proto.GOADelegateParams) proto.GOATask {
		t.Helper()
		var task proto.GOATask
		p.NoRoute, p.From, p.Agent, p.Branch = true, g.ID(), "claude", ""
		if err := ca.Call(proto.MethodGOADelegate, p, &task); err != nil {
			t.Fatal(err)
		}
		return task
	}
	recall := func(q string) string {
		t.Helper()
		var hits []proto.RAGHit
		if err := ca.Call(proto.MethodGOARecall, proto.GOARecallParams{Session: g.ID(), Query: q}, &hits); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, h := range hits {
			out = append(out, h.Snippet)
		}
		return strings.Join(out, "\n")
	}
	task := delegate(proto.GOADelegateParams{Project: here, Title: "Merge the totals", Prompt: "Merge the invoice totals branch"})
	if got := recall("merge invoice totals"); !strings.Contains(got, "[delegation]") || !strings.Contains(got, task.Session) || !strings.Contains(got, here) || !strings.Contains(got, "Merge the invoice totals branch") {
		t.Fatalf("the delegation: %q", got)
	}
	far := delegate(proto.GOADelegateParams{Host: b.daemonID, Project: there, Title: "Rotate the logs", Prompt: "Rotate the server logs weekly"})
	if got := recall("rotate logs"); !strings.Contains(got, b.daemonID) || !strings.Contains(got, far.Session) {
		t.Fatalf("on another host: %q", got)
	}

	// A turn ends: no outcome, and the board doesn't say done.
	worker, _ := a.sessions.Get(task.Session)
	if err := a.sessions.SetState(worker, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the turn ended", func() bool {
		for _, x := range a.goa.board.List() {
			if x.ID == task.ID {
				return x.Status == proto.GOATaskTurnEnded
			}
		}
		return false
	})
	if got := recall("merge invoice totals"); strings.Contains(got, "[outcome]") {
		t.Fatalf("a turn's end kept as its outcome: %q", got)
	}
	// Settled: the outcome, with what it names.
	report := "Merged in abc1234, https://github.com/shop/api/pull/42"
	if err := ca.Call(proto.MethodGOATask, proto.GOATaskParams{ID: task.ID, Status: proto.GOATaskDone, Result: report}, nil); err != nil {
		t.Fatal(err)
	}
	if err := ca.Call(proto.MethodGOATask, proto.GOATaskParams{ID: far.ID, Status: proto.GOATaskDone, Result: "Logs rotate on Mondays"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := recall("merge totals abc1234"); !strings.Contains(got, "[outcome]") || !strings.Contains(got, "ended done") || !strings.Contains(got, "pull/42") {
		t.Fatalf("the outcome: %q", got)
	}
	// Settled again says nothing new.
	before := len(a.ragSettings.Memories(rag.ProjectID(a.goaDir())))
	if err := ca.Call(proto.MethodGOATask, proto.GOATaskParams{ID: task.ID, Status: proto.GOATaskDone}, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(a.ragSettings.Memories(rag.ProjectID(a.goaDir()))); n != before {
		t.Fatalf("kept twice: %d, was %d", n, before)
	}
	// A secret stays out.
	const secret = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	leak := delegate(proto.GOADelegateParams{Project: here, Title: "Use the token", Prompt: "Use the token " + secret})
	if err := ca.Call(proto.MethodGOATask, proto.GOATaskParams{ID: leak.ID, Status: proto.GOATaskDone, Result: "Used " + secret}, nil); err != nil {
		t.Fatal(err)
	}
	mems := a.ragSettings.Memories(rag.ProjectID(a.goaDir()))
	for _, m := range mems {
		if strings.Contains(m.Text, secret) {
			t.Fatalf("a secret kept: %q", m.Text)
		}
	}
	// Only GOA's collection, and nothing was sent: RAG is off for it.
	if n := len(a.ragSettings.Memories(rag.ProjectID(here))); n != 0 {
		t.Fatalf("%d memories in a project's collection", n)
	}
	// Deleted by the commands that delete memories.
	var listed []proto.RAGMemory
	if err := ca.Call(proto.MethodRAGMemories, proto.RAGRef{Session: g.ID()}, &listed); err != nil || len(listed) != len(mems) {
		t.Fatalf("listed %d of %d: %v", len(listed), len(mems), err)
	}
	if err := ca.Call(proto.MethodRAGForget, proto.RAGForgetParams{RAGRef: proto.RAGRef{Session: g.ID()}, ID: listed[0].ID}, nil); err != nil {
		t.Fatal(err)
	}
	// After a restart, still there.
	again := rag.OpenSettings(dataDir).Memories(rag.ProjectID(a.goaDir()))
	if len(again) != len(mems)-1 {
		t.Fatalf("after a restart: %d, want %d", len(again), len(mems)-1)
	}
}

// The user's default of skipping prompts reaches sessions started without
// asking them only for an agent with no hooks, like OpenCode: its prompts
// would wait with nothing to tell of them. An agent with hooks, GOA's workers
// of one too, stays as it was asked to start.
func TestSkipUnheardAgents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	for _, name := range []string{"opencode", "claude"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv, _ := hookServer(t)
	c := connect(t, srv, nil)
	create := func(p proto.SessionCreateParams) proto.SessionInfo {
		t.Helper()
		p.Cols, p.Rows, p.Dir = 80, 24, t.TempDir()
		var info proto.SessionInfo
		if err := c.Call(proto.MethodSessionCreate, p, &info); err != nil {
			t.Fatal(err)
		}
		return info
	}
	if s := create(proto.SessionCreateParams{Agent: "opencode"}); s.SkipPermissions {
		t.Fatal("skipped with no default set")
	}
	if s := create(proto.SessionCreateParams{Agent: "opencode", SkipUnheard: true}); !s.SkipPermissions {
		t.Fatal("OpenCode's prompts would wait unseen")
	}
	if s := create(proto.SessionCreateParams{Agent: "claude", SkipUnheard: true}); s.SkipPermissions {
		t.Fatal("an agent with hooks was put in bypass though nobody asked")
	}

	worker := func() proto.SessionInfo {
		t.Helper()
		var task proto.GOATask
		if err := c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Agent: "opencode", Project: t.TempDir(), Branch: "", Prompt: "Do it", NoRoute: true}, &task); err != nil {
			t.Fatal(err)
		}
		s, err := srv.sessions.Get(task.Session)
		if err != nil {
			t.Fatal(err)
		}
		return s.Info()
	}
	if w := worker(); w.SkipPermissions {
		t.Fatal("a worker skipped with no GOA that keeps prompts skipped")
	}
	g := create(proto.SessionCreateParams{Agent: "claude", GOA: true, SkipUnheard: true})
	if g.SkipPermissions || !g.SkipUnheard {
		t.Fatalf("GOA on an agent with hooks: %+v", g)
	}
	if w := worker(); !w.SkipPermissions {
		t.Fatal("GOA's OpenCode worker would wait on a prompt it can't hear of")
	}
}

// A task for a project with a busy session is queued, not a new session, and
// reaches it at its turn's end, in order; a --branch still starts a new one.
func TestGOAQueuesForABusySession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nwhile read -r line; do echo \"heard: $line\"; done\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv, _ := hookServer(t)
	c := connect(t, srv, nil)
	project := t.TempDir()
	var info proto.SessionInfo
	if err := c.Call(proto.MethodSessionCreate, proto.SessionCreateParams{Agent: "claude", Project: project, BranchMode: proto.BranchNone, Cols: 80, Rows: 24}, &info); err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessions.Get(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.sessions.SetState(sess, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	delegate := func(prompt string) proto.GOATask {
		t.Helper()
		var task proto.GOATask
		if err := c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Agent: "claude", Project: project, Prompt: prompt, NoRoute: true}, &task); err != nil {
			t.Fatal(err)
		}
		return task
	}
	first, second := delegate("first thing"), delegate("second thing")
	if first.Session != info.ID || second.Session != info.ID || first.Status != proto.GOATaskQueued || second.Status != proto.GOATaskQueued {
		t.Fatalf("%+v %+v", first, second)
	}
	if strings.Contains(sess.Terminal().Text(), "first thing") {
		t.Fatal("delivered on top of its turn")
	}
	if err := srv.sessions.SetState(sess, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(sess.Terminal().Text(), "first thing") {
		if time.Now().After(deadline) {
			t.Fatalf("never delivered: %q", sess.Terminal().Text())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if strings.Contains(sess.Terminal().Text(), "second thing") {
		t.Fatal("the second went with the first")
	}
	var task proto.GOATask
	if err := c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Agent: "claude", Project: project, Branch: "other", Prompt: "apart", NoRoute: true}, &task); err == nil && task.Session == info.ID {
		t.Fatalf("a branch asked for took the project's session: %+v", task)
	}
}
