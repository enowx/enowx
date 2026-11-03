package ui

import (
	"net"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// Clone from Git: an address names the folder, Clone clones it on the
// host, and the new project opens.
func TestCloneDialogClonesAndOpensTheProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	src := filepath.Join(t.TempDir(), "notes-app")
	for _, args := range [][]string{{"init", "-q", "-b", "main", src}, {"-C", src, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "first"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	app := New(Options{})
	var later mainQueue
	app.onMain = later.post
	cl, err := client.New(b, func(ev proto.Event) {
		if ev.Name == proto.EventProjectClone {
			if apply := app.cloneEvent(ev.Data); apply != nil {
				app.onMain(apply)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	app.client, app.host.Name = cl, "this Mac"
	dest := t.TempDir()
	app.openClone()
	app.clone.dir = dest
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(900).Height(900).Background(theme.Background).Children(func() { app.cloneDialog(c) })
	}, 900, 900)
	settle := func(what string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for {
			tt.Frame()
			if done() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q", what, tt.Texts())
			}
			time.Sleep(10 * time.Millisecond)
			later.run()
		}
	}
	tt.Frame()
	tt.Type("file://" + src)
	settle("the name from the address", func() bool { return app.clone.name == "notes-app" && tt.HasText("Clones into "+dest+"/notes-app") })
	shot(t, tt, "clone-dialog.png")
	if err := tt.Click("Clone"); err != nil {
		t.Fatal(err)
	}
	settle("the project open", func() bool { return app.clone == nil && app.project == filepath.Join(dest, "notes-app") })
	if app.state.CloneDirs[app.host.ID] != dest {
		t.Fatalf("remembered %q", app.state.CloneDirs)
	}
}
