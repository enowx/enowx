//go:build !windows

package ui

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// agentsTester is Settings → Agents against a daemon of its own, with
// only the agents named installed (fake programs on the PATH).
func agentsTester(t *testing.T, installed ...string) (*App, *ui.Tester, *client.Client, func(func() bool, string)) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := t.TempDir()
	for _, name := range installed {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	app := New(Options{})
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.agents, app.host.Name = cl, agent.List(), "this Mac"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(900).Height(980).Background(theme.Background).TextColor(theme.Text).Children(func() { app.agentsSection(c) })
	}, 900, 980)
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
	}
	tt.Frame()
	return app, tt, cl, frames
}

// Settings → Agents makes a model API config of Claude Code in a dialog:
// it shows in the agent's page with its badge, and Claude Code's own
// settings aren't written.
func TestMakeAModelAPIConfig(t *testing.T) {
	app, tt, cl, frames := agentsTester(t, "claude")
	frames(func() bool { return app.accts.loaded && app.agentAPIs.loaded && tt.HasText("Official") }, "Claude Code's page")
	if err := tt.Click("New config"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	frames(func() bool { return tt.HasText("Base URL") }, "the dialog")
	f := app.accts.edit
	if f.agent != "claude" || f.kindName() != kindAPI {
		t.Fatalf("the dialog: %s, %s", f.agent, f.kindName())
	}
	f.name, f.api.url, f.keyCred = "GLM", "https://openrouter.ai/api", "openrouter"
	f.api.models[0].id, f.api.models[0].output = "z-ai/glm-5", "32000"
	if err := tt.Click("Add a model"); err != nil {
		t.Fatal(err)
	}
	f.api.models[1].id = "moonshot/kimi-k3"
	tt.Frame()
	shot(t, tt, "config-dialog.png")
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool {
		return app.accts.edit == nil && tt.HasText("https://openrouter.ai/api · z-ai/glm-5 +1 · key openrouter")
	}, "the config listed")
	shot(t, tt, "agents-configs.png")
	list, _ := cl.Accounts()
	if len(list) != 1 || list[0].Kind != proto.ConfigKindAPI || list[0].API.Models[0].Output != 32000 || list[0].Color == "" {
		t.Fatalf("saved: %+v", list)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")); err == nil {
		t.Fatal("Claude Code's own settings written")
	}

	// New session offers Official or the config, the preferences' default
	// first.
	app.newSession.agent = "claude"
	ns := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(600).Height(120).Background(theme.Background).TextColor(theme.Text).Children(func() { app.accountField(c) })
	}, 600, 120)
	ns.Frame()
	if !ns.HasText("Official") || !ns.HasText("GLM") || app.newSession.account != "" {
		t.Fatalf("New session's configs: %q, picked %q", ns.Texts(), app.newSession.account)
	}
	if err := ns.Click("GLM"); err != nil {
		t.Fatal(err)
	}
	ns.Frame()
	if app.newSession.account != list[0].ID || !app.newSession.accountPicked {
		t.Fatalf("picked %q", app.newSession.account)
	}

	// A session on it shows its badge in the list.
	row := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(300).Height(60).Background(theme.Panel).TextColor(theme.Text).Children(func() {
			app.sessionRow(c, 0, proto.SessionInfo{ID: "s1", Agent: "claude", Name: "Fix the parser", Account: list[0].ID,
				Config: &proto.SessionConfig{ID: list[0].ID, Name: "GLM", Kind: proto.ConfigKindAPI, Color: list[0].Color}})
		})
	}, 300, 60)
	row.Frame()
	if !row.HasText("GLM") {
		t.Fatalf("the session's badge: %q", row.Texts())
	}
	shot(t, row, "session-config-badge.png")

	// Duplicated.
	app.accts.edit = app.newConfigForm("claude", proto.Account{Agent: "claude", Name: "GLM copy", Kind: list[0].Kind, API: list[0].API, Color: list[0].Color})
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return tt.HasText("GLM copy") }, "the copy")
}

// An agent that takes a model API only in its own settings (Droid) still
// gets one there, from its page, backed up first.
func TestModelAPIInAnAgentsOwnSettings(t *testing.T) {
	app, tt, _, frames := agentsTester(t, "droid")
	frames(func() bool {
		return app.accts.loaded && app.agentAPIs.loaded && tt.HasText("A model API in its own settings")
	}, "Droid's page")
	if err := tt.Click("Use an API…"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	frames(func() bool { return tt.HasText("Base URL") && app.agentAPIs.edit != nil }, "the dialog")
	f := app.agentAPIs.edit
	f.url, f.key = "https://gw.example.com/v1", "fake-key-9876"
	f.models[0].id = "gw-model"
	tt.Frame()
	if err := tt.Click("Use it"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool {
		return app.agentAPIs.edit == nil && tt.HasText("https://gw.example.com/v1 · gw-model · key ••••9876")
	}, "Droid set")
	data, _ := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".factory", "settings.json"))
	if !strings.Contains(string(data), "gw-model") {
		t.Fatalf("Droid's settings:\n%s", data)
	}
}
