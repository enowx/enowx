package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

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

// tabStrip is the row of session s's tabs, its own first, with the button
// that opens another.
func (a *App) tabStrip(c *ui.Context, s proto.SessionInfo) {
	front := a.frontTab(s).ID
	ui.Row(c).Height(36).Shrink(0).FillWidth().PaddingX(theme.S2).Gap(theme.S2).Background(theme.Sunken).
		BorderWidth(0, 0, 0.5, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).MinWidth(0).Gap(theme.S1).Clip().Children(func() {
			a.tabButton(c.Key("tab:"+s.ID), s, s, front == s.ID, false)
			for _, t := range a.tabsOf(s.ID) {
				a.tabButton(c.Key("tab:"+t.ID), s, t, front == t.ID, true)
			}
		})
		add := smallIcon(c, icon.Plus, "New tab in this session's folder")
		a.menu.OpenBelow(add, "tab:"+s.ID, 24)
		a.menu.Show(c, add, "tab:"+s.ID, func(done func()) {
			a.shellTabItems(c, s, done)
			for _, ag := range a.agents {
				if ag.Command == "" || !ag.Installed {
					continue
				}
				if widget.MenuItem(c, icon.Agent(ag.ID), ag.Name, false) {
					a.openTab(s, ag.ID, "")
					done()
				}
			}
		})
		a.runButton(c, s)
		ui.Spacer(c)
	})
}

// tabButton is one tab of parent: its program, and an × that closes it
// for tabs other than the session's own.
func (a *App) tabButton(c *ui.Context, parent, t proto.SessionInfo, front, closable bool) {
	name := a.agentName(t.Agent)
	if closable && t.Name != "" {
		name = t.Name // a command run in it
	}
	b := ui.ButtonBase(c).Height(28).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).Shrink(1).MinWidth(64).
		Label(name).Tooltip(a.sessionName(t)).Cursor(ui.CursorPointer)
	bg, fg := theme.Fade(theme.Selected, 0), theme.TextSoft
	switch {
	case front:
		bg, fg = theme.Selected, theme.Text
	case b.Hovered():
		bg, fg = theme.Hover, theme.Text
	}
	b.Background(widget.QuickTint(b, "bg", bg)).TextColor(widget.QuickTint(b, "fg", fg)).Transition(widget.QuickMotion)
	st := statusOf(t)
	closed := false
	b.Children(func() {
		widget.Dot(c, st.color, st.hollow, 8)
		ui.Icon(c, icon.Agent(t.Agent)).FontSize(12)
		widget.Running(c, name, widget.RunStyle{})
		if !closable {
			return
		}
		x := ui.ButtonBase(c).Size(16, 16).Shrink(0).Radius(theme.RadiusFull).Center().Cursor(ui.CursorPointer).Label("Close " + name)
		if x.Hovered() {
			x.Background(theme.Hover)
		}
		x.Children(func() { ui.Icon(c, icon.Close).FontSize(10).TextColor(theme.Muted) })
		closed = x.Clicked()
	})
	switch {
	case closed:
		a.closeTab(parent, t)
	case b.Clicked():
		a.showTab(parent.ID, t.ID)
	}
}

// openTab starts agent id (agent.Shell for a terminal, shell its shell or
// "" for the host's default) as a tab of s.
func (a *App) openTab(s proto.SessionInfo, id, shell string) {
	cl := a.client
	if cl == nil {
		return
	}
	cols, rows := 0, 0
	if p := a.panes[s.ID]; p != nil {
		cols, rows = p.term.Size()
	}
	params := proto.SessionCreateParams{Parent: s.ID, Project: s.Project, Agent: id, BranchMode: proto.BranchNone,
		SkipPermissions: s.SkipPermissions, Shell: shell, Cols: cols, Rows: rows}
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
