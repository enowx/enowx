package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/tokenview"
	"github.com/enowx/enowx/ui/widget"
)

// sessionsBody is the Sessions list: how it is arranged, folding it, and the
// sessions by state or by project.
func (a *App) sessionsBody(c *ui.Context) {
	ui.Row(c).FillWidth().Shrink(0).Padding(theme.S2, theme.S2, 0, theme.S2).Gap(theme.S1).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			current := 0
			if a.state.SessionsByProject {
				current = 1
			}
			if i := widget.Segmented(c.Key("list-mode"), []string{"Sessions only", "By project"}, current, nil); i >= 0 {
				a.state.SessionsByProject = i == 1
			}
		})
		if widget.IconButton(c, icon.Sidebar, "Collapse the sidebar ("+modKey+"B)", false).Clicked() {
			a.state.RailWide = false
		}
	})
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(theme.S1, theme.S2, theme.S2, theme.S2).Children(func() {
			if len(a.projects) == 0 {
				msg := "No projects yet. Open a folder to start."
				if a.client == nil {
					msg = "Not connected."
				}
				ui.Text(c, msg).FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3)
			}
			if a.state.SessionsByProject {
				a.sessionsByProject(c)
			} else {
				a.sessionsByState(c)
			}
			a.otherHostsSessions(c)
		})
	})
}

// sessionsByProject is the list by project: a parent row each, its sessions
// nested under it.
func (a *App) sessionsByProject(c *ui.Context) {
	shown := 0
	for _, e := range a.projectEntries() {
		if e.group != "" {
			shown += a.groupSection(c.Key("group:"+e.group), e)
		} else if a.projectGroup(c.Key(e.projects[0].Path), e.projects[0]) {
			shown++
		}
	}
	if a.filter != "" && shown == 0 && len(a.projects) > 0 {
		ui.Text(c, "Nothing matches.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3)
	}
}

// stateGroups are the groups of the list by state, in the order they matter.
var stateGroups = []struct {
	label string
	has   func(proto.SessionInfo) bool
}{
	{"Waiting on you", func(s proto.SessionInfo) bool { return !s.Exited && s.State == proto.StateWaiting }},
	{"Working", isWorking},
	{"Failed", sessionFailed},
	{"Done", sessionDone},
	{"Idle", func(proto.SessionInfo) bool { return true }},
}

// sessionFailed says s ended with an error.
func sessionFailed(s proto.SessionInfo) bool { return statusOf(s).color == theme.Error }

// sessionDone says s has ended well, or its agent finished its turn.
func sessionDone(s proto.SessionInfo) bool {
	return statusOf(s).label == "done"
}

// sessionsByState is the list by state: every project's sessions together,
// each row naming its project quietly.
func (a *App) sessionsByState(c *ui.Context) {
	placed := map[string]bool{}
	for _, g := range stateGroups {
		var list []proto.SessionInfo
		var names []string
		for _, e := range a.projectEntries() {
			for _, p := range e.projects {
				for _, s := range a.projectSessions(p.Path) {
					if !placed[s.ID] && g.has(s) {
						placed[s.ID] = true
						list, names = append(list, s), append(names, p.Name)
					}
				}
			}
		}
		if len(list) == 0 {
			continue
		}
		widget.SectionLabel(c.Key("state:"+g.label), g.label, theme.TextSoft).Padding(theme.S3, theme.S3, theme.S1, theme.S3)
		for i, s := range list {
			a.sessionRow(c.Key(s.ID), i, s, names[i])
		}
	}
}

// projectGroup draws a project and its sessions, and reports whether it
// drew anything: while filtering, projects without a match are hidden.
func (a *App) projectGroup(c *ui.Context, p proto.ProjectInfo) bool {
	sessions := a.projectSessions(p.Path)
	filtering := strings.TrimSpace(a.filter) != ""
	if filtering {
		sessions = a.matching(p, sessions)
		if len(sessions) == 0 {
			return false
		}
	}
	open := filtering || !a.collapsed[p.Path]
	selected := p.Path == a.project

	ui.Row(c).Height(theme.ControlHeight).FillWidth().Margin(theme.S1, 0, 0, 0).Children(func() {
		row := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(theme.ControlHeight).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).Justify(ui.Start).Cursor(ui.CursorPointer).Label(p.Name)
		row.Drag(listDrag{path: p.Path})
		a.dropOnProject(row, p)
		if row.Hovered() {
			row.Background(theme.Hover)
		}
		row.Children(func() {
			chev := icon.ChevronRight
			if open {
				chev = icon.ChevronDown
			}
			ui.Icon(c, chev).FontSize(12).TextColor(theme.TextSoft)
			col := theme.TextSoft
			if selected {
				col = theme.TextStrong
			}
			ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
				widget.Running(c, p.Name, widget.RunStyle{Weight: 500, Color: col})
			})
			a.teamProjectMark(c, p.Path)
			ui.Spacer(c)
			if !open {
				a.projectSignal(c, sessions)
			}
			ui.Text(c, strconv.Itoa(len(sessions))).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		})
		if row.Clicked() {
			if selected || filtering {
				a.collapsed[p.Path] = open
			} else {
				a.project = p.Path
				a.selected = ""
				a.page = ""
				delete(a.collapsed, p.Path)
			}
		}
		a.menu.Open(row, "project:"+p.Path)
		a.menu.Show(c, row, "project:"+p.Path, func(done func()) { a.projectMenuItems(c, p, done) })
		if n := a.linkConflictCount(p.Path); n > 0 {
			warn := smallIcon(c, icon.Alert, fmt.Sprintf("%d file(s) in conflict with its other copies: pick versions", n))
			warn.TextColor(theme.Warning)
			if warn.Clicked() {
				a.openLinks(p)
			}
		}
		if smallIcon(c, icon.Plus, "New session in "+p.Name).Clicked() {
			a.openNewSession(p)
		}
	})
	if !open {
		return true
	}
	ui.Column(c.Key("kids:"+p.Path)).FillWidth().Padding(0, 0, 0, theme.S4).Children(func() {
		if !filtering {
			a.pipelineCards(c, p)
		}
		if len(sessions) == 0 {
			ui.Text(c, "No sessions yet").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Padding(theme.S1, theme.S3)
		}
		for i, s := range sessions {
			a.sessionRow(c.Key(s.ID), i, s, "")
		}
		if !filtering {
			a.projectNotesRow(c, p)
		}
	})
	return true
}

// smallIcon is the 24-DIP round button of a list's row.
func smallIcon(c *ui.Context, svg *ui.SVG, label string) ui.Element {
	b := ui.ButtonBase(c).Size(24, 24).Radius(theme.RadiusFull).Center().Shrink(0).Label(label).Tooltip(label).Cursor(ui.CursorPointer).TextColor(theme.TextSoft)
	if b.Hovered() {
		b.Background(theme.Hover).TextColor(theme.Text)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(12) })
	return b
}

// projectSignal says what a folded project's sessions need: those waiting or
// failed, what the row keeps in sight while they are out of it.
func (a *App) projectSignal(c *ui.Context, sessions []proto.SessionInfo) {
	waiting, failed := 0, 0
	for _, s := range sessions {
		switch {
		case !s.Exited && s.State == proto.StateWaiting:
			waiting++
		case sessionFailed(s):
			failed++
		}
	}
	signal := func(n int, col ui.Color, word string) {
		if n == 0 {
			return
		}
		ui.Row(c).Gap(theme.S1).Shrink(0).Children(func() {
			widget.Dot(c, col, false, 8)
			ui.Text(c, strconv.Itoa(n)+" "+word).FontSize(theme.TextLabel).FontWeight(400).SingleLine()
		})
	}
	signal(waiting, theme.Warning, "waiting")
	signal(failed, theme.Error, "failed")
}

// projectMenuItems are a project's menu: its right-click menu and its
// More button's.
func (a *App) projectMenuItems(c *ui.Context, p proto.ProjectInfo, done func()) {
	if widget.MenuItem(c, icon.Plus, "New session", false) {
		a.openNewSession(p)
		done()
	}
	if a.client != nil && widget.MenuItemTip(c, icon.Chat, "History…", "The agents' past conversations in this project, to resume", false) {
		a.openHistory(p)
		done()
	}
	if a.client != nil && p.IsGit && widget.MenuItemTip(c, icon.List, "New pipeline…", "Agents one after another on a branch: plan, build, review", false) {
		a.openPipeline(p)
		done()
	}
	if a.client != nil && widget.MenuItemTip(c, icon.Gauge, "Token usage", "The tokens and cost of this project's agents", false) {
		a.openProjectTokens(p.Path)
		done()
	}
	if a.client != nil && widget.MenuItemTip(c, icon.List, "Rules…", "What every session in this project reads first", false) {
		a.openRules(p)
		done()
	}
	if a.client != nil && widget.MenuItemTip(c, icon.Notebook, "New note", "A note attached to this project", false) {
		a.newNoteFor(p)
		done()
	}
	if n := len(a.notesOfProject(p.Path)); a.client != nil && n > 0 && widget.MenuItem(c, icon.Notebook, fmt.Sprintf("Notes (%d)", n), false) {
		a.showProjectNotes(p.Path)
		done()
	}
	if a.client != nil && p.IsGit && len(a.allHosts()) > 1 && widget.MenuItemTip(c, icon.Git, "Clone to host…", "Clone this repository on another host", false) {
		a.openCloneTo(p)
		done()
	}
	if a.client != nil && widget.MenuItemTip(c, icon.Server, "Copy to host…", "Copy the project and its sessions to another host", false) {
		a.openCopy(p)
		done()
	}
	if a.client != nil && len(a.allHosts()) > 1 && widget.MenuItemTip(c, icon.Sync, "Sync with host…", "Sync its files with its copy on another host, now", false) {
		a.openProjectSync(p)
		done()
	}
	if a.client != nil && widget.MenuItemTip(c, icon.ArrowUp, "Auto sync & push…", "Keep its copies on other hosts in sync, and push to a branch of its own, on a schedule", false) {
		a.openLinks(p)
		done()
	}
	if a.client != nil && widget.MenuItemTip(c, icon.Moon, "Hibernate idle", "Hibernate the project's idle sessions, freeing their memory", false) {
		a.hibernateIdle(p.Path)
		done()
	}
	if a.client != nil {
		widget.MenuSeparator(c)
		a.projectArrangeItems(c, p, done)
	}
	widget.MenuSeparator(c)
	if widget.MenuItemTip(c, icon.Trash, "Remove…", "Remove the project from enowX; its files stay", true) {
		a.removing = &p
		done()
	}
}

// removeProjectDialog confirms removing a project from the list. Its
// folder stays on disk; a project with sessions still running can't go,
// as they would be left out of the list.
func (a *App) removeProjectDialog(c *ui.Context) {
	if a.removing == nil {
		return
	}
	p := *a.removing
	running := 0
	for _, s := range a.projectSessions(p.Path) {
		if !s.Exited {
			running++
		}
	}
	open := true
	if running > 0 {
		msg := fmt.Sprintf("%d %s still running in it. Delete %s first.", running,
			textfmt.Plural(running, "session is", "sessions are"), textfmt.Plural(running, "it", "them"))
		if widget.AlertDialog(c, &open, "Can't remove “"+p.Name+"” yet", msg, "OK") != -1 || !open {
			a.removing = nil
		}
		return
	}
	msg := "The folder and its worktrees stay on disk. Open it again to bring it back."
	switch widget.AlertDialog(c, &open, "Remove “"+p.Name+"” from enowX?", msg, "Cancel", "Remove") {
	case 1:
		a.removeProject(p)
		a.removing = nil
	case 0, ui.AlertDismissed:
		a.removing = nil
	}
}

// removeProject forgets p on the host; the projects event updates the
// list.
func (a *App) removeProject(p proto.ProjectInfo) {
	cl := a.client
	if cl == nil {
		return
	}
	if a.project == p.Path {
		a.project, a.selected = "", ""
	}
	go func() {
		if err := cl.RemoveProject(p.Path); err != nil {
			a.onMain(func() { a.status = err.Error() })
		}
	}()
}

// matching is the sessions whose name, branch, agent or project contain
// the filter.
func (a *App) matching(p proto.ProjectInfo, sessions []proto.SessionInfo) []proto.SessionInfo {
	q := strings.ToLower(strings.TrimSpace(a.filter))
	var out []proto.SessionInfo
	for _, s := range sessions {
		for _, field := range []string{a.sessionName(s), s.Branch, a.agentName(s.Agent), p.Name} {
			if strings.Contains(strings.ToLower(field), q) {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// sessionRow is a session in the list: its name, and under it project when
// not "" (the list by state says which). The name runs under the pointer when
// the row is too narrow for it; the row keeps its height.
func (a *App) sessionRow(c *ui.Context, i int, s proto.SessionInfo, project string) {
	st := statusOf(s)
	active := s.ID == a.selected
	row := ui.ButtonBase(c).Label(a.sessionName(s)).Transition(widget.RowMotion(i)).Radius(theme.RadiusCard).Cursor(ui.CursorPointer).
		FillWidth().PaddingX(theme.S3).Gap(theme.S2).Justify(ui.Start)
	row.Height(theme.ControlHeight)
	if project != "" {
		row.Height(40)
	}
	if a.picked[s.ID] && !active {
		row.Border(1, theme.Now)
	}
	bg := theme.Fade(theme.Hover, 0)
	if !active && row.Hovered() {
		bg = theme.Hover
	}
	row.Background(widget.QuickTint(row, "bg", bg)).Children(func() {
		if active {
			widget.SelectionPill(c, "sessions", theme.RadiusCard)
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			style := widget.RunStyle{}
			if active || a.unread[s.ID] {
				style.Color = theme.TextStrong
			}
			if a.unread[s.ID] {
				style.Weight = 600
			}
			name := widget.Running(c, a.sessionName(s), style)
			// The agent's note stays out of the row, which keeps every row
			// the same height; hovering the name shows it.
			if s.Note != "" {
				name.Tooltip(s.Note)
			}
			detail := project
			if m := s.Member; m != nil {
				detail = "@" + nonEmpty(m.GitHub, m.Name) + "'s" // a team's member's
			}
			if detail != "" {
				line := widget.Running(c.Key("detail"), detail, widget.RunStyle{Size: theme.TextLabel, Color: rowDetail(active)})
				if t, ok := a.sessionTokens(s.ID); ok {
					line.Tooltip(tokenview.SpentTip(t)) // its tokens on hover
				}
			}
		})
		if cfg := s.Config; cfg != nil {
			widget.BadgeTag(c, cfg.Name, cfg.Color) // the config it runs with
		}
		a.conflictMark(c, s)
		if a.unread[s.ID] {
			ui.Box(c).Size(8, 8).Radius(theme.RadiusFull).Background(theme.Now).Shrink(0).Tooltip("Unread: its agent finished or asks something")
		}
		widget.Dot(c, st.color, st.hollow, 8)
	})
	a.sessionMenu(c, row, s)
	if row.DoubleClicked() {
		a.askRename(s)
	}
	if row.Clicked() {
		if row.ClickModifiers()&(ui.Cmd|ui.Shift) != 0 {
			a.pick(s)
		} else {
			a.picked = nil
			a.selectSession(s)
		}
	}
}

// sessionMenu is a session's right-click menu, also opened by the
// focused session's More button.
func (a *App) sessionMenu(c *ui.Context, anchor ui.Element, s proto.SessionInfo) {
	key := "session:" + s.ID
	a.menu.Open(anchor, key)
	a.menu.Show(c, anchor, key, func(done func()) { a.sessionMenuItems(c, s, done) })
}

func (a *App) sessionMenuItems(c *ui.Context, s proto.SessionInfo, done func()) {
	if canChat(s) {
		label, svg := "Show as chat", icon.Chat
		if a.showsChat(s.ID) {
			label, svg = "Show the terminal", icon.Terminal
		}
		if widget.MenuItem(c, svg, label, false) {
			a.toggleChat(s)
			done()
		}
	}
	if a.sharedProject(s.Project) || s.Private {
		// A session of a project shared with a team shows to it, unless kept.
		label, svg := "Keep from the team", icon.Lock
		if s.Private {
			label, svg = "Show to the team", icon.Eye
		}
		if widget.MenuItem(c, svg, label, false) {
			cl, id, private := a.client, s.ID, !s.Private
			go func() { _ = cl.SetSessionPrivate(id, private) }()
			done()
		}
	}
	if _, isAgent := agent.Find(s.Agent); isAgent && s.Agent != agent.Shell && s.Parent == "" && !s.GOA && widget.MenuItem(c, icon.Split, "Fork…", false) {
		a.askFork(s)
		done()
	}
	label := "Maximize"
	if a.maximized != "" {
		label = "Restore"
	}
	if widget.MenuItem(c, icon.Maximize, label, false) {
		a.selectSession(s)
		a.toggleMaximized(s.ID)
		done()
	}
	if a.canReview(s) && !(a.panelShows(panelChanges) && a.selected == s.ID) && widget.MenuItem(c, icon.Git, "Show changes", false) {
		a.selectSession(s)
		a.openPanel(panelChanges)
		done()
	}
	if !a.unread[s.ID] && widget.MenuItem(c, icon.Chat, "Mark as unread", false) {
		if a.unread == nil {
			a.unread = map[string]bool{}
		}
		a.unread[s.ID] = true
		if a.selected == s.ID {
			a.selected = "" // shown, it would read it again
		}
		done()
	}
	muted := slices.Contains(a.notifySettings().MutedSessions, s.ID)
	if label := map[bool]string{false: "Mute sounds", true: "Unmute sounds"}[muted]; s.Agent != "" && widget.MenuItem(c, icon.VolumeDown, label, false) {
		a.muteSession(s.ID, !muted)
		done()
	}
	if canContinue(s) && widget.MenuItem(c, icon.Play, "Continue", false) {
		a.continueSession(s)
		done()
	}
	if widget.MenuItem(c, icon.Pencil, "Rename…", false) {
		a.askRename(s)
		done()
	}
	a.switchAccountItems(c, s, done)
	a.hibernateItems(c, s, done)
	if !s.Exited && widget.MenuItem(c, icon.Gauge, "Set time budget…", false) {
		a.askBudget(s)
		done()
	}
	for _, ed := range editors {
		if widget.MenuItem(c, icon.Pencil, "Open in "+ed.name, false) {
			a.openInEditor(ed, a.sessionFolder(s), 0)
			done()
		}
	}
	if a.splitOn() && s.Project == a.project && len(a.splitSessions(a.projectSessions(a.project))) > 1 &&
		widget.MenuItemTip(c, icon.Split, "Unsplit", "Leave it out of the sessions side by side", false) {
		a.hideFromSplit(s.ID, true)
		done()
	}
	if i := slices.IndexFunc(a.projects, func(p proto.ProjectInfo) bool { return p.Path == s.Project }); i >= 0 &&
		widget.MenuItemTip(c, icon.Notebook, "New note", "A note attached to its project", false) {
		a.newNoteFor(a.projects[i])
		done()
	}
	widget.MenuSeparator(c)
	if widget.MenuItem(c, icon.Trash, "Delete session…", true) {
		a.askDelete(s)
		done()
	}
}

func summary(sessions []proto.SessionInfo) string {
	working, waiting := 0, 0
	for _, s := range sessions {
		if isWorking(s) {
			working++
		}
		if s.State == proto.StateWaiting && !s.Exited {
			waiting++
		}
	}
	switch {
	case waiting > 0:
		return fmt.Sprintf("%d need input", waiting)
	case working > 0:
		return fmt.Sprintf("%d working", working)
	case len(sessions) > 0:
		return fmt.Sprintf("%d", len(sessions))
	}
	return ""
}

// rowDetail is the color of the quiet line of a row, selected or not.
func rowDetail(active bool) ui.Color {
	if active {
		return theme.Text
	}
	return theme.TextSoft
}
