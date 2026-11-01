package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// Settings lists its sections beside the frame, grouped, every one of them
// there; picking one shows it.
func TestSettingsNavigation(t *testing.T) {
	app := New(Options{})
	app.page = pageSettings
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Width(1200).Height(700).Background(theme.Background).Children(func() {
			app.settingsSide(c)
			ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() { app.settingsPage(c) })
		})
	}, 1200, 700)
	tt.Frame()
	tt.Frame()
	shot(t, tt, "settings-nav.png")
	list := settingsSections()
	for _, name := range list {
		if _, ok := tt.Find(name); !ok {
			t.Errorf("%q isn't in the navigation", name)
		}
	}
	if len(list) != len(sectionIcons) {
		t.Fatalf("%d sections listed, %d with icons", len(list), len(sectionIcons))
	}
	if !tt.HasText("Agents") || !tt.HasText("Account") {
		t.Errorf("the groups' labels don't show")
	}
	if err := tt.Click(sectionCloud); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.settingsSection != sectionCloud {
		t.Fatalf("picking Cloud shows %q", app.settingsSection)
	}
}
