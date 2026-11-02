package fileview

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// From the tree's menus, through the daemon: a new folder, a rename, the
// properties, then a delete confirmed.
func TestChangeFilesFromTheMenus(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
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
	s, err := cl.CreateSession(proto.SessionCreateParams{Command: []string{"sh"}, Dir: root, Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	v := New()
	posted := make(chan func(), 64)
	o := Options{Client: cl, Session: s, Update: func(fn func()) { posted <- fn }, Wanted: func() bool { return true }}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(520).Height(600).Background(theme.Background).TextColor(theme.Text).Children(func() { v.Draw(c, o) })
	}, 520, 600)
	frames := func(until func() bool, what string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
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
		tt.Frame()
	}
	click := func(s string) {
		t.Helper()
		if err := tt.Click(s); err != nil {
			t.Fatalf("%v: %q", err, tt.Texts())
		}
		tt.Frame()
	}
	exists := func(p string) bool { _, err := os.Stat(filepath.Join(root, p)); return err == nil }
	frames(func() bool { return tt.HasText("README.md") }, "the tree")

	tt.RightClickAt(260, 500) // below the rows: the project folder
	tt.Frame()
	lookShot(t, tt, "files-space-menu")
	click("New folder in the project folder…")
	lookShot(t, tt, "files-name")
	v.ask.name = "docs"
	tt.Frame()
	click("Create")
	frames(func() bool { return exists("docs") && v.ask == nil }, "docs made")

	frames(func() bool { return tt.HasText("README.md") }, "README listed")
	if err := tt.RightClick("README.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	lookShot(t, tt, "files-row-menu")
	click("Rename…")
	v.ask.name = "GUIDE.md"
	tt.Frame()
	click("Rename")
	frames(func() bool { return exists("GUIDE.md") && !exists("README.md") }, "the rename")

	frames(func() bool { return tt.HasText("GUIDE.md") }, "GUIDE listed")
	if err := tt.RightClick("GUIDE.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	click("Properties")
	frames(func() bool { return v.ask != nil && v.ask.stat != nil }, "the properties")
	lookShot(t, tt, "files-properties")
	if !tt.HasText("Permissions") || !tt.HasText("-rw-r--r--") || !tt.HasText("2 B") {
		t.Fatalf("properties: %q", tt.Texts())
	}
	click("Close")

	if err := tt.RightClick("GUIDE.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	click("Delete…")
	if !strings.Contains(strings.Join(tt.Texts(), " "), "Delete GUIDE.md?") {
		t.Fatalf("no confirmation: %q", tt.Texts())
	}
	lookShot(t, tt, "files-delete")
	click("Delete")
	frames(func() bool { return !exists("GUIDE.md") }, "the delete")
}
