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
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// restartAgent is a stand-in claude: it says how it was started and what it hears.
func restartAgent(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"started: $*\"\necho '❯ '\nwhile read -r line; do echo \"heard: $line\"; done\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SHELL", "/bin/sh")
	return filepath.Join(bin, "claude")
}

func restartServers(t *testing.T) (string, *Server, *client.Client) {
	t.Helper()
	dir := t.TempDir()
	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, srv, connect(t, srv, nil)
}

func screenOf(t *testing.T, srv *Server, id string) string {
	t.Helper()
	sess, err := srv.sessions.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(sess.Terminal().Text()), "")
}

func waitInScreen(t *testing.T, srv *Server, id, what string) {
	t.Helper()
	what = strings.Join(strings.Fields(what), "")
	waitFor(t, what, func() bool { return strings.Contains(screenOf(t, srv, id), what) })
}

func agentSession(t *testing.T, c *client.Client) proto.SessionInfo {
	t.Helper()
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Cols: 120, Rows: 40})
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// The plan says what a restart would stop: agents at work, what runs in terminals;
// not an idle agent, an idle shell or one whose state went stale.
func TestRestartPlanSaysWhatWouldStop(t *testing.T) {
	restartAgent(t)
	_, srv, c := restartServers(t)
	t.Cleanup(srv.Close)
	idle, busy := agentSession(t, c), agentSession(t, c)
	waitInScreen(t, srv, busy.ID, "started:")
	sess, _ := srv.sessions.Get(busy.ID)
	if err := srv.sessions.SetState(sess, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	quiet, err := c.CreateSession(proto.SessionCreateParams{Agent: "shell", Dir: t.TempDir(), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	running, err := c.CreateSession(proto.SessionCreateParams{Agent: "shell", Dir: t.TempDir(), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // the shells at their prompts
	if err := c.Input(running.ID, "sleep 1234", false, true); err != nil {
		t.Fatal(err)
	}
	var plan proto.RestartPlan
	waitFor(t, "the command seen", func() bool {
		var err error
		plan, err = c.RestartPlan()
		return err == nil && len(plan.Commands) > 0
	})
	if len(plan.Agents) != 1 || plan.Agents[0].Session != busy.ID || plan.Agents[0].What != "claude" {
		t.Fatalf("agents: %+v (idle is %s)", plan.Agents, idle.ID)
	}
	if len(plan.Commands) != 1 || plan.Commands[0].Session != running.ID || !strings.Contains(plan.Commands[0].What, "sleep 1234") {
		t.Fatalf("commands: %+v (quiet is %s)", plan.Commands, quiet.ID)
	}
	if plan.Empty() {
		t.Fatal("empty")
	}
}

// An agent cut off mid-turn is told once to carry on, on its conversation, its prompt
// not sent again. One that was idle is told nothing.
func TestAgentMidTurnCarriesOnAfterARestart(t *testing.T) {
	restartAgent(t)
	dir, srv, c := restartServers(t)
	first, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: t.TempDir(), Prompt: "the first prompt", Cols: 120, Rows: 40})
	if err != nil {
		t.Fatal(err)
	}
	idle := agentSession(t, c)
	waitInScreen(t, srv, first.ID, "started:")
	sess, _ := srv.sessions.Get(first.ID)
	if err := srv.sessions.SetState(sess, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	srv.Close()

	again, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)
	waitInScreen(t, again, first.ID, "heard: [enowX] The daemon restarted")
	time.Sleep(300 * time.Millisecond)
	text := screenOf(t, again, first.ID)
	if n := strings.Count(text, "heard:[enowX]"); n != 1 {
		t.Fatalf("told %d times: %s", n, text)
	}
	if strings.Count(text, "heard:thefirstprompt") != 0 {
		t.Fatalf("its prompt went again: %s", text)
	}
	if strings.Contains(screenOf(t, again, idle.ID), "heard:[enowX]") {
		t.Fatal("an idle agent was told to carry on")
	}
	// A further restart has nothing more to say.
	again2 := filepath.Join(dir, "sessions.json")
	data, err := os.ReadFile(again2)
	if err != nil || strings.Contains(string(data), "midTurn") {
		t.Fatalf("the note stayed: %v", err)
	}
}

// An agent run by hand in a terminal comes back as itself, on its conversation and in
// its mode, not as a shell that would run what it is sent; its prompt never again. A
// command is said to have ended, and isn't run again.
func TestTerminalsAfterARestart(t *testing.T) {
	agentPath := restartAgent(t) // by its path: a login shell sets a PATH of its own
	dir, srv, c := restartServers(t)
	newShell := func() proto.SessionInfo {
		info, err := c.CreateSession(proto.SessionCreateParams{Agent: "shell", Dir: t.TempDir(), Cols: 160, Rows: 40})
		if err != nil {
			t.Fatal(err)
		}
		return info
	}
	tab, cmd := newShell(), newShell()
	time.Sleep(500 * time.Millisecond)
	if err := c.Input(tab.ID, agentPath+` --model opus --dangerously-skip-permissions "do the thing"`, false, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Input(cmd.ID, "sleep 4321", false, true); err != nil {
		t.Fatal(err)
	}
	waitInScreen(t, srv, tab.ID, "started: --model opus")
	waitFor(t, "both running", func() bool {
		plan, err := c.RestartPlan()
		return err == nil && len(plan.Commands) == 2
	})
	srv.Close()

	again, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)
	waitInScreen(t, again, tab.ID, "started: --continue --model opus --dangerously-skip-permissions")
	if err := connect(t, again, nil).Input(tab.ID, "are you there", false, true); err != nil {
		t.Fatal(err)
	}
	waitInScreen(t, again, tab.ID, "heard: are you there")
	if strings.Contains(strings.SplitN(screenOf(t, again, tab.ID), "enowXrestarted", 2)[1], "dothething") {
		t.Fatalf("its prompt went again: %s", screenOf(t, again, tab.ID))
	}
	waitInScreen(t, again, cmd.ID, "This was running here, and ended when the daemon restarted: sleep 4321")
	plan, err := connect(t, again, nil).RestartPlan()
	if err != nil || !slices.ContainsFunc(plan.Lost, func(it proto.RestartItem) bool {
		return it.Session == cmd.ID && strings.Contains(it.What, "sleep 4321")
	}) {
		t.Fatalf("not listed: %+v, %v", plan.Lost, err)
	}
	if slices.ContainsFunc(plan.Commands, func(it proto.RestartItem) bool { return strings.Contains(it.What, "sleep 4321") }) {
		t.Fatal("run again")
	}
}

// A secret in a command stays out of what is listed.
func TestCommandWordsMaskSecrets(t *testing.T) {
	got := commandWords([]string{"/usr/bin/curl", "-H", "Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345", "https://x"})
	if strings.Contains(got, "abcdefghijklmnop") || !strings.HasPrefix(got, "curl") {
		t.Fatal(got)
	}
}

// GOD of Agents' tasks stay as they were, and what waits for it stays, through a restart.
func TestGOAStateSurvivesARestart(t *testing.T) {
	restartAgent(t)
	dir, srv, c := restartServers(t)
	g, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	worker := agentSession(t, c)
	task, err := srv.goa.board.Add(proto.GOATask{Title: "Totals", Session: worker.ID, Status: proto.GOATaskWorking})
	if err != nil {
		t.Fatal(err)
	}
	srv.goa.mu.Lock()
	srv.goa.tell = append(srv.goa.tell, goaLine{Text: "[enowX GOA] waiting for it"})
	srv.goa.mu.Unlock()
	srv.saveGOATell()
	srv.Close()

	again, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)
	tasks := again.goa.board.List()
	i := slices.IndexFunc(tasks, func(x proto.GOATask) bool { return x.ID == task.ID })
	if i < 0 || tasks[i].Status != proto.GOATaskWorking || tasks[i].Session != worker.ID {
		t.Fatalf("the board: %+v", tasks)
	}
	if _, err := again.sessions.Get(g.ID); err != nil {
		t.Fatal(err)
	}
	var sess *session.Session
	if sess, err = again.sessions.Get(worker.ID); err != nil || sess.Info().Exited {
		t.Fatalf("the worker: %v", err)
	}
	waitFor(t, "the line waits or has gone in", func() bool {
		return slices.Contains(again.goaWaitingLines(), "[enowX GOA] waiting for it") || strings.Contains(screenOf(t, again, g.ID), "heard:[enowXGOA]waitingforit")
	})
}
