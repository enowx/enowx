package widget

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// RunStyle is how running text looks.
type RunStyle struct {
	Size   float32  // 0 is theme.TextUI
	Weight int      // 0 is 400
	Color  ui.Color // zero inherits
	Mono   bool
	// Always runs it without the pointer over it: for a title, the one
	// thing a frame says. Rows of a list run only under the pointer, as many
	// at once would never be still.
	Always bool
}

// Running is one line of text in the room its frame gives: the frame never
// grows for it. Text too long is cut with an ellipsis at rest and runs,
// back and forth, to show the rest. With less motion asked for it stays cut,
// and its tooltip says it whole.
func Running(c *ui.Context, text string, st RunStyle) ui.Element {
	size := st.Size
	if size == 0 {
		size = theme.TextUI
	}
	font := ""
	if st.Mono {
		font = theme.MonoFont
	}
	natural, _ := c.MeasureText(0, ui.Span{Text: text, Font: font, Size: size, Weight: st.Weight})
	box := ui.Row(c).MinWidth(0).Shrink(1).Clip()
	room := box.LayoutBounds().W
	over := natural - room
	style := func(t ui.Element) ui.Element {
		t.FontSize(size).SingleLine()
		if st.Weight != 0 {
			t.FontWeight(st.Weight)
		}
		if st.Color.A != 0 {
			t.TextColor(st.Color)
		}
		if st.Mono {
			t.Font(theme.MonoFont)
		}
		return t
	}
	box.Children(func() {
		if room <= 0 || over <= 1 || !(st.Always || box.Hovered()) {
			t := style(ui.Text(c, text)).Shrink(1).MinWidth(0)
			if room > 0 && over > 1 {
				t.Tooltip(text)
			}
			return
		}
		// A pause at each end; about 40 DIPs a second between them.
		travel := time.Duration(over/40*float32(time.Second)) + 2*runPause
		period := 2 * travel
		p := box.Loop("run", period, ui.Linear)
		style(ui.Text(c, text)).Shrink(0).Translate(-over*runAt(p, float32(runPause)/float32(travel)), 0)
	})
	return box
}

// runPause is how long running text rests at each end.
const runPause = 1200 * time.Millisecond

// runAt is how far running text has gone at p of its way there and back,
// from 0 to 1, resting for rest of each way at its end.
func runAt(p, rest float32) float32 {
	way := p * 2
	if way > 1 {
		way = 2 - way
	}
	moving := 1 - rest*2
	return min(max((way-rest)/moving, 0), 1)
}
