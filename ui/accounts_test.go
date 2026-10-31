//go:build !windows

package ui

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/ui/theme"
)

// Settings → Agents adds a config that is another account and starts
// signing it in.
func TestAddAnAgentAccount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\necho \"home=$CLAUDE_CONFIG_DIR\"\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
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
	app := New(Options{})
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.agents = cl, agent.List()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(760).Height(900).Background(theme.Background).Children(func() { app.agentsSection(c) })
	}, 760, 900)
	frames := func(until func() bool, what string) {
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
	}
	tt.Frame()
	frames(func() bool { return app.accts.loaded }, "the accounts")
	if err := tt.Click("New config"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	f := app.accts.edit
	f.kind, f.name = slices.Index(f.kinds, kindAccount), "Work"
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return len(app.accts.list) == 1 && len(app.sessions) == 1 }, "the account and its sign-in")
	tt.Frame()
	shot(t, tt, "agent-accounts.png")
	if s := app.sessions[0]; s.Account != app.accts.list[0].ID || s.Agent != "claude" {
		t.Fatalf("sign-in session %+v", s)
	}
	if !tt.HasText("Work") || !tt.HasText("Official") {
		t.Fatalf("listed: %q", tt.Texts())
	}
	if got := app.accts.accountsOfAgent("claude-agent-teams"); len(got) != 1 {
		t.Fatalf("Agent Teams' accounts: %+v", got)
	}
}
