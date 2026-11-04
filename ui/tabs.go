package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A session's tabs are more terminals in its folder: a shell or another
// agent beside the first, on the same branch. Each is a session of the
// daemon (SessionInfo.Parent) that the session list leaves out; deleting
// the session deletes them.

// tabsOf returns the tabs of session id, oldest first.
func (a *App) tabsOf(id string) []proto.SessionInfo {
	var out []proto.SessionInfo
	for _, s := range a.sessions {
		if s.Parent == id {
			out = append(out, s)
		}
	}
	return out
}

// frontTab is what session s shows: the tab picked last, else s itself.
func (a *App) frontTab(s proto.SessionInfo) proto.SessionInfo {
	id := a.tabs[s.ID]
	for _, t := range a.sessions {
		if id != "" && t.ID == id && t.Parent == s.ID {
			return t
		}
	}
	return s
}

// parentOf is the session tab s belongs to, s itself for a session of its
// own.
func (a *App) parentOf(s proto.SessionInfo) proto.SessionInfo {
	if s.Parent == "" {
		return s
	}
	for _, p := range a.sessions {
		if p.ID == s.Parent {
			return p
		}
	}
	return s
}

// showTab brings tab id of session parent to the front.
func (a *App) showTab(parent, id string) {
	if a.tabs == nil {
		a.tabs = map[string]string{}
	}
	if id == parent {
		delete(a.tabs, parent)
		return
	}
	a.tabs[parent] = id
}

// tabStrip is the row of session s's tabs: its own first, the terminals and
// agents opened beside it, then the viewers tools opened, with the buttons that
// open another. Every tab is one width; those that don't fit scroll.
func (a *App) tabStrip(c *ui.Context, s proto.SessionInfo) {
	a.pruneGOATabs()
	viewer := a.centerFront(s)
	front := a.frontTab(s).ID
	ui.Row(c.Key("tab-strip")).Height(36).Shrink(0).FillWidth().PaddingX(theme.S2).Gap(theme.S1).Background(theme.Sunken).
		BorderWidth(0, 0, theme.Rule, 0).BorderColor(theme.Border).Children(func() {
		ui.ScrollHorizontal(c.Key("tabs")).Grow(1).MinWidth(0).Height(36).Children(func() {
			ui.Row(c).Gap(theme.S1).Height(36).Children(func() {
				a.tabButton(c.Key("tab:"+s.ID), s, s, viewer == "" && front == s.ID, false)
				for _, t := range a.tabsOf(s.ID) {
					a.tabButton(c.Key("tab:"+t.ID), s, t, viewer == "" && front == t.ID, true)
				}
				for _, tab := range a.centerOf(s.ID).tabs {
					a.centerTabButton(c, s, tab, viewer == tab)
				}
			})
		})
		add := widget.IconButton(c, icon.Plus, "New tab in this session's folder", false)
		a.menu.OpenBelow(add, "tab:"+s.ID, theme.ControlHeight)
		a.menu.Show(c, add, "tab:"+s.ID, func(done func()) {
			a.shellTabItems(c, s, done)
			for _, ag := range a.agents {
				if ag.Command == "" || !ag.Installed {
					continue
				}
				a.agentTabItems(c.Key("tab-agent:"+ag.ID), s, ag, done)
			}
		})
		a.runButton(c, s)
	})
}

// tabButton is one tab of parent: its program, and an × that closes it
// for tabs other than the session's own.
func (a *App) tabButton(c *ui.Context, parent, t proto.SessionInfo, front, closable bool) {
	name := a.agentName(t.Agent)
	if closable && t.Name != "" {
		name = t.Name // a command run in it
	}
	st := statusOf(t)
	svg := icon.Agent(t.Agent)
	if t.GOA {
		name, svg = "GOD of Agents", icon.Sparkle
	}
	a.tabFace(c, name, a.sessionName(t), front, closable, st.color, st.hollow, svg,
		func() {
			a.centerOf(parent.ID).front = ""
			a.showTab(parent.ID, t.ID)
		},
		func() { a.closeTab(parent, t) })
}

// openTab starts agent id (agent.Shell for a terminal, shell its shell or
// "" for the host's default) as a tab of s.
func (a *App) openTab(s proto.SessionInfo, id, shell string) {
	a.openTabWith(s, id, shell, "")
}

// tabConfig is a config the + menu offers for an agent.
type tabConfig struct {
	account, label string
}

// tabConfigsOf are what an agent's entry of the + menu opens to, as New session offers
// them: Default and each config, the one the preferences name marked. None for an agent
// with no other config, which stays one click.
func tabConfigsOf(list []proto.Account, preferred string) []tabConfig {
	if len(list) == 0 {
		return nil
	}
	mark := func(account, name string) string {
		if account == preferred {
			return name + " (preferred)"
		}
		return name
	}
	out := []tabConfig{{"", mark("", "Default")}}
	for _, acc := range list {
		out = append(out, tabConfig{acc.ID, mark(acc.ID, acc.Name)})
	}
	return out
}

// agentTabItems is agent ag in the + menu: one click with no other config; with some,
// it opens in place to pick one, as a menu here has no submenus.
func (a *App) agentTabItems(c *ui.Context, s proto.SessionInfo, ag proto.AgentInfo, done func()) {
	configs := tabConfigsOf(a.accountsOf().accountsOfAgent(ag.ID), a.defaultAccountFor(ag.ID, s.Project))
	if len(configs) == 0 {
		if widget.MenuItem(c, icon.Agent(ag.ID), ag.Name, false) {
			a.openTab(s, ag.ID, "")
			done()
		}
		return
	}
	open := a.tabMenuAgent == ag.ID
	chevron := icon.ChevronRight
	if open {
		chevron = icon.ChevronDown
	}
	if widget.MenuItemTrailing(c, icon.Agent(ag.ID), ag.Name, chevron) {
		a.tabMenuAgent = ag.ID
		if open {
			a.tabMenuAgent = ""
		}
	}
	if !open {
		return
	}
	for _, cfg := range configs {
		if widget.MenuItem(c.Key("config:"+cfg.account), nil, "    "+cfg.label, false) {
			a.openTabWith(s, ag.ID, "", cfg.account)
			a.tabMenuAgent = ""
			done()
		}
	}
}

// openTabWith is openTab with the agent's config, "" for Default.
func (a *App) openTabWith(s proto.SessionInfo, id, shell, account string) {
	cl := a.client
	if cl == nil {
		return
	}
	cols, rows := 0, 0
	if p := a.panes[s.ID]; p != nil {
		cols, rows = p.term.Size()
	}
	// A Claude Code session always skips: that says nothing of the agent in this tab.
	skip := s.SkipPermissions
	if agent.AlwaysSkips(s.Agent) {
		skip = a.skipPermissions
	}
	params := proto.SessionCreateParams{Parent: s.ID, Project: s.Project, Agent: id, BranchMode: proto.BranchNone,
		SkipPermissions: skip, Shell: shell, Account: account, Cols: cols, Rows: rows}
	go func() {
		info, err := cl.CreateSession(params)
		a.onMain(func() {
			if err != nil {
				a.status = strings.TrimPrefix(err.Error(), proto.MethodSessionCreate+": ")
				return
			}
			if info.Parent != s.ID {
				// An older daemon made a session of its own.
				a.upsert(info)
				a.selectSession(info)
				return
			}
			a.upsert(info)
			a.showTab(s.ID, info.ID)
		})
	}()
}

// closeFrontTab closes what is in front of the selected session, as Cmd+W: a
// viewer, or a tab of its own, asking first if its program still runs. Nothing
// else closes: the session's own tab has no ×, and with no session in front
// (killed from elsewhere, or a page) there is no tab, which is fine.
func (a *App) closeFrontTab() {
	if a.page != "" || a.confirming != nil {
		return
	}
	s, ok := a.selectedSession()
	if !ok {
		return
	}
	s = a.parentOf(s)
	if front := a.centerFront(s); front != "" {
		a.closeCenter(s.ID, front)
		return
	}
	t := a.frontTab(s)
	switch {
	case t.ID == s.ID:
	case t.Exited:
		a.closeTab(s, t)
	default:
		a.confirm("Close this tab?", "“"+a.sessionName(t)+"” is still running, and stops with it.", "Close", func() { a.closeTab(s, t) })
	}
}

// closeTab deletes tab t of parent, showing the tab before it.
func (a *App) closeTab(parent, t proto.SessionInfo) {
	if a.tabs[parent.ID] == t.ID {
		tabs := a.tabsOf(parent.ID)
		prev := parent.ID
		if i := slices.IndexFunc(tabs, func(o proto.SessionInfo) bool { return o.ID == t.ID }); i > 0 {
			prev = tabs[i-1].ID
		}
		a.showTab(parent.ID, prev)
	}
	a.killSession(t.ID)
}
