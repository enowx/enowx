package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

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
