package ui

import (
	"path/filepath"
	"testing"

	mygoui "github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

func appearanceTester(t *testing.T, app *App, width, height int) *mygoui.Tester {
	t.Helper()
	tt := mygoui.NewTester(func(c *mygoui.Context) {
		c.SetTheme(app.theme)
		mygoui.Column(c).Fill().Background(theme.Background).TextColor(theme.Text).Children(func() { app.appearancePage(c) })
	}, width, height)
	tt.Frame()
	return tt
}

func TestAppearanceModePersistsWithoutLosingPaletteOrAccent(t *testing.T) {
	old := theme.Current()
	defer theme.Apply(old)
	file := filepath.Join(t.TempDir(), "state.json")
	app := New(Options{StateFile: file})
	app.host = LocalHost()
	tt := appearanceTester(t, app, 1200, 800)
	if err := tt.Click("Light"); err != nil {
		t.Fatal(err)
	}
	if app.theme.Dark || !theme.Current().Light {
		t.Fatal("Light did not update the app appearance")
	}
	if err := tt.Click("Midnight"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("#0E7490"); err != nil {
		t.Fatal(err)
	}
	app.saveState()
	reopened := New(Options{StateFile: file})
	if reopened.state.ThemeMode != "light" || reopened.state.Theme != "midnight" || reopened.state.Accent != "#0E7490" || reopened.theme.Dark {
		t.Fatal("appearance did not survive reopening")
	}
	if err := tt.Click("Dark"); err != nil {
		t.Fatal(err)
	}
	if !app.theme.Dark || app.state.Theme != "midnight" || app.state.Accent != "#0E7490" {
		t.Fatal("switching mode lost the palette or accent")
	}
}

func TestSystemAppearanceFollowsChangesOnlyWhenSelected(t *testing.T) {
	old := theme.Current()
	defer theme.Apply(old)
	app := New(Options{})
	tt := appearanceTester(t, app, 900, 800)
	if err := tt.Click("System"); err != nil {
		t.Fatal(err)
	}
	app.systemAppearanceChanged(false)
	tt.Frame()
	if app.theme.Dark {
		t.Fatal("system light appearance was ignored")
	}
	app.systemAppearanceChanged(true)
	tt.Frame()
	if !app.theme.Dark {
		t.Fatal("system dark appearance was ignored")
	}
	if err := tt.Click("Light"); err != nil {
		t.Fatal(err)
	}
	app.systemAppearanceChanged(false)
	app.systemAppearanceChanged(true)
	tt.Frame()
	if app.theme.Dark || app.state.ThemeMode != "light" {
		t.Fatal("system changes overrode explicit Light mode")
	}
}
