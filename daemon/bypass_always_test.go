package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// agentStarts is what the fake Claude Code in home has logged of its starts: their arguments
// hold lines of their own, so a start is told by the log growing.
type agentStarts struct {
	file string
	seen int
}

func startsOf(home string) *agentStarts { return &agentStarts{file: filepath.Join(home, "args")} }

// next waits for a start since the last and is its arguments.
func (a *agentStarts) next(t *testing.T) string {
	t.Helper()
	line := ""
	waitFor(t, "a start of the agent", func() bool {
		data, _ := os.ReadFile(a.file)
		if len(data) <= a.seen {
			return false
		}
		line, a.seen = string(data[a.seen:]), len(data)
		return true
	})
	return line
}

func wantBypassLine(t *testing.T, what, line string) {
	t.Helper()
	if !strings.Contains(line, "--dangerously-skip-permissions") || strings.Contains(line, "--permission-mode") || strings.Contains(line, "--allow-dangerously") {
		t.Errorf("%s started as: %s", what, clip(line, 400))
	}
}

// Every way a Claude Code session starts or comes back, asked for nothing and with a mode
// saved that isn't bypass: the case that brought a woken session back in auto mode.
func TestClaudeCodeAlwaysComesUpInBypass(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, c, home, dir := forkHarness(t)
	starts := startsOf(home)
	last := ""
	check := func(what, id string) {
		t.Helper()
		last = starts.next(t)
		wantBypassLine(t, what, last)
		sess, err := srv.sessions.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if info := sess.Info(); !info.SkipPermissions {
			t.Errorf("%s: says it asks", what)
		}
		if m := sess.Meta().Mode; m != agent.ModeBypass {
			t.Errorf("%s: its mode is %q", what, m)
		}
	}
	// What spawn sends too, without --yolo; the New session dialog with the switch off.
	s, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Project: dir, Name: "new", Mode: "plan", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	check("a new session", s.ID)
	waitScreen(t, c, s.ID, "ready")
	waitFor(t, "its conversation", func() bool { return srv.transcriptPath(s.ID) != "" })
	sess, _ := srv.sessions.Get(s.ID)
	convo := srv.sessionConversation(sess)
	if convo == "" {
		t.Fatal("no conversation")
	}

	asleep := func() {
		t.Helper()
		sess, err := srv.sessions.Get(s.ID)
		if err != nil {
			t.Fatal(err)
		}
		// As a session a chat switched, or an earlier daemon kept.
		srv.sessions.SetMode(sess, "auto")
		if err := srv.hibernateSession(sess, true); err != nil {
			t.Fatal(err)
		}
	}

	asleep()
	if err := c.Call(proto.MethodSessionWake, proto.SessionRef{ID: s.ID}, nil); err != nil {
		t.Fatal(err)
	}
	check("wake", s.ID)
	waitFor(t, "awake", func() bool { i, _ := srv.sessions.Get(s.ID); return !i.Info().Exited })

	asleep()
	if _, err := c.ContinueSession(s.ID); err != nil {
		t.Fatal(err)
	}
	check("continue", s.ID)

	asleep()
	if err := c.Input(s.ID, "hello", false, true); err != nil {
		t.Fatal(err)
	}
	check("send to one asleep", s.ID)

	// What d1d2fb8908e9 went through: hibernated, then woken by goa delegate.
	asleep()
	var task proto.GOATask
	if err := c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Session: s.ID, Prompt: "Do it", NoRoute: true}, &task); err != nil {
		t.Fatal(err)
	}
	check("goa delegate on one asleep", s.ID)

	if err := c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Agent: "claude", Project: dir, Branch: "delegated", Prompt: "Do it", NoRoute: true}, &task); err != nil {
		t.Fatal(err)
	}
	check("goa delegate on a new one", task.Session)

	resumed, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Project: dir, Name: "resumed", Resume: convo, Mode: "auto", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	check("resume from history", resumed.ID)

	tab, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Parent: s.ID, Mode: "default", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	check("an agent in a tab", tab.ID)

	goaInfo, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	check("GOD of Agents", goaInfo.ID)
	if !strings.Contains(last, "--tools "+agent.GOAClaudeTools) {
		t.Errorf("GOA kept none of its limits: %s", clip(last, 400))
	}
}

// A fork, of a session in another mode, starts in bypass, and so does the session it was
// brought back from when hibernated.
func TestForkOfClaudeCodeIsInBypass(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, c, home, dir := forkHarness(t)
	starts := startsOf(home)
	s, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Project: dir, Name: "Long work", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	wantBypassLine(t, "a new session", starts.next(t))
	waitScreen(t, c, s.ID, "ready")
	waitFor(t, "its conversation", func() bool { return srv.transcriptPath(s.ID) != "" })
	sess, _ := srv.sessions.Get(s.ID)
	srv.sessions.SetMode(sess, "auto")
	fork, err := c.ForkSession(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantBypassLine(t, "a fork", starts.next(t))
	if !fork.SkipPermissions {
		t.Errorf("the fork says it asks: %+v", fork)
	}
}

// The mode new sessions take, as Settings keeps it, doesn't turn bypass off.
func TestAgentStartModeDoesNotTurnBypassOff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, c, home, dir := forkHarness(t)
	srv.sched.mu.Lock()
	srv.sched.settings.AgentStart = map[string]proto.AgentStart{"claude": {Mode: "plan"}}
	srv.sched.mu.Unlock()
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Project: dir, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	wantBypassLine(t, "with a mode for new sessions", startsOf(home).next(t))
}

// An agent taking over from Claude Code doesn't inherit what only Claude Code always does.
func TestAgentTakingOverDoesNotInheritBypass(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, c, _, dir := forkHarness(t)
	bin := filepath.SplitList(os.Getenv("PATH"))[0]
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$HOME/codexargs\"\necho '❯ '\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Project: dir, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	waitScreen(t, c, s.ID, "ready")
	next, err := srv.switchAgent(proto.AccountSwitchParams{Session: s.ID, Agent: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if next.SkipPermissions {
		t.Fatalf("Codex took bypass from Claude Code: %+v", next)
	}
}

// A record of an earlier daemon: asked, in auto mode, its old command saved. A restart or
// an update brings it back in bypass, and an agent run by hand in a terminal too.
func TestRestartBringsBackBypassWhateverWasSaved(t *testing.T) {
	agentPath := restartAgent(t)
	// The stand-in says how it started.
	dir, srv, c := restartServers(t)
	s := agentSession(t, c)
	tab, err := c.CreateSession(proto.SessionCreateParams{Agent: "shell", Dir: t.TempDir(), Cols: 160, Rows: 40})
	if err != nil {
		t.Fatal(err)
	}
	waitInScreen(t, srv, s.ID, "started:")
	srv.Close()

	file := filepath.Join(dir, "sessions.json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}
	for _, r := range list {
		switch r["id"] {
		case s.ID:
			r["skipPermissions"], r["mode"] = false, "auto"
		case tab.ID:
			// As an earlier daemon noted it, stopping.
			r["tabAgent"] = []string{agentPath, "--continue", "--permission-mode", "auto", "--allow-dangerously-skip-permissions"}
		}
	}
	data, err = json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}

	again, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)
	back, err := again.sessions.Get(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !back.Info().SkipPermissions || back.Meta().Mode != agent.ModeBypass {
		t.Errorf("came back saying %v, %q", back.Info().SkipPermissions, back.Meta().Mode)
	}
	if got := back.Meta().Restart; !strings.Contains(strings.Join(got, " "), "--dangerously-skip-permissions") {
		t.Errorf("would restart as %s", clip(strings.Join(got, " "), 300))
	}
	waitInScreen(t, again, s.ID, "--dangerously-skip-permissions")
	waitInScreen(t, again, tab.ID, "started: --continue --dangerously-skip-permissions")
	if screen := screenOf(t, again, tab.ID); strings.Contains(screen, "--permission-mode") {
		t.Errorf("the tab's saved mode came back: %s", screen)
	}
}

// An agent run by hand in a shell tab, in a mode of its own, comes back in bypass.
func TestRestartBringsHandRunAgentBackInBypass(t *testing.T) {
	agentPath := restartAgent(t)
	dir, srv, c := restartServers(t)
	tab, err := c.CreateSession(proto.SessionCreateParams{Agent: "shell", Dir: t.TempDir(), Cols: 160, Rows: 40})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "its shell", func() bool { return strings.TrimSpace(screenOf(t, srv, tab.ID)) != "" })
	if err := c.Input(tab.ID, agentPath+" --permission-mode auto", false, true); err != nil {
		t.Fatal(err)
	}
	waitInScreen(t, srv, tab.ID, "started: --permission-mode auto")
	waitFor(t, "seen running", func() bool {
		plan, err := c.RestartPlan()
		return err == nil && len(plan.Commands) == 1
	})
	srv.Close()
	again, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)
	waitInScreen(t, again, tab.ID, "started: --continue --dangerously-skip-permissions")
}

// As root Claude Code refuses bypass: a session still starts, asking, and says so.
func TestClaudeAsRootStartsAskingAndSaysSo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	old := agent.AsRoot
	agent.AsRoot = func() bool { return true }
	t.Cleanup(func() { agent.AsRoot = old })
	srv, c, home, dir := forkHarness(t)
	s, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Project: dir, SkipPermissions: true, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	line := startsOf(home).next(t)
	if strings.Contains(line, "--dangerously-skip-permissions") || strings.Contains(line, "--allow-dangerously") {
		t.Errorf("would be refused: %s", clip(line, 300))
	}
	sess, _ := srv.sessions.Get(s.ID)
	if s.SkipPermissions || !strings.Contains(sess.Meta().Note, "refuses bypass as root") {
		t.Errorf("says %v, note %q", s.SkipPermissions, sess.Meta().Note)
	}
	if !slices.Contains([]string{"", "plan"}, sess.Meta().Mode) {
		t.Errorf("mode %q", sess.Meta().Mode)
	}
}

// keepBypass only ever speaks for an agent that always skips.
func TestKeepBypassLeavesOthersAlone(t *testing.T) {
	m := session.Meta{Agent: "codex", Mode: "plan"}
	keepBypass(&m)
	if m.SkipPermissions || m.Mode != "plan" {
		t.Fatalf("%+v", m)
	}
}
