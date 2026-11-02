package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// Each section's More menu opens on the row it belongs to, with what the row can
// do, and stays inside the window. A section alone, so the first More is the
// row's and not the top bar's.
func TestSettingsMoreMenus(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	app := h.app
	for _, tc := range []struct {
		name    string
		section func(*ui.Context)
		open    func()
		want    []string
	}{
		{"hook", app.hooksSection, func() { auditHooks(app) }, []string{"Edit…", "Remove…"}},
		{"automation", app.automationsSection, func() { auditAutomations(app) }, []string{"Edit…", "Remove…"}},
		{"config", app.agentsSection, func() {
			auditAgents(app)
			prefs := app.accts.info.Prefs["claude"]
			prefs.Order = []string{"Work account", ""}
			app.accts.info.Prefs["claude"] = prefs
		}, []string{"Change…", "Duplicate…", "Remove…"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.open()
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme.Window())
				ui.Column(c).Width(1000).Height(800).Background(theme.Background).TextColor(theme.Text).Children(func() { tc.section(c) })
			}, 1000, 800)
			h.tt = tt
			for range 4 {
				h.settle("", func() bool { return true })
			}
			if err := tt.Click("More"); err != nil {
				t.Fatalf("%v: %q", err, tt.Texts())
			}
			h.settle("its menu", func() bool { return tt.HasText(tc.want[0]) })
			shot(t, tt, "settings-menu-"+tc.name+".png")
			for _, item := range tc.want {
				if !tt.HasText(item) {
					t.Errorf("%q isn't in the menu: %q", item, tt.Texts())
				}
			}
			if out := outOfParents(tt); len(out) > 0 {
				t.Errorf("%s", strings.Join(out, "\n"))
			}
		})
	}
}

// The agent's API menu offers going back and the backups, and a session's menu
// switches among the agent's configs.
func TestSettingsConfigMenus(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	app := h.app
	auditAgents(app)
	draw := func(fn func(c *ui.Context)) *ui.Tester {
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(theme.Window())
			ui.Column(c).Width(800).Height(600).Background(theme.Background).TextColor(theme.Text).Children(func() { fn(c) })
		}, 800, 600)
		h.tt = tt
		for range 3 {
			h.settle("", func() bool { return true })
		}
		return tt
	}

	apis := app.agentAPIsOf()
	apis.loaded = true
	info := proto.AgentAPIInfo{Agent: "droid", Name: "Droid", Installed: true, Protocols: []string{"openai"},
		Current: &proto.AgentAPI{BaseURL: "https://gw.example.com/v1", Model: "gw-model"}, Backups: []proto.AgentAPIBackup{{Time: time.Now()}}}
	tt := draw(func(c *ui.Context) { app.agentAPIRow(c, apis, info) })
	if err := tt.Click("More"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	h.settle("the API's menu", func() bool { return tt.HasText("Back to its own API") })
	shot(t, tt, "settings-menu-api.png")

	// New session offers the agent's configs, Official among them.
	app.page = ""
	app.openNewSession(app.projects[0])
	app.newSession.agent = "claude"
	tt = draw(app.view)
	h.settle("the Config field", func() bool { return tt.HasText("Default") })
	if err := tt.Click("Config of the session"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	h.settle("the Config menu", func() bool { return tt.HasText("Work account") })
	shot(t, tt, "settings-new-session-config.png")
	app.newSession.open = false

	s := proto.SessionInfo{ID: "s1", Agent: "claude", Account: "work"}
	tt = draw(func(c *ui.Context) { app.switchAccountItems(c, s, func() {}) })
	if !tt.HasText("Switch to the official config") {
		t.Fatalf("the session's items: %q", tt.Texts())
	}
	s.Account = ""
	tt = draw(func(c *ui.Context) { app.switchAccountItems(c, s, func() {}) })
	if tt.HasText("Switch to the official config") {
		t.Fatalf("already on the official config: %q", tt.Texts())
	}
}
