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
	// Always runs it without the pointer over it: for what is in front, as a
	// selected row or a frame's title. Others run only under the pointer, as
	// many at once would never be still.
	Always bool
}

// Running is one line of text in the room its frame gives: the frame never
// grows for it. Text too long is cut with an ellipsis at rest and runs, back
// and forth a few times, to show the rest, then rests again. Its tooltip
// says it whole.
func Running(c *ui.Context, text string, st RunStyle) ui.Element {
	text = LongLine(text) // measured each frame, and a row shows no more of it
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
	wants := room > 0 && over > 1 && (st.Always || box.Hovered())
	// A pause at each end; about 40 DIPs a second between them.
	travel := time.Duration(over/40*float32(time.Second)) + 2*runPause
	period := 2 * travel
	// How long it has wanted to run: Animate climbs to 1 over its runs, then
	// stands still, which stops the frames too.
	ran := float32(0)
	if wants {
		ran = box.AnimateWith("ran", 1, runTimes*period, ui.Linear)
	} else {
		box.AnimateWith("ran", 0, 0, ui.Linear)
	}
	box.Children(func() {
		if !wants || ran >= 1 {
			t := style(ui.Text(c, text)).Shrink(1).MinWidth(0)
			if room > 0 && over > 1 {
				t.Tooltip(text)
			}
			return
		}
		p := box.Loop("run", period, ui.Linear)
		style(ui.Text(c, text)).Shrink(0).Translate(-over*runAt(p, float32(runPause)/float32(travel)), 0)
	})
	return box
}

// runPause is how long running text rests at each end, and runTimes how many
// times it goes there and back.
const (
	runPause = 1200 * time.Millisecond
	runTimes = 3
)

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
