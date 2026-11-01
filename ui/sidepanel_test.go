//go:build !windows

package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/ui/theme"
)

// The dock picks the panel's tool, and the tool's button folds it again; what
// doesn't fit in the dock is in its menu.
func TestDockPicksThePanelsTool(t *testing.T) {
	app := New(Options{})
	app.client = &client.Client{}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(600).Height(500).Background(theme.Background).TextColor(theme.Text).Children(func() { app.toolsDock(c) })
	}, 600, 500)
	tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%v: %q", err, tt.Texts())
		}
		tt.Frame()
	}
	click("Files")
	if !app.state.Panel.Open || app.state.Panel.Tab != panelFiles {
		t.Fatalf("Files: %+v", app.state.Panel)
	}
	click("Files")
	if app.state.Panel.Open {
		t.Fatal("Files again doesn't fold the panel")
	}
	click("More tools")
	click("Ports")
	if !app.state.Panel.Open || app.state.Panel.Tab != panelPorts {
		t.Fatalf("Ports: %+v", app.state.Panel)
	}
}
