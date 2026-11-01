package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The side panel sits right of the workspace, about the selected session,
// and folds away; the dock beside it picks its tool.

// panelTool is a tool the side panel can open as a tab.
type panelTool struct {
	id, name string
	svg      *ui.SVG
}

// Tools of the side panel, in the order its launchers show them.
const (
	panelChanges  = "git"
	panelFiles    = "files"
	panelPorts    = "ports"
	panelActivity = "activity"
	panelChecks   = "checks"
	panelBrowser  = "browser"
	panelTraffic  = "traffic"
	panelAndroid  = "android"
)

var panelTools = []panelTool{
	{panelChanges, "Git", icon.Git},
	{panelChecks, "Checks", icon.Check},
	{panelFiles, "Files", icon.Folder},
	{panelIndex, "Index", icon.Search},
	{panelPorts, "Ports", icon.Globe},
	{panelBrowser, "Browser", icon.Browser},
	{panelTraffic, "Traffic", icon.Traffic},
	{panelAndroid, "Android", icon.Android},
	{panelActivity, "Activity", icon.List},
	{panelChat, "Chat", icon.Chat},
	{panelTeamProjects, "Projects", icon.Folder},
	{panelTeamMembers, "Members", icon.Users},
	{panelSessions, "Sessions", icon.Bot},
	{panelTasks, "Tasks", icon.List},
}

// panelGroup is a group of the panel's tools, its first level: picking it
// shows the tool it showed last.
type panelGroup struct {
	id, name string
	svg      *ui.SVG
	tools    []string
}

// panelGroups group the tools by what they are for.
var panelGroups = []panelGroup{
	{"code", "Code", icon.Git, []string{panelChanges, panelFiles, panelChecks, panelIndex}},
	{"preview", "Preview", icon.Browser, []string{panelBrowser, panelPorts, panelAndroid}},
	{"debug", "Debug", icon.Traffic, []string{panelTraffic, panelActivity}},
	{"team", "Team", icon.Account, []string{panelChat, panelTeamProjects, panelTeamMembers}}, // while in a team (shownGroups)
	{"goa", "GOA", icon.Sparkle, []string{panelSessions, panelTasks}},                        // in GOD of Agents mode (goa_panel.go)
}

// shownGroups are the groups the panel offers: Team while the host is in
// a team, GOA (first) in GOD of Agents mode.
func (a *App) shownGroups() []panelGroup {
	// The spaces as last read (the rail reads them): drawing starts no read.
	inTeam := a.client != nil && a.spaceList != nil && a.spaceList.host == a.host.ID && len(a.spaceList.list) > 0
	goaOn := a.goaOn()
	out := slices.DeleteFunc(slices.Clone(panelGroups), func(g panelGroup) bool { return g.id == "team" && !inTeam || g.id == "goa" })
	if goaOn {
		out = append([]panelGroup{panelGroups[len(panelGroups)-1]}, out...)
	}
	return out
}

// groupOf is the group of tab, a tool or a file (Code's).
func groupOf(tab string) panelGroup {
	for _, g := range panelGroups {
		if slices.Contains(g.tools, tab) {
			return g
		}
	}
	return panelGroups[0]
}

func findTool(id string) (panelTool, bool) {
	i := slices.IndexFunc(panelTools, func(t panelTool) bool { return t.id == id })
	if i < 0 {
		return panelTool{}, false
	}
	return panelTools[i], true
}

// sidePanel is the side panel's state, remembered between launches.
type sidePanel struct {
	Open bool   `json:"open,omitempty"`
	Tab  string `json:"tab,omitempty"` // the tool it shows
	// Last is the tool each group showed last, by group.
	Last  map[string]string `json:"last,omitempty"`
	Width float32           `json:"width,omitempty"` // in DIPs; 0 for the default
	// Team is the team the Team group shows, as the user switched to it;
	// "" for the selected session's, or the page's.
	Team string `json:"team,omitempty"`
}

// The side panel's width: as wide as the user dragged it, within these.
// A narrow window shrinks it down to its minimum before the workspace.
const (
	defaultPanelWidth = 436
	minPanelWidth     = 300
	maxPanelWidth     = 1100
	panelGrip         = 6 // the strip along its left edge that resizes it
)

// panelTab is what the panel shows: a tool, or a file of the selected
// session ("file:<path>"). A file the session hasn't open gives Files.
func (a *App) panelTab() string {
	tab := a.state.Panel.Tab
	if _, ok := findTool(tab); ok {
		return tab
	}
	if _, ok := a.fileTabOf(a.selected, tab); ok {
		return tab
	}
	if strings.HasPrefix(tab, fileTabPrefix) {
		return panelFiles
	}
	return panelTools[0].id
}

// panelShown reports whether the side panel shows: open, beside a
// session's workspace (not a rail page).
func (a *App) panelShown() bool { return a.state.Panel.Open && a.page == "" && a.client != nil }

// panelShows reports whether the side panel shows tool.
func (a *App) panelShows(tool string) bool { return a.panelShown() && a.panelTab() == tool }

// openPanel shows the panel with tool in it.
func (a *App) openPanel(tool string) {
	a.page = ""
	a.state.Panel.Open, a.state.Panel.Tab = true, tool
}

// showBrowserSoon marks the Browser for a page an agent or a dev server
// opened: the panel stays as the user has it.
func (a *App) showBrowserSoon() {
	if !a.panelShows(panelBrowser) {
		a.browserNews = true
	}
}

// togglePanel folds the panel, or shows it with the tool it showed.
func (a *App) togglePanel() {
	if a.panelShown() {
		a.state.Panel.Open = false
		return
	}
	a.openPanel(a.panelTab())
}

// toggleTool shows tool in the panel, or folds the panel if it shows it.
func (a *App) toggleTool(tool string) {
	if a.panelShows(tool) {
		a.state.Panel.Open = false
		return
	}
	a.openPanel(tool)
}

// sidePanelView is the panel: its tab strip, then the tool in front, for
// the selected session. It slides in from the right as the session list
// does from the left, and its left edge resizes it.
func (a *App) sidePanelView(c *ui.Context) {
	if !a.panelShown() {
		return
	}
	width := a.state.Panel.Width
	if width <= 0 {
		width = defaultPanelWidth
	}
	widget.Bento(ui.Column(c.Key("side-panel")).Width(width).MinWidth(minPanelWidth).Shrink(1).FillHeight(), theme.Fade(theme.Panel, 0.94)).
		Transition(widget.PanelMotion(24)).Children(func() {
		a.panelStrip(c)
		s, ok := a.selectedSession()
		switch tool := a.panelTab(); {
		case tool == panelChat:
			a.chatTab(c.Key("chat")) // the team's, whichever session
		case tool == panelTeamProjects:
			a.teamProjectsTab(c.Key("team-projects"))
		case tool == panelTeamMembers:
			a.teamMembersTab(c.Key("team-members"))
		case tool == panelSessions:
			a.sessionsTab(c.Key("goa-sessions")) // every host's (goa_panel.go)
		case tool == panelTasks:
			a.tasksTab(c.Key("goa-tasks"))
		case !ok:
			panelNote(c, "Select a session to see its tools.")
		case tool == panelChanges && !a.canReview(s):
			panelNote(c, "“"+a.sessionName(s)+"” isn't in a git repository.")
		case tool == panelChanges:
			a.changesTab(c.Key("changes:"+s.ID), s)
		case tool == panelFiles:
			a.filesTab(c.Key("files:"+s.ID), s)
		case tool == panelPorts:
			a.portsTab(c.Key("ports:"+s.ID), s)
		case tool == panelBrowser && a.state.BrowserViews[s.ID]:
			// A web view shows in one place: the session shows it.
			panelNote(c, "The preview shows as “"+a.sessionName(s)+"”'s view. Switch it to Terminal or Chat to have it here.")
		case tool == panelBrowser:
			a.browserTab(c.Key("browser:"+s.ID), s)
		case tool == panelAndroid:
			a.androidTab(c.Key("android"), s)
		case tool == panelTraffic && a.state.TrafficViews[s.ID]:
			panelNote(c, "Traffic shows as “"+a.sessionName(s)+"”'s view. Switch it to Terminal or Chat to have it here.")
		case tool == panelTraffic:
			a.trafficTab(c.Key("traffic"), s)
		case tool == panelIndex:
			a.indexTab(c.Key("index:"+s.ID), s)
		case tool == panelChecks:
			a.checksTab(c.Key("checks:"+s.ID), s)
		case tool == panelActivity:
			a.activityTab(c.Key("activity:"+s.ID), s)
		case strings.HasPrefix(tool, fileTabPrefix):
			a.docTab(c.Key("doc:"+s.ID+":"+tool), s, strings.TrimPrefix(tool, fileTabPrefix))
		}
		a.panelGrip(c, width)
	})
}

// panelGrip is the panel's left edge, which the pointer drags to resize
// it; it shows a line only while hovered or dragged.
func (a *App) panelGrip(c *ui.Context, width float32) {
	grip := ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).Width(panelGrip).Cursor(ui.CursorResizeEW).Label("Resize the panel")
	dx, _, held := grip.Dragged()
	if held && dx != 0 {
		a.state.Panel.Width = min(max(width-dx, minPanelWidth), maxPanelWidth)
	}
	if held || grip.Hovered() {
		grip.Children(func() {
			ui.Box(c).Width(2).FillHeight().Background(theme.Accent)
		})
	}
}

// panelStrip is the panel's top: the tool's name and the button that folds
// the panel; the tools themselves are in the dock. Code's open files follow
// as tabs.
func (a *App) panelStrip(c *ui.Context) {
	front := a.panelTab()
	if front == panelBrowser {
		a.browserNews = false // looked at
	}
	group := groupOf(front)
	if a.state.Panel.Last == nil {
		a.state.Panel.Last = map[string]string{}
	}
	a.state.Panel.Last[group.id] = front
	ui.Row(c).Height(52).Shrink(0).FillWidth().Padding(0, theme.S2, 0, theme.S4).Gap(theme.S2).Children(func() {
		name := "Files"
		if t, ok := findTool(front); ok {
			name = t.name
		}
		ui.Text(c, name).FontSize(theme.TextRead).FontWeight(600).SingleLine().Grow(1).MinWidth(0)
		if widget.IconButton(c, icon.Close, "Hide the panel ("+modKey+"⇧D)", false).Clicked() {
			a.state.Panel.Open = false
		}
	})
	if tabs := a.fileTabs[a.selected]; group.id == "code" && len(tabs) > 0 && (front == panelFiles || strings.HasPrefix(front, fileTabPrefix)) {
		ui.Row(c).Height(36).Shrink(0).FillWidth().PaddingX(theme.S3).Gap(theme.S1).Clip().Children(func() {
			for _, p := range tabs {
				a.fileTabButton(c.Key("file:"+p), p, fileTabPrefix+p == front)
			}
		})
	}
}

// dockFirst are the tools the dock always shows; the others are in its menu.
var dockFirst = []string{panelChanges, panelFiles, panelChecks, panelBrowser, panelTraffic}

// toolsDock is the column of tools at the window's right, while a session
// shows: the conversation keeps the room until one is asked for.
func (a *App) toolsDock(c *ui.Context) {
	if a.page != "" || a.client == nil {
		return
	}
	front := ""
	if a.panelShown() {
		front = a.panelTab()
		if strings.HasPrefix(front, fileTabPrefix) {
			front = panelFiles
		}
	}
	first := dockFirst
	var rest []panelTool
	if a.goaOn() {
		first = append([]string{panelSessions, panelTasks}, first...)
	}
	for _, g := range a.shownGroups() {
		for _, id := range g.tools {
			if !slices.Contains(first, id) {
				t, _ := findTool(id)
				rest = append(rest, t)
			}
		}
	}
	ui.Column(c.Key("dock-room")).Width(44).Shrink(0).FillHeight().Justify(ui.Center).Children(func() {
		widget.Glass(ui.Column(c.Key("dock")).FillWidth().Padding(theme.S1, 0).Gap(theme.S1).Radius(theme.RadiusFull).AlignItems(ui.Center)).Children(func() {
			for _, id := range first {
				t, _ := findTool(id)
				if a.dockButton(c.Key("tool:"+id), t, id == front).Clicked() {
					a.toggleTool(id)
				}
			}
			more := dockFace(c.Key("tool-more"), icon.More, "More tools", slices.ContainsFunc(rest, func(t panelTool) bool { return t.id == front }))
			a.menu.OpenBelow(more, "tool-more", 36)
			a.menu.Show(c, more, "tool-more", func(done func()) {
				for _, t := range rest {
					if widget.MenuItem(c.Key(t.id), t.svg, t.name, false) {
						a.openPanel(t.id)
						done()
					}
				}
			})
		})
	})
}

// dockFace is the round button of a dock's entry.
func dockFace(c *ui.Context, svg *ui.SVG, label string, on bool) ui.Element {
	b := ui.ButtonBase(c).Size(36, 36).Radius(theme.RadiusFull).Center().Shrink(0).Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	bg, fg := theme.Fade(theme.Selected, 0), theme.TextSoft
	if on || b.Hovered() {
		bg, fg = theme.Selected, theme.Text
	}
	b.Background(widget.QuickTint(b, "bg", bg)).TextColor(widget.QuickTint(b, "fg", fg)).Children(func() {
		ui.Icon(c, svg).FontSize(16)
	})
	return b
}

// dockButton is a tool's entry of the dock, with its news.
func (a *App) dockButton(c *ui.Context, t panelTool, front bool) ui.Element {
	b := dockFace(c, t.svg, t.name, front)
	if t.id == panelBrowser && a.browserNews && !front {
		ui.Box(c).Size(8, 8).AttachTo(b, ui.AnchorTopRight, ui.AnchorTopRight).Margin(4, 4, 0, 0).Radius(theme.RadiusFull).Background(theme.Now)
	}
	return b
}

// fileTabButton is the tab of a file: its name, and an × that closes it.
func (a *App) fileTabButton(c *ui.Context, p string, front bool) {
	name := fileTabName(p)
	b := ui.ButtonBase(c).Height(28).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).Shrink(1).MinWidth(64).
		Label(p).Tooltip(p).Cursor(ui.CursorPointer)
	bg, fg := theme.Fade(theme.Selected, 0), theme.TextSoft
	switch {
	case front:
		bg, fg = theme.Selected, theme.TextStrong
	case b.Hovered():
		bg, fg = theme.Hover, theme.Text
	}
	b.Background(widget.QuickTint(b, "bg", bg)).TextColor(widget.QuickTint(b, "fg", fg)).Transition(widget.QuickMotion)
	closed := false
	b.Children(func() {
		ui.Icon(c, icon.File).FontSize(12)
		widget.Running(c, name, widget.RunStyle{})
		x := ui.ButtonBase(c).Size(16, 16).Shrink(0).Radius(theme.RadiusFull).Center().Cursor(ui.CursorPointer).Label("Close " + name)
		if x.Hovered() {
			x.Background(theme.Hover)
		}
		x.Children(func() { ui.Icon(c, icon.Close).FontSize(10).TextColor(theme.Muted) })
		closed = x.Clicked()
	})
	switch {
	case closed:
		a.closeFileTab(a.selected, p)
	case b.Clicked():
		a.state.Panel.Tab = fileTabPrefix + p
	}
}

func panelNote(c *ui.Context, text string) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(theme.TextSoft).TextAlign(ui.Center)
	})
}

// newsDot marks a button whose tool has something new.
func newsDot(c *ui.Context) {
	ui.Box(c).Size(8, 8).Radius(theme.RadiusFull).Shrink(0).Background(theme.Now)
}
