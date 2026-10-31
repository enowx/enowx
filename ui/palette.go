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

// commands are what the palette offers: actions, then hosts, then the
// host's sessions.
func (a *App) commands() []command {
	var out []command
	if p, ok := a.currentProject(); ok {
		out = append(out, command{"new", "New session", p.Name + " · " + modKey + "N", icon.Plus, func() { a.openNewSession(p) }})
		if a.client != nil && a.host.ID != hosts.LocalID {
			out = append(out, command{"setup", "Set up " + a.host.Name + " like this device", "toolchains, git, gh, agents", icon.Server, a.openSetup})
		}
		if a.client != nil {
			out = append(out, command{"history", "Past conversations…", p.Name + " · resume one", icon.Chat, func() { a.openHistory(p) }})
		}
		if a.client != nil && p.IsGit {
			out = append(out, command{"pipeline", "New pipeline…", p.Name + " · plan, implement, review", icon.List, func() { a.openPipeline(p) }})
		}
		if a.client != nil {
			out = append(out, command{"copy-project", "Copy " + p.Name + " to another host…", "with its sessions", icon.Server, func() { a.openCopy(p) }})
			out = append(out, command{"links-project", "Sync & push " + p.Name + " on its own…", "copies on other hosts, auto push", icon.ArrowUp, func() { a.openLinks(p) }})
		}
	}
	out = append(out, a.sessionCommands()...)
	out = append(out, a.runPaletteCommands()...)
	out = append(out,
		command{"open", "Open folder", modKey + "O", icon.FolderPlus, a.openFolder},
		command{"clone", "Clone from Git", "", icon.Git, a.openClone},
		command{"goto-file", "Go to file…", modKey + "P", icon.File, a.openQuickOpen},
		command{"add-host", "Add host", "a VPS or another device", icon.Server, a.openAddHost},
		command{"hosts", "Manage hosts", "", icon.Server, func() { a.page = pageHosts }},
		command{"credits", "Credits", "what each agent account has left", icon.Credits, func() { a.page = pageCredits }},
		command{"usage", "Resource usage", "memory and CPU", icon.Usage, func() { a.page = pageUsage }},
		command{"appearance", "Appearance", "theme and accent", icon.Palette, func() {
			a.accentInput = a.state.Accent
			a.page = pageAppearance
		}},
		command{"reconnect", "Reconnect to " + a.host.Name, "and update its daemon if it is older", icon.Sync, func() { a.connect(a.host) }},
		command{"agent-limit", "Agents at once…", "on " + a.host.Name, icon.Gauge, a.askAgentLimit},
		command{"stop-agents", "Stop all agents", "on " + a.host.Name + " · their sessions stay, to continue", icon.Stop, a.stopAgentsHere},
		command{"quit-stop", "Quit and stop all agents", "on this device", icon.Stop, a.quitAndStopAgents},
		command{"updates", "Check for updates", "enowX " + version.String, icon.ArrowUp, func() { a.checkForUpdates(true) }},
		command{"settings", "Settings: skills, MCP servers, instructions", "the agents on " + a.host.Name, icon.Settings, func() { a.page = pageSettings }},
		command{"rail", "Expand or collapse sidebar", modKey + "B", icon.Sidebar, func() { a.state.RailWide = !a.state.RailWide }},
		command{"sessions-panel", "Show or hide the sidebar", modKey + "⇧S", icon.Sidebar, a.toggleSessionsPanel},
		command{"floating", "Floating control", floatingKey + " · over any app", icon.Floating, a.toggleFloating},
		command{"goa", "Enter or leave GOD of Agents", "one session controlling every session, on " + a.host.Name, icon.Sparkle, a.toggleGOA},
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
	cmds := a.matchingCommands()
	var run func()
	widget.Dialog(c, &p.open, 560, 8, func() {
		prev := p.query
		in := widget.Input(c, &p.query, "Search sessions, hosts and actions").FillWidth().AutoFocus()
		if p.query != prev {
			p.pick = 0
		}
		// The input would move its caret with the arrows; the palette takes
		// them first to move the pick.
		in.OnTextKey(func(k ui.TextKey) bool {
			switch {
			case k.Key == ui.KeyDown && k.Mods == 0, k.Key == ui.KeyN && k.Mods == ui.Ctrl:
				p.pick++
			case k.Key == ui.KeyUp && k.Mods == 0, k.Key == ui.KeyP && k.Mods == ui.Ctrl:
				p.pick--
			default:
				return false
			}
			return true
		})
		if len(cmds) > 0 {
			p.pick = (p.pick + len(cmds)) % len(cmds) // wraps around
		} else {
			p.pick = 0
		}
		if in.Submitted() && len(cmds) > 0 {
			run = cmds[p.pick].run
		}
		if len(cmds) == 0 {
			ui.Text(c, "Nothing matches.").FontSize(12).TextColor(theme.Muted).Padding(8)
		}
		ui.Column(c).FillWidth().Gap(1).Children(func() {
			for i, cmd := range cmds {
				row := ui.ButtonBase(c.Key(cmd.key)).Label(cmd.label).Transition(widget.RowMotion(i))
				widget.ListRow(c, row, i == p.pick, "palette", func() {
					ui.Icon(c, cmd.svg).FontSize(14).TextColor(theme.TextSoft)
					ui.Text(c, cmd.label).FontSize(12.5).SingleLine().Shrink(1)
					ui.Spacer(c)
					ui.Text(c, cmd.detail).FontSize(11).TextColor(theme.Muted).SingleLine().Shrink(1)
				})
				if row.Clicked() {
					run = cmd.run
				}
			}
		})
	})
	if run != nil {
		p.open = false
		a.page = ""
		run()
	}
}
