//go:build !windows

package ui

import (
	"net"
	"os"
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

// The accounts show in the order they're used in, which moves; what a
// limit does is set; an account at its limit asks before switching when
// set to.
func TestAccountsOrderAndAsk(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nsleep 30\n"), 0o755)
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
	cl.AddAccount("claude", "Work")
	cl.AddAccount("claude", "Spare")
	app := New(Options{})
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.agents, app.host.Name = cl, agent.List(), "this Mac"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(820).Height(900).Background(theme.Background).TextColor(theme.Text).Children(func() {
			app.agentsSection(c)
			app.accountAskDialog(c)
		})
	}, 820, 900)
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
	frames(func() bool { return app.accts.loaded && len(app.accts.info.Accounts) >= 2 }, "the accounts")
	st := &app.accts
	app.saveAccountPrefs(st, "claude", proto.AccountPrefs{Order: []string{"Work", "", "Spare"}, Fallback: proto.FallbackAsk, Default: "Work"})
	frames(func() bool { return srv != nil && len(st.info.Prefs["claude"].Order) == 3 }, "the order kept")
	tt.Frame()
	if !tt.HasText("Ask me first") || !tt.HasText("new sessions") {
		t.Fatalf("shown: %q", tt.Texts())
	}
	shot(t, tt, "accounts-order.png")
	if got := app.defaultAccountFor("claude", "/code/x"); got == "" {
		t.Fatal("the default for new sessions")
	}
	app.acctAsk = &proto.AccountFallback{Session: "s1", Agent: "claude", From: "Work", To: "Spare", ToID: "x", Reason: proto.AccountLimit, Ask: true,
		Line: "Claude usage limit reached. Your limit will reset at 3pm", Until: time.Now().Add(3 * time.Hour)}
	tt.Frame()
	tt.Frame()
	if !tt.HasText("Switch and go on") {
		t.Fatalf("ask: %q", tt.Texts())
	}
	shot(t, tt, "accounts-ask.png")
}
