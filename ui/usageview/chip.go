package usageview

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// ChipActions are what the chip's popover leads to.
type ChipActions struct {
	OpenPage   func() // the Resource usage page
	StopAgents func() // stops the host's agents; nil hides it
	Select     func(id string)
}

// Open reports whether the chip's popover is open, when the numbers are
// worth measuring often.
func (u *Meter) Open() bool { return u.open }

// Chip is the title bar's memory button: how full the connected host's
// memory is and what enowX takes of it. It opens a popover with the
// biggest sessions.
func (u *Meter) Chip(c *ui.Context, h Host, act ChipActions) {
	if !u.Measured() {
		return
	}
	host, ok := u.Host()
	total := u.total(h.Local)
	used := ratio(float64(host.MemUsed), float64(host.MemTotal))
	b := ui.ButtonBase(c.Key("memory-chip")).Height(26).PaddingX(9).Gap(6).Radius(theme.RadiusControl).Shrink(0).
		Background(theme.Sunken).Label("Memory").Tooltip("Memory on " + h.Name).Cursor(ui.CursorPointer).TextColor(theme.TextSoft)
	border := theme.BorderStrong
	if b.Hovered() || u.open {
		border = theme.AccentLine
	}
	b.Border(1, widget.Tint(b, "border", border)).Children(func() {
		if ok {
			widget.Dot(c, widget.Level(used), false, 6)
		}
		ui.Text(c, "Memory").FontSize(theme.TextLabel).SingleLine()
		if ok {
			ui.Text(c, fmt.Sprintf("%.0f%%", used*100)).FontSize(theme.TextLabel).TextColor(levelText(used)).SingleLine()
			ui.Text(c, "·").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		ui.Text(c, "enowX "+textfmt.Bytes(total.RSS)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
	})
	if b.Clicked() {
		u.open = !u.open
		u.Poke()
	}
	if !u.open {
		return
	}
	ui.PopoverBase(c, b, &u.open, func(panel ui.Element) {
		widget.Popup(panel.AttachTo(b, ui.AnchorBottomRight, ui.AnchorTopRight).Margin(6, 0, 0, 0).Width(340).Padding(10).Gap(8))
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, "Memory on "+h.Name).FontSize(theme.TextUI).Grow(1)
			if ok {
				ui.Text(c, fmt.Sprintf("%s of %s", textfmt.Bytes(host.MemUsed), textfmt.Bytes(host.MemTotal))).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
			}
		})
		if ok {
			ours := min(float64(total.RSS), float64(host.MemUsed))
			share := ratio(ours, float64(host.MemTotal))
			widget.Meter(c, widget.Segment{Fraction: share, Color: theme.Accent}, widget.Segment{Fraction: used - share, Color: theme.Idle})
			ui.Row(c).FillWidth().Gap(12).Children(func() {
				widget.Legend(c, theme.Accent, "enowX "+textfmt.Bytes(uint64(ours)))
				widget.Legend(c, theme.Idle, "Other apps "+textfmt.Bytes(host.MemUsed-uint64(ours)))
				widget.Legend(c, theme.BorderStrong, "Free "+textfmt.Bytes(host.MemTotal-min(host.MemUsed, host.MemTotal)))
			})
		}
		widget.Divider(c)
		sessions := slices.Clone(u.host.Sessions)
		slices.SortFunc(sessions, func(x, y proto.SessionUsage) int {
			switch {
			case x.RSS > y.RSS:
				return -1
			case x.RSS < y.RSS:
				return 1
			}
			return 0
		})
		if len(sessions) == 0 {
			ui.Text(c, "No session is running.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		for i, su := range sessions {
			if i == 6 {
				ui.Text(c, fmt.Sprintf("and %d more", len(sessions)-6)).FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(0, 6)
				break
			}
			name, agentID, project := h.Session(su.ID)
			r := ui.ButtonBase(c.Key(su.ID)).FillWidth().Padding(5, 6).Gap(8).Radius(theme.RadiusControl).Cursor(ui.CursorPointer).Label(name)
			if r.Hovered() {
				r.Background(theme.Hover)
			}
			r.Children(func() {
				ui.Icon(c, icon.Agent(agentID)).FontSize(12).TextColor(theme.TextSoft)
				ui.Text(c, name).FontSize(theme.TextLabel).SingleLine().Shrink(1)
				if project != "" {
					ui.Text(c, project).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
				}
				ui.Spacer(c)
				ui.Text(c, textfmt.CPU(su.CPU)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(0)
				ui.Text(c, textfmt.Bytes(su.RSS)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Width(64).TextAlign(ui.End).SingleLine().Shrink(0)
			})
			if r.Clicked() && act.Select != nil {
				u.open = false
				act.Select(su.ID)
			}
		}
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if act.StopAgents != nil && len(sessions) > 0 && widget.GhostButton(c, "Stop all agents").Clicked() {
				u.open = false
				act.StopAgents()
			}
			ui.Spacer(c)
			if act.OpenPage != nil && widget.GhostButton(c, "Resource usage").Clicked() {
				u.open = false
				act.OpenPage()
			}
		})
	})
}

// levelText colors a fill's percentage: plain until it gets high.
func levelText(fill float64) ui.Color {
	if fill < 0.75 {
		return theme.Text
	}
	return widget.Level(fill)
}
