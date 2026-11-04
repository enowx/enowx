package chatview

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// OwnChat says the chat is agent's main face: enowX's own agent, whose sessions open
// on it, which records its answers as they arrive and says what they cost.
func OwnChat(agent string) bool { return agent == record.AgentID }

// Facts are what the agent last said it runs on, for a page's words about it.
func (ch *Chat) Facts() (model, effort string, budget float64) {
	return ch.model, ch.effort, ch.budget
}

const (
	// costRoomLeast is the least the cost chip reads in, and costChipMost the most
	// it takes of the composer's row; a longer figure runs inside it.
	costRoomLeast = 52
	costChipMost  = 150
	// costNear is how much of the budget spent turns the chip amber.
	costNear = 0.8
	// costPopWidth is how wide what the chip opens is, in a window that leaves it.
	costPopWidth = 300
)

// money writes dollars as a cost reads: to the cent, and never as nothing.
func money(usd float64) string {
	if usd < 0.01 {
		return "<$0.01"
	}
	return fmt.Sprintf("$%.2f", usd)
}

// budgetMoney writes a budget, which is mostly a round figure: $10, $12.50.
func budgetMoney(usd float64) string {
	if usd == math.Trunc(usd) {
		return fmt.Sprintf("$%.0f", usd)
	}
	return fmt.Sprintf("$%.2f", usd)
}

// costLevel colors what was spent by how close it is to the budget.
func costLevel(cost, budget float64) ui.Color {
	switch {
	case budget <= 0 || cost < budget*costNear:
		return theme.TextSoft
	case cost < budget:
		return theme.Warning
	}
	return theme.Error
}

// costLabel is the chip's words: the budget left out when narrow.
func (ch *Chat) costLabel() string {
	if ch.budget > 0 && !ch.compact {
		return money(ch.cost) + " of " + budgetMoney(ch.budget)
	}
	return money(ch.cost)
}

// costChip is what the session has cost, beside the context, in the room the row
// leaves: it opens to what that means and how to change it.
func (ch *Chat) costChip(c *ui.Context, o Options, room float32) {
	const key = "cost"
	label := ch.costLabel()
	b := ui.ButtonBase(c.Key(key)).Height(24).PaddingX(theme.S2).Radius(theme.RadiusSmall).MinWidth(0).Shrink(1).MaxWidth(min(room, costChipMost)).
		AlignItems(ui.Center).Justify(ui.Start).Label("This session's cost: " + label).Cursor(ui.CursorPointer)
	bg := theme.Fade(theme.Selected, 0)
	if b.Hovered() || ch.menu == key {
		bg = theme.Selected
	}
	b.Background(widget.QuickTint(b, "bg", bg)).Children(func() {
		widget.Running(c, label, widget.RunStyle{Size: theme.TextLabel, Color: costLevel(ch.cost, ch.budget)})
	})
	if widget.Pressable(b).Clicked() {
		if ch.menu == key {
			ch.menu = ""
		} else {
			ch.menu = key
		}
	}
	if ch.menu != key {
		return
	}
	open := true
	ui.PopoverBase(c, b, &open, func(panel ui.Element) {
		width := float32(costPopWidth)
		if ch.width > 0 {
			width = min(width, ch.width-2*theme.S3)
		}
		widget.Popup(panel.AttachTo(b, ui.AnchorTopRight, ui.AnchorBottomRight).Margin(0, 0, theme.S1, 0).Width(width))
		ch.costDetails(c, o, &open)
	})
	if !open {
		ch.menu, ch.focus = "", true
	}
}

// costDetails says what the chip opens to: the spend, the budget, how to change it
// and where the rest of what the user wants to know is.
func (ch *Chat) costDetails(c *ui.Context, o Options, open *bool) {
	ui.Column(c).FillWidth().MinWidth(0).Padding(theme.S2, theme.S3).Gap(theme.S2).Children(func() {
		ui.Text(c, "This session's spend, at list prices").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		ui.Row(c).FillWidth().MinWidth(0).Children(func() {
			widget.Running(c, money(ch.cost), widget.RunStyle{Size: theme.TextTitle, Weight: 600, Color: theme.Text})
		})
		if ch.budget > 0 {
			col := costLevel(ch.cost, ch.budget)
			if col == theme.TextSoft {
				col = theme.Working
			}
			widget.Meter(c, widget.Segment{Fraction: ch.cost / ch.budget, Color: col})
			ui.Row(c).FillWidth().MinWidth(0).Children(func() {
				widget.Running(c.Key("budget"), "Budget: "+budgetMoney(ch.budget), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			ui.Text(c, "It asks you before going past it.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		} else {
			ui.Text(c, "No budget: it goes on until you stop it.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
		ui.Text(c, "Change it by sending /budget and an amount.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		ui.Text(c, "Remaining credit shows in the Claude Console: an ordinary API key can't read it.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		ui.Row(c).FillWidth().Gap(theme.S2).Wrap().Children(func() {
			if widget.Button(c.Key("write-budget"), widget.Secondary, nil, "Write /budget", "").Clicked() {
				if strings.TrimSpace(ch.input) == "" {
					ch.input, ch.caretEnd = "/budget ", true
				}
				*open = false
			}
			if o.OpenConsole != nil && widget.Button(c.Key("console"), widget.Secondary, icon.Globe, "Open the Claude Console", "").Clicked() {
				o.OpenConsole()
				*open = false
			}
		})
	})
}
