package chatview

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The outline beside a wide conversation: the user's messages, the one
// read now lit, each a click away, so a long chat is crossed in a jump.

// outlineWidth is the outline's width.
const outlineWidth = 210

func (ch *Chat) outline(c *ui.Context, turns []turn, more int) {
	var rows []int // the list rows of the turns with a message of the user's
	for i, t := range turns {
		if t.prompt != nil {
			rows = append(rows, i+more)
		}
	}
	if len(rows) < 2 {
		return
	}
	first, _ := ch.list.Visible()
	current := rows[0]
	for _, r := range rows {
		if r <= first {
			current = r
		}
	}
	ch.outlineFollow()
	ui.Column(c.Key("outline")).Absolute().Right(theme.S4).Top(theme.S5 + theme.ControlHeight).Width(outlineWidth).MaxHeight(420).Gap(0).Children(func() {
		ui.Text(c, "Your messages").FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft).Padding(0, 0, theme.S1, 0)
		widget.Scroll(c.Key("outline-rows")).FillWidth().TrackScroll(&ch.outlineScroll).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, r := range rows {
					t := turns[r-more]
					on := r == current
					b := ui.ButtonBase(c.Key(r)).FillWidth().Padding(theme.S1, theme.S2).Justify(ui.Start).Cursor(ui.CursorPointer).
						BorderWidth(0, 0, 0, 2).Label("Go to: " + firstLine(t.prompt.Text))
					col, edge := theme.TextSoft, theme.BorderStrong
					switch {
					case on:
						col, edge = theme.Text, theme.Accent
					case b.Hovered():
						col = theme.TextSoft
					}
					b.BorderColor(edge).Children(func() {
						widget.Running(c, firstLine(t.prompt.Text), widget.RunStyle{Size: theme.TextLabel, Color: col})
					})
					if b.Clicked() {
						ch.list.ScrollTo(r, ui.Start)
					}
				}
			})
		})
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
