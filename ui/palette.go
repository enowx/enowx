package ui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/ui/diffview"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
	"github.com/enowx/enowx/version"
)

// paletteLimit is how many commands the palette lists at once.
const paletteLimit = 12

// palette is the state of the command palette.
type palette struct {
	open  bool
	query string
	pick  int // the highlighted command among those shown
}

// command is an entry of the palette.
type command struct {
	key, label, detail string
	svg                *ui.SVG
	run                func()
}

func (a *App) openPalette() {
	a.palette = palette{open: true}
	if s, ok := a.selectedSession(); ok {
		a.loadCommands(s) // offered as Run: …
	}
}

// commands are what the palette offers: pages and actions as the design lists
// them, what acts on the session shown, then hosts, then the host's sessions.
func (a *App) commands() []command {
	var out []command
	p, hasProject := a.currentProject()
	if hasProject {
		out = append(out, command{"new", "New session", modKey + "N", icon.Plus, func() { a.openNewSession(p) }})
	}
	out = append(out, command{"sessions", "Sessions", "Page", icon.Terminal, func() {
		if a.goaOn() {
			a.confirmLeaveGOA(nil)
		}
	}})
	if a.client != nil {
		out = append(out,
			command{"notes", "Notes", "Page", icon.Notebook, func() { a.page = pageNotes }},
			command{"canvas", "Canvas", "Page", icon.Canvas, func() { a.page = pageCanvas }},
		)
	}
	out = append(out, command{"goa", "GOD of Agents", "Enter or leave", icon.Sparkle, a.toggleGOA})
	if a.client != nil {
		out = append(out, command{"team", "Team", "Page", icon.Users, a.openTeamPage})
	}
	out = append(out,
		command{"vault", "Credentials", "Page", icon.Key, a.openVault},
		command{"hosts", "Manage hosts", "Page", icon.Server, func() { a.page = pageHosts }},
		command{"usage", "Resource usage", "Memory and CPU", icon.Usage, func() { a.page = pageUsage }},
		command{"credits", "Credits", "What each agent account has left", icon.Credits, func() { a.page = pageCredits }},
		command{"appearance", "Appearance", "Theme and accent", icon.Palette, func() {
			a.page = pageAppearance
		}},
		command{"settings", "Settings", "Skills, MCP servers, instructions", icon.Settings, func() { a.page = pageSettings }},
	)
	if hasProject && a.client != nil && a.host.ID != hosts.LocalID {
		out = append(out, command{"setup", "Set up this host", "Toolchains, git, gh", icon.Server, a.openSetup})
	}
	if a.hostIsRemote() {
		out = append(out, command{"sync-env", "Sync environment", "Skills and settings between hosts", icon.Sync, a.openSync})
	}
	out = append(out,
		command{"add-host", "Add host", "A VPS or another device", icon.Server, a.openAddHost},
		command{"open", "Open folder", modKey + "O", icon.FolderPlus, a.openFolder},
		command{"clone", "Clone from Git", "", icon.Git, a.openClone},
		command{"goto-file", "Go to file…", modKey + "P", icon.File, a.openQuickOpen},
	)
	if hasProject && a.client != nil {
		out = append(out, command{"history", "Past conversations", p.Name, icon.Chat, func() { a.openHistory(p) }})
	}
	out = append(out, command{"rail", "Expand or collapse sidebar", modKey + "B", icon.Sidebar, func() { a.state.RailWide = !a.state.RailWide }})
	out = append(out, a.sessionCommands()...)
	out = append(out, a.runPaletteCommands()...)
	if hasProject && a.client != nil {
		if p.IsGit {
			out = append(out, command{"pipeline", "New pipeline…", p.Name + " · Plan, implement, review", icon.List, func() { a.openPipeline(p) }})
		}
		out = append(out,
			command{"copy-project", "Copy " + p.Name + " to another host…", "With its sessions", icon.Server, func() { a.openCopy(p) }},
			command{"links-project", "Sync & push " + p.Name + " on its own…", "Copies on other hosts, auto push", icon.ArrowUp, func() { a.openLinks(p) }},
		)
	}
	out = append(out,
		command{"reconnect", "Reconnect to " + a.host.Name, "And update its daemon if it is older", icon.Sync, func() { a.connect(a.host) }},
		command{"agent-limit", "Agents at once…", "On " + a.host.Name, icon.Gauge, a.askAgentLimit},
		command{"stop-agents", "Stop all agents", "On " + a.host.Name + " · Their sessions stay, to continue", icon.Stop, a.stopAgentsHere},
		command{"quit-stop", "Quit and stop all agents", "On this device", icon.Stop, a.quitAndStopAgents},
		command{"updates", "Check for updates", "enowX " + version.String, icon.ArrowUp, func() { a.checkForUpdates(true) }},
		command{"sessions-panel", "Show or hide the sidebar", modKey + "⇧S", icon.Sidebar, a.toggleSessionsPanel},
		command{"floating", "Floating control", floatingKey + " · Over any app", icon.Floating, a.toggleFloating},
	)
	for _, h := range a.allHosts() {
		if h.ID == a.host.ID {
			continue
		}
		out = append(out, command{"host:" + h.ID, "Connect to " + h.Name, hostDetail(h), hostIcon(h), func() { a.connect(h) }})
	}
	for _, s := range a.sessions {
		detail := filepath.Base(s.Project)
		if s.Branch != "" {
			detail += " · " + s.Branch
		}
		out = append(out, command{"session:" + s.ID, a.sessionName(s), detail, icon.Agent(s.Agent), func() {
			a.selectSession(s)
		}})
	}
	return out
}

// sessionCommands act on the selected session and the project's layout.
func (a *App) sessionCommands() []command {
	var out []command
	if len(a.projectSessions(a.project)) > 1 {
		label := "Show sessions side by side"
		if a.splitOn() {
			label = "Show one session at a time"
		}
		out = append(out,
			command{"split", label, modKey + "⇧L", icon.Split, func() { a.setSplit(!a.splitOn()) }},
			command{"next", "Next session", modKey + "⇧]", icon.ChevronRight, func() { a.stepSession(1) }},
			command{"previous", "Previous session", modKey + "⇧[", icon.ChevronRight, func() { a.stepSession(-1) }},
		)
	}
	s, ok := a.selectedSession()
	if !ok {
		return out
	}
	name := a.sessionName(s)
	for _, view := range a.sessionViews(s) {
		if view == a.currentView(s) {
			continue
		}
		label, svg := "Show the terminal", icon.Terminal
		if view == viewChat {
			label, svg = "Show as chat", icon.Chat
		}
		out = append(out, command{"view:" + view, label, name, svg, func() { a.showView(s, view) }})
	}
	if a.canReview(s) {
		label := "Show changes"
		if a.panelShows(panelChanges) {
			label = "Hide changes"
		}
		out = append(out, command{"changes", label, name + " · " + modKey + "⇧D", icon.Git, func() { a.toggleTool(panelChanges) }})
	}
	for _, t := range panelTools[1:] {
		label := "Show " + strings.ToLower(t.name)
		if a.panelShows(t.id) {
			label = "Hide " + strings.ToLower(t.name)
		}
		out = append(out, command{"tool:" + t.id, label, name, t.svg, func() { a.toggleTool(t.id) }})
	}
	maximize := "Maximize session"
	if a.maximized != "" {
		maximize = "Restore session"
	}
	out = append(out, command{"maximize", maximize, name + " · " + modKey + "⇧↩", icon.Maximize, func() { a.toggleMaximized(s.ID) }})
	if a.canReview(s) {
		for _, r := range []struct {
			action, label string
			svg           *ui.SVG
		}{
			{diffview.ActCommit, "Commit changes…", icon.Check},
			{diffview.ActPush, "Push branch", icon.ArrowUp},
			{diffview.ActPR, "Open a pull request…", icon.Globe},
			{diffview.ActMerge, "Merge into the base branch…", icon.Git},
			{diffview.ActDiscard, "Discard worktree…", icon.Trash},
		} {
			if r.action == diffview.ActDiscard && !s.Worktree {
				continue
			}
			out = append(out, command{"review:" + r.action, r.label, name, r.svg, func() { a.review(s, r.action) }})
		}
	}
	for _, ed := range editors {
		out = append(out, command{"editor:" + ed.id, "Open in " + ed.name, name, icon.Pencil, func() { a.openInEditor(ed, a.sessionFolder(s), 0) }})
	}
	if !s.Exited {
		out = append(out, command{"budget", "Set time budget…", name, icon.Gauge, func() { a.askBudget(s) }})
	}
	if s.Group != "" {
		out = append(out, command{"compare", "Compare with the others given its prompt", name, icon.Split, func() { a.openCompare(s.Group) }})
	}
	out = append(out, command{"delete", "Delete session…", name, icon.Trash, func() { a.askDelete(s) }})
	return out
}

// matchingCommands is the commands whose label or detail contain every
// word of the query.
func (a *App) matchingCommands() []command {
	words := strings.Fields(strings.ToLower(a.palette.query))
	var out []command
	for _, cmd := range a.commands() {
		text := strings.ToLower(cmd.label + " " + cmd.detail)
		ok := true
		for _, w := range words {
			if !strings.Contains(text, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, cmd)
			if len(out) == paletteLimit {
				break
			}
		}
	}
	return out
}

// commandPalette is the ⌘K dialog: type to filter, arrows to move, Enter
// to run.
func (a *App) commandPalette(c *ui.Context) {
	p := &a.palette
	if !p.open {
		return
	}
	prev := p.query
	cmds := a.matchingCommands()
	if len(cmds) > 0 {
		p.pick = (p.pick + len(cmds)) % len(cmds) // wraps around
	} else {
		p.pick = 0
	}
	var run func()
	// The input would move its caret with the arrows; the palette takes them
	// first to move the pick.
	submitted := widget.Palette(c, &p.open, &p.query, "Search sessions, hosts and actions", func(k ui.TextKey) bool {
		switch {
		case k.Key == ui.KeyDown && k.Mods == 0, k.Key == ui.KeyN && k.Mods == ui.Ctrl:
			p.pick++
		case k.Key == ui.KeyUp && k.Mods == 0, k.Key == ui.KeyP && k.Mods == ui.Ctrl:
			p.pick--
		default:
			return false
		}
		return true
	}, func() {
		if len(cmds) == 0 {
			ui.Text(c, "Nothing matches.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3)
		}
		for i, cmd := range cmds {
			row := ui.ButtonBase(c.Key(cmd.key)).Label(cmd.label).Transition(widget.RowMotion(i))
			widget.PaletteRow(c, row, i == p.pick, cmd.svg, cmd.label, cmd.detail, false)
			if row.Clicked() {
				run = cmd.run
			}
		}
	})
	if p.query != prev {
		p.pick = 0
	}
	if submitted && len(cmds) > 0 {
		run = cmds[p.pick].run
	}
	if run != nil {
		p.open = false
		a.page = ""
		run()
	}
}
