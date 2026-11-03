//go:build !windows

package ui

import (
	"net"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/ui/theme"
)

// Settings' Terminal section lists the host's shells, its login shell
// picked, and picking another makes it the host's terminals' shell.
func TestTerminalSectionPicksTheHostsShell(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	cl, err := client.New(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	defer cl.SetShell("") // the daemon's choice is the process's
	app := New(Options{})
	var later mainQueue
	app.onMain = later.post
	app.client = cl
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(720).Height(560).Padding(20).Gap(16).Background(theme.Background).Children(func() { app.terminalSection(c) })
	}, 720, 560)
	settle := func(done func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !done() {
			if time.Now().After(deadline) {
				t.Fatalf("never settled: %q", tt.Texts())
			}
			time.Sleep(10 * time.Millisecond)
			later.run()
			tt.Frame()
		}
	}
	tt.Frame()
	settle(func() bool { return len(app.shells.list.Shells) > 0 })
	if !tt.HasText("login shell") {
		t.Fatalf("no login shell in %q", tt.Texts())
	}
	shells := app.shells.list.Shells
	if len(shells) < 2 {
		t.Skip("this host has one shell")
	}
	other := shells[1]
	if err := tt.Click(other.Name); err != nil {
		t.Fatal(err)
	}
	settle(func() bool {
		l, _ := cl.Shells()
		return l.Default == other.Path && app.shells.list.Default == other.Path
	})
	shot(t, tt, "terminal-section.png")
}
