//go:build !windows

package ui

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/wrapcheck"
)

// fanOutForm is a New session form on a new branch, in a dialog in a 900 wide
// window, over a real daemon. Its frames waits for what it says, drawing.
func fanOutForm(t *testing.T) (*App, *client.Client, *ui.Tester, func(func() bool, string)) {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho \"asked: $*\"\nsleep 30\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	if _, err := cl.AddProject(repo); err != nil {
		t.Fatal(err)
	}
	detail, err := cl.InspectProject(repo)
	if err != nil {
		t.Fatal(err)
	}
	app := New(Options{})
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.agents = cl, agent.List()
	app.newSession = newSessionForm{open: true, project: detail, loaded: true, agent: "claude",
		branchMode: proto.BranchNew, branch: "try", base: detail.Branch, prompt: "fix the login"}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(900).Height(1000).Background(theme.Background).Children(func() {
			app.newSessionDialog(c)
			app.compareDialogView(c)
		})
	}, 900, 1000)
	frames := func(until func() bool, what string) {
		deadline := time.Now().Add(15 * time.Second)
		for !until() {
			select {
			case fn := <-posted:
				fn()
			case <-time.After(30 * time.Millisecond):
			}
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("never: %s; %q", what, tt.Texts())
			}
		}
	}
	tt.Frame()
	return app, cl, tt, frames
}

// clickFanOut clicks the Fan out row's switch, at the end of the 640 wide dialog.
func clickFanOut(t *testing.T, tt *ui.Tester) {
	t.Helper()
	row, ok := tt.Find("Fan out")
	if !ok {
		t.Fatalf("no Fan out row: %q", tt.Texts())
	}
	tt.ClickAt(730, row.Y+row.H/2)
	tt.Frame()
}

// clickCheck clicks an agent's checkbox.
func clickCheck(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	if err := tt.Click(name); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
}

// The switch starts off with no list; on, it lists the other agents, not the
// form's own; off and on again, the picks come back; and off, they are ignored.
func TestFanOutSwitchShowsAndKeepsThePicks(t *testing.T) {
	app, _, tt, _ := fanOutForm(t)
	if app.newSession.fanOut || tt.HasText("Codex") {
		t.Fatalf("not off to begin with: %q", tt.Texts())
	}
	if !tt.HasText("Give it to more agents, then compare the results") {
		t.Fatalf("no description: %q", tt.Texts())
	}
	clickFanOut(t, tt)
	shot(t, tt, "fan-out-on.png")
	if !app.newSession.fanOut || !tt.HasText("Codex") {
		t.Fatalf("no list: %q", tt.Texts())
	}
	if tt.HasText("Claude Code") && len(app.newSession.fanOutAgents()) > 0 {
		t.Fatalf("picked by itself: %v", app.newSession.also)
	}
	clickCheck(t, tt, "Codex")
	if got := app.newSession.fanOutAgents(); len(got) != 1 || got[0] != "codex" {
		t.Fatalf("picked %v", got)
	}
	clickFanOut(t, tt)
	if tt.HasText("Codex") || len(app.newSession.fanOutAgents()) != 0 {
		t.Fatalf("off: %q, agents %v", tt.Texts(), app.newSession.fanOutAgents())
	}
	clickFanOut(t, tt)
	if got := app.newSession.fanOutAgents(); len(got) != 1 || got[0] != "codex" || !tt.HasText("Codex") {
		t.Fatalf("back on: %v", got)
	}
	clickCheck(t, tt, "Codex")
	if len(app.newSession.fanOutAgents()) != 0 {
		t.Fatalf("unpicked: %v", app.newSession.also)
	}
}

// Checked agents with the switch off start the one session, no comparison.
func TestFanOutOffIgnoresThePicks(t *testing.T) {
	app, cl, tt, frames := fanOutForm(t)
	clickFanOut(t, tt)
	clickCheck(t, tt, "Codex")
	clickFanOut(t, tt)
	if err := tt.Click("Start session"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return !app.newSession.open }, "the session")
	list, err := cl.Sessions()
	if err != nil || len(list) != 1 || list[0].Agent != "claude" || app.compare.open {
		t.Fatalf("started %+v, %v; comparing %v", list, err, app.compare.open)
	}
}

// A name too long for its column runs inside it: nothing sticks out.
func TestFanOutLongNameStaysInItsColumn(t *testing.T) {
	app, _, tt, _ := fanOutForm(t)
	app.agents = append(app.agents, proto.AgentInfo{ID: "long", Name: "An agent with a very long name indeed, far too long for half a dialog of this width", Command: "long", Installed: true})
	tt.Frame()
	clickFanOut(t, tt)
	tt.Frame()
	if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
		t.Fatalf("%q", out)
	}
	r, ok := tt.Find(app.agents[len(app.agents)-1].Name)
	if !ok || r.W > 320 {
		t.Fatalf("its box %+v, %v", r, ok)
	}
}

// New session with another agent picked gives both the prompt, each on
// its own branch; Compare then keeps one and discards the other.
func TestFanOutFromNewSessionAndKeepOne(t *testing.T) {
	app, cl, tt, frames := fanOutForm(t)
	clickFanOut(t, tt)
	clickCheck(t, tt, "Codex")
	shot(t, tt, "fan-out-form.png")
	if err := tt.Click("Start session"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return app.compare.open && len(app.groupSessions(app.compare.group)) == 2 }, "the comparison")
	frames(func() bool { return len(app.compare.diffs) == 2 }, "what each changed")
	tt.Frame()
	shot(t, tt, "fan-out-compare.png")
	group := app.groupSessions(app.compare.group)
	if group[0].Branch != "try-claude" || group[1].Branch != "try-codex" {
		t.Fatalf("branches %s, %s", group[0].Branch, group[1].Branch)
	}
	app.keepFromGroup(group[0], group)
	frames(func() bool { return !app.compare.open }, "keeping one")
	list, err := cl.Sessions()
	if err != nil || len(list) != 1 || list[0].ID != group[0].ID {
		t.Fatalf("left: %+v, %v", list, err)
	}
}
