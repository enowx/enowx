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
)

// New session with another agent picked gives both the prompt, each on
// its own branch; Compare then keeps one and discards the other.
func TestFanOutFromNewSessionAndKeepOne(t *testing.T) {
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
	defer srv.Close()
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
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
	// Its switch, at the row's end of the 640 wide dialog in this 900 wide window.
	also, ok := tt.Find("Also give it to Codex")
	if !ok {
		t.Fatalf("no row: %q", tt.Texts())
	}
	tt.ClickAt(730, also.Y+also.H/2)
	tt.Frame()
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
