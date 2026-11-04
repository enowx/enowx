package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// Space is a space a page may show: a team's (Team its ID), or the
// user's own (Team "").
type Space struct {
	Team, Name string
}

// SpacePicker is the line naming the space a page shows, with detail
// after it, in text of size, that opens a list of spaces to switch to:
// pick gets the one chosen. Without other spaces to go to it only names
// the one shown.
func SpacePicker(c *ui.Context, current Space, detail string, size float32, spaces []Space, pick func(team string)) {
	label := current.Name
	if detail != "" {
		label += " · " + detail
	}
	mark := func(sp Space, size float32) {
		if sp.Team == "" {
			ui.Icon(c, icon.Account).FontSize(size).TextColor(theme.Muted)
			return
		}
		TeamMark(c, sp.Team, sp.Name, size)
	}
	if len(spaces) < 2 || pick == nil {
		ui.Row(c).Gap(theme.S1).AlignItems(ui.Center).MinWidth(0).Children(func() {
			mark(current, size+0.5)
			ui.Text(c, label).FontSize(size).TextColor(theme.Muted).SingleLine().Shrink(1)
		})
		return
	}
	choice := current.Team
	sel := ui.SelectBase(c, &choice)
	trig := sel.Trigger.Gap(theme.S1).Padding(1, theme.S1, 1, 2).Margin(0, 0, 0, -2).Radius(theme.RadiusSmall).AlignItems(ui.Center).MinWidth(0).Shrink(1).
		Cursor(ui.CursorPointer).Label("Space: " + current.Name).Tooltip("Switch to another space")
	if sel.Open() || trig.Hovered() {
		trig.Background(theme.Hover)
	}
	trig.Children(func() {
		mark(current, size+0.5)
		ui.Text(c, label).FontSize(size).TextColor(theme.TextSoft).SingleLine().Shrink(1)
		ui.Icon(c, icon.ChevronDown).FontSize(size - 0.5).TextColor(theme.Muted)
	})
	sel.Popup(func(panel ui.Element) {
		Popup(panel.AttachTo(trig, ui.AnchorTopLeft, ui.AnchorBottomLeft).Margin(theme.S1, 0, 0, 0).Width(220))
		FitScroll(c, PopupRoom(c, trig, 420), func() {
			for _, sp := range spaces {
				it := sel.Item(sp.Team)
				PopupItem(it, it.Highlighted()).TextColor(theme.TextSoft).Children(func() {
					ui.Box(c).Size(18, 18).Center().Shrink(0).Children(func() { mark(sp, 14) })
					ui.Text(c, sp.Name).TextColor(theme.Text).SingleLine().Grow(1).Shrink(1)
					if sp.Team == current.Team {
						ui.Icon(c, icon.Check).FontSize(12).TextColor(theme.Now)
					}
				})
			}
		})
	})
	if trig.Changed() && choice != current.Team {
		pick(choice)
	}
}
