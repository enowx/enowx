package chatview

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The outline of the user's messages, the one read now lit, each a click away, so a
// long chat is crossed in a jump. It never lies over the conversation: beside a
// column with room at its side; in a side of its own taken from a column that runs
// the frame's width (GOD of Agents'); else folded to a button in the chat's corner,
// which opens it as a menu.

const (
	// outlineWidth is the outline's width, outlineMost its height at most.
	outlineWidth = 210
	outlineMost  = 420
)

// outlineTop is where the outline starts, under the corner's tools.
func outlineTop() float32 { return theme.S5 + theme.ControlHeight }

// outlinePlace is where the outline goes.
type outlinePlace int

const (
	outlineFolded outlinePlace = iota // a button
	outlineBeside                     // at the side of a centered column
	outlineOwn                        // at the side the column gave up
)

// outlineSide is what the outline takes from the conversation's width, with the
// margin to the frame and the one to the column.
func outlineSide() float32 { return outlineWidth + theme.S4 + theme.S3 }

// outlineFit is where the outline goes in a conversation width wide, whose column
// runs column at most and reads well from read.
func outlineFit(width, column, read float32) outlinePlace {
	side := outlineSide()
	switch {
	case width >= column+2*side:
		return outlineBeside
	case column+2*theme.S5 >= width && width-side-theme.S5 >= read:
		return outlineOwn
	}
	return outlineFolded
}

// outlineRowsOf are the list rows of the turns with a message of the user's, and the
// one read now; the outline is for two or more.
func (ch *Chat) outlineRowsOf(turns []turn, more int) (rows []int, current int) {
	for i, t := range turns {
		if t.prompt != nil {
			rows = append(rows, i+more)
		}
	}
	if len(rows) == 0 {
		return nil, 0
	}
	first, _ := ch.list.Visible()
	current = rows[0]
	for _, r := range rows {
		if r <= first {
			current = r
		}
	}
	return rows, current
}

// outlineTall is the height the outline may take: what's free between the corner's
// tools and the composer floating over the conversation's foot.
func (ch *Chat) outlineTall() float32 {
	if ch.height <= 0 {
		return outlineMost
	}
	return max(min(outlineMost, ch.height-outlineTop()-ch.composerH-2*theme.S4), 0)
}

// outline is the outline at the conversation's side, unless folded.
func (ch *Chat) outline(c *ui.Context, turns []turn, more int) {
	rows, current := ch.outlineRowsOf(turns, more)
	tall := ch.outlineTall()
	if len(rows) < 2 || tall < 2*theme.ControlHeight {
		return
	}
	ch.outlineFollow()
	ui.Column(c.Key("outline")).Absolute().Right(theme.S4).Top(outlineTop()).Width(outlineWidth).MaxHeight(tall).Gap(0).Children(func() {
		ch.outlineList(c, turns, more, rows, current)
	})
}

// outlineList is the outline's title and rows.
func (ch *Chat) outlineList(c *ui.Context, turns []turn, more int, rows []int, current int) {
	ui.Text(c, "Your messages").FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft).Padding(0, 0, theme.S1, 0)
	widget.Scroll(c.Key("outline-rows")).FillWidth().MinHeight(0).Shrink(1).TrackScroll(&ch.outlineScroll).Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			for _, r := range rows {
				t := turns[r-more]
				on := r == current
				b := ui.ButtonBase(c.Key(r)).FillWidth().Padding(theme.S1, theme.S2).Justify(ui.Start).Cursor(ui.CursorPointer).
					BorderWidth(0, 0, 0, 2).Label("Go to: " + firstLine(t.prompt.Text))
				col, edge := theme.TextSoft, theme.BorderStrong
				if on {
					col, edge = theme.Text, theme.Accent
				}
				b.BorderColor(edge).Children(func() {
					widget.Running(c, firstLine(t.prompt.Text), widget.RunStyle{Size: theme.TextLabel, Color: col})
				})
				if b.Clicked() {
					ch.list.ScrollTo(r, ui.Start)
					ch.outlineOpen = false
				}
			}
		})
	})
}

// outlineButton is the outline folded: a button of the chat's corner opening it.
func (ch *Chat) outlineButton(c *ui.Context, turns []turn, more int) {
	rows, current := ch.outlineRowsOf(turns, more)
	if len(rows) < 2 {
		ch.outlineOpen = false
		return
	}
	b := widget.IconButton(c.Key("outline-open"), icon.List, "Your messages", ch.outlineOpen)
	if widget.AnchorNew(b) {
		ch.outlineOpen = false
	}
	if b.Clicked() {
		ch.outlineOpen = !ch.outlineOpen
	}
	if !ch.outlineOpen {
		return
	}
	ch.outlineFollow()
	ui.PopoverBase(c, b, &ch.outlineOpen, func(panel ui.Element) {
		widget.Popup(panel.AttachTo(b, ui.AnchorBottomRight, ui.AnchorTopRight).Margin(theme.S1, 0, 0, 0).
			Width(outlineWidth + 2*theme.S2).MaxHeight(max(ch.outlineTall(), 3*theme.ControlHeight)).Padding(theme.S2))
		ch.outlineList(c, turns, more, rows, current)
	})
}

// outlineFollow keeps the outline at its newest messages, as it was left
// unless the user scrolled it up, until they scroll back to its end. The row
// lit for what is read in the conversation doesn't move it.
func (ch *Chat) outlineFollow() {
	st := &ch.outlineScroll
	const near = 1 // a DIP's rounding is still the end
	if st.MaxY > 0 {
		ch.outlineLeft = st.Y < st.MaxY-near
	}
	if !ch.outlineLeft {
		st.Y = math.MaxFloat32
	}
}

// firstLine is a message's first line, cut short for the outline.
func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			s = s[:i]
			break
		}
	}
	if r := []rune(s); len(r) > 48 {
		return string(r[:48]) + "…"
	}
	return s
}
