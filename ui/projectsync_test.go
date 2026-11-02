package ui

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/projectsync"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// Sync with another host shows what goes each way and what changed on
// both, and brings both copies together, the side picked winning.
func TestSyncWithAnotherHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ENOWX_HOME", t.TempDir())
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repo := filepath.Join(t.TempDir(), "app")
	_ = os.MkdirAll(repo, 0o755)
	write(filepath.Join(repo, "main.go"), "package main")
	write(filepath.Join(repo, "notes.md"), "notes")
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	serve := func() *client.Client {
		srv, err := daemon.NewServer(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(srv.Close)
		a, b := net.Pipe()
		go srv.ServeConn(a)
		c, err := client.New(b, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	here, there := serve(), serve()
	proj, err := here.AddProject(repo)
	if err != nil {
		t.Fatal(err)
	}
	info, err := here.PackTransfer(proto.TransferPackParams{Project: repo, Sessions: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CopyTransfer(here, there, info.ID, info.Size, nil); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "app")
	if _, err := there.UnpackTransfer(info.ID, dest); err != nil {
		t.Fatal(err)
	}
	// Worked on in both places, notes.md on both.
	write(filepath.Join(dest, "server.txt"), "made there")
	write(filepath.Join(repo, "notes.md"), "notes, here")
	write(filepath.Join(dest, "notes.md"), "notes, there")

	app := New(Options{})
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.host = here, LocalHost()
	target := hosts.Host{ID: "vps", Name: "enowxlabs"}
	f := &projectSyncForm{open: true, project: proj, target: target, folder: dest, other: there}
	app.projectSyncing = f
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(600).Height(640).Padding(20).Gap(14).Background(theme.Dialog).TextColor(theme.Text).Children(func() {
			app.projectSyncBody(c, f)
			ui.Row(c.Key("foot")).FillWidth().Children(func() {
				app.projectSyncFooter(c, f, (f.step == syncComparing || f.step == syncRunning) && f.err == "")
			})
		})
	}, 600, 640)
	drain := func(until func() bool, what string) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !until() {
			select {
			case fn := <-posted:
				fn()
			case <-time.After(50 * time.Millisecond):
			}
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("never: %s (err %q, texts %q)", what, f.err, tt.Texts())
			}
		}
	}
	tt.Frame()
	if err := tt.Click("Compare"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return f.step == syncReview }, "the comparison")
	for _, want := range []string{"Changed on both: keep whose?", "notes.md", "server.txt"} {
		if !tt.HasText(want) {
			t.Fatalf("review lacks %q: %q", want, tt.Texts())
		}
	}
	shot(t, tt, "projectsync-review.png")
	// Keep this host's notes, though the server's are newer.
	for i := range f.plan.Files {
		if f.plan.Files[i].Conflict {
			f.plan.Files[i].From = projectsync.Here
		}
	}
	if err := tt.Click("Sync"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return f.step == syncDone }, "the sync")
	shot(t, tt, "projectsync-done.png")
	for path, want := range map[string]string{
		filepath.Join(repo, "server.txt"): "made there",
		filepath.Join(dest, "notes.md"):   "notes, here",
	} {
		if got, _ := os.ReadFile(path); string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	// The base is kept: comparing again finds nothing.
	partners := app.syncStore().Partners(projectsync.End{Host: app.host.ID, Path: repo})
	if len(partners) != 1 || partners[0].Host != "vps" || partners[0].Path != dest {
		t.Fatalf("partners %+v", partners)
	}
	f.step = ""
	tt.Frame()
	if err := tt.Click("Compare"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return f.step == syncReview }, "the second comparison")
	if !f.plan.Empty() || !tt.HasText("Both copies are the same.") {
		t.Errorf("second comparison: %+v", f.plan)
	}
}

// Connecting to the other host can be given up: the dialog closes while
// it still connects.
func TestProjectSyncCancelsWhileConnecting(t *testing.T) {
	app := New(Options{})
	f := &projectSyncForm{open: true, project: proto.ProjectInfo{Name: "app", Path: "/app"}, target: hosts.Host{ID: "vps", Name: "enowxlabs"},
		folder: "~/app", step: syncComparing, doing: "Connecting to enowxlabs…"}
	app.projectSyncing = f
	tt := ui.NewTester(func(c *ui.Context) { app.projectSyncDialog(c) }, 700, 600)
	tt.Frame()
	if err := tt.Click("Cancel"); err != nil {
		t.Fatalf("no Cancel while connecting: %v (texts %q)", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	if app.projectSyncing != nil {
		t.Fatal("the dialog stayed open")
	}
}
