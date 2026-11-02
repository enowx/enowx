package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/libraryview"
	"github.com/enowx/enowx/ui/widget"
)

// The Settings page: its sections listed beside the frame, grouped (agents,
// the host, the account, the app), the one picked in it. The first is the
// library of what the agents on the connected host use (package
// libraryview), with its own list of skills, MCP servers and the rest.

const pageSettings = "settings"

// libraryOptions connect the library to the connected host.
func (a *App) libraryOptions() libraryview.Options {
	ed := a.preferredEditor()
	var sync func()
	if a.host.ID != hosts.LocalID && a.client != nil {
		sync = a.openSync
	}
	return libraryview.Options{
		Client:       a.client,
		Host:         a.host.Name,
		Update:       a.onMain,
		OpenInEditor: func(path string) { a.openInEditor(ed, path, 0) },
		Sync:         sync,
	}
}

// settingsGroups are the sections, grouped as the navigation lists them.
var settingsGroups = []struct {
	label    string
	sections []string
}{
	{"Agents", []string{sectionLibrary, sectionSessions, sectionAgents, sectionAutomations, sectionHooks, sectionDecide, sectionRAG}},
	{"Host", []string{sectionEnvironment, sectionTerminal, sectionFeatures, sectionSync, sectionDiagnostics}},
	{"Account", []string{sectionCloud}},
	{"App", []string{sectionApp, sectionFloating, sectionNotify, sectionDiscord}},
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
	sectionAutomations: icon.History, sectionHooks: icon.Terminal, sectionRAG: icon.Search, sectionDecide: icon.Gauge, sectionEnvironment: icon.Key, sectionTerminal: icon.Terminal, sectionFeatures: icon.Layers,
	sectionSync: icon.Sync, sectionDiagnostics: icon.Gauge, sectionCloud: icon.Account, sectionApp: icon.Settings,
	sectionNotify: icon.VolumeUp, sectionFloating: icon.Floating, sectionDiscord: icon.Chat,
}

// settingsPage is the frame's side of the page: the section picked. The
// sections are listed beside the frame (settingsSide), and each section heads
// the frame with what it is and what it can do.
func (a *App) settingsPage(c *ui.Context) {
	if a.library == nil || a.libraryHost != a.host.ID {
		a.library, a.libraryHost = libraryview.New(), a.host.ID
	}
	if a.settingsSection == "" {
		a.settingsSection = sectionLibrary
	}
	a.settingsBody(c)
}

// settingsSide lists the sections beside the frame, grouped. It keeps its width
// and its words whatever the window: design-1's side list replaced the narrow
// state of icons alone, so no such state is left.
func (a *App) settingsSide(c *ui.Context) {
	widget.SidePanel(c, 248, func() {
		for _, g := range settingsGroups {
			widget.SideLabel(c, g.label)
			for _, name := range g.sections {
				on := a.settingsSection == name || a.settingsSection == "" && name == sectionLibrary
				if widget.SideEntry(c, sectionIcons[name], name, on).Clicked() && !on {
					a.settingsSection = name
					if a.hostSet != nil && name != sectionLibrary {
						a.loadHostSettings()
					}
				}
			}
		}
	})
}

// settingsOn says where the host's name reads in a section's header.
func (a *App) settingsOn() string { return "On " + a.host.Name }
