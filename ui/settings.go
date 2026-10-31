package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/libraryview"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The Settings page: its sections listed on its left, grouped (agents,
// the host, the account, the app), the one picked beside. The first is the
// library of what the agents on the connected host use (package
// libraryview), with its own list of skills, MCP servers and the rest.

const pageSettings = "settings"

// libraryOptions connect the library to the connected host.
func (a *App) libraryOptions() libraryview.Options {
	ed := a.preferredEditor()
	return libraryview.Options{
		Client:       a.client,
		Host:         a.host.Name,
		Update:       a.onMain,
		OpenInEditor: func(path string) { a.openInEditor(ed, path, 0) },
	}
}

// settingsGroups are the sections, grouped as the navigation lists them.
var settingsGroups = []struct {
	label    string
	sections []string
}{
	{"AGENTS", []string{sectionLibrary, sectionSessions, sectionAgents, sectionAutomations, sectionHooks, sectionDecide, sectionRAG}},
	{"HOST", []string{sectionEnvironment, sectionTerminal, sectionFeatures, sectionSync, sectionDiagnostics}},
	{"ACCOUNT", []string{sectionCloud}},
	{"APP", []string{sectionApp, sectionFloating, sectionNotify, sectionDiscord}},
}

// settingsSections are the sections, in the navigation's order.
func settingsSections() []string {
	var out []string
	for _, g := range settingsGroups {
		out = append(out, g.sections...)
	}
	return out
}

var sectionIcons = map[string]*ui.SVG{
	sectionLibrary: icon.Brain, sectionSessions: icon.Plus, sectionAgents: icon.Bot,
	sectionAutomations: icon.History, sectionHooks: icon.Terminal, sectionRAG: icon.Search, sectionDecide: icon.Shield, sectionEnvironment: icon.Lock, sectionTerminal: icon.Terminal, sectionFeatures: icon.Layers,
	sectionSync: icon.Sync, sectionDiagnostics: icon.Gauge, sectionCloud: icon.Globe, sectionApp: icon.AppWindow,
	sectionNotify: icon.VolumeUp, sectionFloating: icon.Floating, sectionDiscord: icon.Chat,
}

// settingsNarrow is the width under which the navigation shows its icons
// only, the room going to the section.
const settingsNarrow = 760

func (a *App) settingsPage(c *ui.Context) {
	if a.library == nil || a.libraryHost != a.host.ID {
		a.library, a.libraryHost = libraryview.New(), a.host.ID
	}
	if a.settingsSection == "" {
		a.settingsSection = sectionLibrary
	}
	a.viewHeader(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Breadcrumb(c, "Settings", a.host.Name, a.settingsSection) })
		if a.settingsSection == sectionLibrary && a.host.ID != hosts.LocalID && a.client != nil && widget.GhostButton(c, "Sync…").Tooltip("Copy items to another host").Clicked() {
			a.openSync()
		}
		if widget.IconButton(c, icon.Close, "Back to sessions", false).Clicked() {
			a.page = ""
		}
	})
	body := ui.Row(c.Key("settings-body")).Grow(1).FillWidth().MinHeight(0)
	// The window's width, not the navigation's: it doesn't change with
	// what the navigation shows, so the choice holds from frame to frame.
	narrow := body.Bounds().W > 0 && body.Bounds().W < settingsNarrow
	body.Children(func() {
		a.settingsNav(c, narrow)
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() { a.settingsBody(c) })
	})
}

// settingsNav lists the sections, grouped: the page's own navigation.
func (a *App) settingsNav(c *ui.Context, narrow bool) {
	width := float32(212)
	if narrow {
		width = 56
	}
	ui.Scroll(c.Key("settings-nav")).Width(width).Shrink(0).FillHeight().BorderWidth(0, 1, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Column(c).FillWidth().Gap(2).Padding(12, 8).Children(func() {
			for i, g := range settingsGroups {
				if narrow {
					if i > 0 {
						ui.Box(c).Height(1).FillWidth().Margin(6, 6).Background(theme.Border)
					}
				} else {
					top := float32(12)
					if i == 0 {
						top = 2
					}
					widget.SectionLabel(c, g.label, theme.Muted).Padding(top, 10, 4, 10)
				}
				for _, name := range g.sections {
					a.settingsNavItem(c, name, narrow)
				}
			}
		})
	})
}

func (a *App) settingsNavItem(c *ui.Context, name string, narrow bool) {
	on := a.settingsSection == name
	b := ui.ButtonBase(c.Key("nav:" + name)).FillWidth().Height(32).PaddingX(10).Gap(10).Radius(theme.RadiusControl).
		AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).Label(name)
	if narrow {
		b.Tooltip(name).Justify(ui.Center).PaddingX(0)
	}
	fg := theme.TextSoft
	switch {
	case on:
		b.Background(theme.Selected)
		fg = theme.Text
	case b.Hovered():
		b.Background(theme.Hover)
		fg = theme.Text
	}
	b.TextColor(fg).Children(func() {
		ui.Icon(c, sectionIcons[name]).FontSize(14)
		if !narrow {
			ui.Text(c, name).FontSize(12.5).SingleLine().Shrink(1)
		}
	})
	if widget.Pressable(b).Clicked() && !on {
		a.settingsSection = name
		if a.hostSet != nil && name != sectionLibrary {
			a.loadHostSettings()
		}
	}
}
