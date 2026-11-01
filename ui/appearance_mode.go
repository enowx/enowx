package ui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

func (a *App) lightAppearance() bool {
	return a.state.ThemeMode == "light" || (a.state.ThemeMode == "system" && !a.systemDark)
}

func (a *App) appearanceSource() mygo.ThemeSource {
	switch a.state.ThemeMode {
	case "light":
		return mygo.ThemeLight
	case "system":
		return mygo.ThemeSystem
	default:
		return mygo.ThemeDark
	}
}

// startAppearance runs after MyGo is ready, before the first window draws.
func (a *App) startAppearance() {
	// Until a mode is picked, the app is dark and the rest (web pages,
	// native menus) follows the device, as before modes existed: a page's
	// prefers-color-scheme then stays the device's.
	if a.state.ThemeMode != "" {
		mygo.Theme.SetSource(a.appearanceSource())
	}
	a.systemDark = mygo.Theme.IsDark()
	a.applyTheme()
}

func (a *App) setAppearanceMode(mode string) {
	if mode != "light" && mode != "system" {
		mode = "dark"
	}
	a.state.ThemeMode = mode
	if a.win != nil {
		mygo.Theme.SetSource(a.appearanceSource())
		a.systemDark = mygo.Theme.IsDark()
	}
	a.applyTheme()
}

func (a *App) systemAppearanceChanged(dark bool) {
	if a.systemDark == dark {
		return
	}
	a.systemDark = dark
	if a.state.ThemeMode == "system" {
		a.applyTheme()
	}
}

func (a *App) appearanceMode(c *ui.Context) {
	ui.Column(c.Key("appearance-mode")).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
		ui.Text(c, "Appearance mode").FontSize(theme.TextUI).TextColor(theme.TextStrong)
		selected := 0
		switch a.state.ThemeMode {
		case "light":
			selected = 1
		case "system":
			selected = 2
		}
		if picked := widget.Segmented(c, []string{"Dark", "Light", "System"}, selected, nil); picked >= 0 {
			a.setAppearanceMode([]string{"dark", "light", "system"}[picked])
		}
		ui.Text(c, "System follows your device. Your palette and accent are kept when the mode changes.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		ui.Text(c, "Terminals stay dark to keep CLI text readable.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	})
}
