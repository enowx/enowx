package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The sessions as icons, in the folded sidebar: each project a tile of
// its initial, each session its agent's icon with its status, their names
// in tooltips. They keep the session list's clicks and menus.

// narrowProject is a project's sessions as icons.
func (a *App) narrowProject(c *ui.Context, p proto.ProjectInfo) {
	for _, s := range a.projectSessions(p.Path) {
		a.narrowSession(c, s)
	}
}

// projectInitials are a project's name in two letters: the first two
// words' initials, or its first two letters.
func projectInitials(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '-' || r == '_' || r == '.' })
	var out []rune
	if len(words) >= 2 {
		out = []rune{[]rune(words[0])[0], []rune(words[1])[0]}
	} else {
		r := []rune(name)
		if len(r) == 0 {
			return "?"
		}
		out = r[:min(2, len(r))]
	}
	return strings.ToUpper(string(out))
}

// narrowSession is a session's icon: its agent, its status's dot, its
// name and state in the tooltip.
func (a *App) narrowSession(c *ui.Context, s proto.SessionInfo) {
	st := statusOf(s)
	active := s.ID == a.selected
	tip := a.sessionName(s) + " · " + st.label
	if s.Branch != "" && s.Branch != a.sessionName(s) {
		tip += " · " + s.Branch
	}
	b := ui.ButtonBase(c.Key("ns:"+s.ID)).Size(40, 40).Shrink(0).Radius(theme.RadiusFull).Center().Cursor(ui.CursorPointer).Label(a.sessionName(s)).Tooltip(tip)
	switch {
	case active:
		b.Background(theme.Selected)
	case b.Hovered():
		b.Background(theme.Hover)
	}
	if a.picked[s.ID] && !active {
		b.Border(1, theme.Now)
	}
	b.TextColor(theme.SoftOn(active)).Children(func() {
		ui.Icon(c, icon.Agent(s.Agent)).FontSize(16)
		ui.Box(c).Size(8, 8).AttachTo(b, ui.AnchorBottomRight, ui.AnchorBottomRight).Margin(0, 2, 2, 0).Children(func() {
			widget.Dot(c, st.color, st.hollow, 8)
		})
		if a.unread[s.ID] {
			ui.Box(c).Size(8, 8).Radius(theme.RadiusFull).Background(theme.Now).AttachTo(b, ui.AnchorTopRight, ui.AnchorTopRight).Margin(2, 2, 0, 0)
		}
	})
	a.sessionMenu(c, b, s)
	if widget.Pressable(b).Clicked() {
		if b.ClickModifiers()&(ui.Cmd|ui.Shift) != 0 {
			a.pick(s)
		} else {
			a.picked = nil
			a.selectSession(s)
		}
	}
}
