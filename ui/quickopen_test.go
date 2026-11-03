package ui

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// Go to file finds a session's file by a few letters, on its daemon, and
// opens it in a tab.
func TestQuickOpenFindsAndOpensAFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	for _, p := range []string{"ui/chatview/composer.go", "core/session/session.go", "README.md"} {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
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
	info, err := cl.CreateSession(proto.SessionCreateParams{Command: []string{"sh", "-c", "sleep 30"}, Dir: dir, Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	a := New(Options{})
	posted := make(chan func(), 16)
	a.onMain = func(fn func()) { posted <- fn }
	a.client, a.sessions, a.selected = cl, []proto.SessionInfo{info}, info.ID
	a.openQuickOpen()
	a.quick.query = "cmps"
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(700).Height(500).Background(theme.Background).Children(func() { a.quickOpenDialog(c) })
	}, 700, 500)
	deadline := time.Now().Add(10 * time.Second)
	for len(a.quick.paths) == 0 || a.quick.asked != "cmps" {
		select {
		case fn := <-posted:
			fn()
		case <-time.After(50 * time.Millisecond):
		}
		tt.Frame()
		if time.Now().After(deadline) {
			t.Fatalf("nothing found: %+v", a.quick)
		}
	}
	if a.quick.paths[0] != "ui/chatview/composer.go" || !tt.HasText("composer.go") {
		t.Fatalf("found %q: %q", a.quick.paths, tt.Texts())
	}
	if err := tt.Click("ui/chatview/composer.go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.quick.open || a.centerOf(a.selected).front != fileTabPrefix+"ui/chatview/composer.go" {
		t.Fatalf("open %v, tab %q", a.quick.open, a.centerOf(a.selected).front)
	}
}
