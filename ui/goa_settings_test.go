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
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// GOA's Settings, through a real daemon: only the picked agent's configs are offered,
// another config restarts it unasked on the same session, another agent asks first, and
// what the host refuses stays in the panel with GOA as it was.
func TestGOASettingsChangeWhatItRunsOn(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nwhile read -r line; do echo \"heard: $line\"; done\n"), 0o755)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	app := New(Options{})
	posted := make(chan func(), 256)
	app.onMain = func(fn func()) { posted <- fn }
	cl, err := client.New(b1, func(ev proto.Event) {
		app.goaEvent(ev)
		if ev.Name == proto.EventSessionUpdated {
			app.handleEvent(app.gen, ev)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	work, err := cl.AddAccount("claude", "Work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.SaveConfig(proto.Account{Agent: "codex", Name: "Gateway", Kind: proto.ConfigKindAPI, API: &proto.ConfigAPI{BaseURL: "https://example.com/v1", Models: []proto.AgentAPIModel{{ID: "m"}}}}); err != nil {
		t.Fatal(err)
	}
	var g proto.SessionInfo
	if err := cl.Call(proto.MethodSessionCreate, proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 100, Rows: 30}, &g); err != nil {
		t.Fatal(err)
	}
	app.client, app.host.ID, app.host.Name = cl, "local", "this Mac"
	app.sessions, app.agents = []proto.SessionInfo{g}, ovAgents()
	app.state.GOA, app.goa.host, app.goa.board = true, "local", proto.GOABoard{Session: g.ID, Agent: "claude", Level: proto.GOAEnforced}
	tt := ui.NewTester(func(c *ui.Context) { app.view(c) }, 1400, 850)
	frames := func(what string, until func() bool) {
		t.Helper()
		deadline := time.Now().Add(45 * time.Second)
		for !until() {
			for drained := false; !drained; {
				select {
				case fn := <-posted:
					fn()
				case <-time.After(20 * time.Millisecond):
					drained = true
				}
			}
			tt.Advance(50 * time.Millisecond)
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("never: %s: %q", what, tt.Texts())
			}
		}
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q: %v (%q)", label, err, tt.Texts())
		}
		tt.Advance(time.Second)
		tt.Frame()
	}
	frames("GOA's header", func() bool { return tt.HasText("Rules") })
	for range 10 { // what the host says first moves the header
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
	}
	click("Settings of GOD of Agents")
	// While it opens too, what it holds stays inside its frame.
	const dialogRight, dialogLeft = (1400 + 520) / 2, (1400 - 520) / 2
	frames("the panel, with its agent's configs", func() bool { return tt.HasText("Running now") && tt.HasText("Codex") && tt.HasText("Work") })
	for range 8 {
		tt.Advance(40 * time.Millisecond)
		tt.Frame()
		for _, label := range []string{"Official", "Work", "Codex", "Restart GOA", "Cancel"} {
			if r, ok := tt.Find(label); ok && (r.X+r.W > dialogRight || r.X < dialogLeft) {
				t.Fatalf("%s past the panel: %+v", label, r)
			}
		}
	}
	if tt.HasText("Gateway") {
		t.Fatal("another agent's config is offered")
	}
	shot(t, tt, "goa-settings.png")

	click("Work")
	frames("what it does", func() bool { return tt.HasText("It restarts. It has no conversation yet to go on with.") })
	click("Restart GOA")
	frames("restarted on the config", func() bool { s, ok := app.goaSession(); return ok && s.Account == work.ID && app.goa.settings == nil })
	if s, _ := app.goaSession(); s.ID != g.ID || s.Agent != "claude" {
		t.Fatalf("another session: %+v", s)
	}

	click("Settings of GOD of Agents")
	frames("the panel again", func() bool { return tt.HasText("Running now") && tt.HasText("Codex") })
	click("Codex")
	frames("the other agent's configs", func() bool { return tt.HasText("Gateway") && !slices.Contains(tt.Texts(), "Work") })
	shot(t, tt, "goa-settings-agent.png")
	frames("what it does", func() bool {
		return tt.HasText("It restarts on a new conversation: the one it is on doesn't carry over to another agent.")
	})
	click("Restart GOA")
	frames("asked first", func() bool { return tt.HasText("Start a new conversation on Codex?") })
	if s, _ := app.goaSession(); s.Agent != "claude" {
		t.Fatal("changed before the yes")
	}
	for range 8 { // the question comes in, and takes clicks once there
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
	}
	shot(t, tt, "goa-settings-confirm.png")
	click("Restart it")
	frames("restarted on the agent", func() bool { s, ok := app.goaSession(); return ok && s.Agent == "codex" && app.goa.settings == nil })
	if s, _ := app.goaSession(); s.ID != g.ID || !s.GOA || s.Account != "" {
		t.Fatalf("after: %+v", s)
	}
}

// A host that doesn't know the change says so in the panel, and the button stays off.
func TestGOASettingsOnAnOlderDaemon(t *testing.T) {
	if got := goaSwitchErrorText(errOldDaemon{}); got != "This host's enowX is too old to change this: update it. GOD of Agents goes on as it was." {
		t.Fatalf("got %q", got)
	}
}

type errOldDaemon struct{}

func (errOldDaemon) Error() string { return `unknown method "goa.switch"` }
