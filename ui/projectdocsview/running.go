package projectdocsview

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// runStyle is how running text looks.
type runStyle struct {
	Size   float32
	Weight int
	Color  ui.Color
}

// A pause at each end, how many times it goes there and back, and how far it
// goes in a second.
const (
	runPause = 1200 * time.Millisecond
	runTimes = 3
	runSpeed = 40
)

// running is one line of text in the room its frame gives: the frame never
// grows for it. Text too long is cut at rest and, under the pointer, runs back
// and forth a few times to show the rest; its tooltip says it whole.
//
// widget.Running replaces it once release/next is merged: same behaviour.
func running(c *ui.Context, text string, st runStyle) ui.Element {
	if st.Size == 0 {
		st.Size = 12.5
	}
	natural, _ := c.MeasureText(0, ui.Span{Text: text, Size: st.Size, Weight: st.Weight})
	box := ui.Row(c).MinWidth(0).Shrink(1).Clip()
	room := box.LayoutBounds().W
	over := natural - room
	style := func(t ui.Element) ui.Element {
		t.FontSize(st.Size).SingleLine()
		if st.Weight != 0 {
			t.FontWeight(st.Weight)
		}
		if st.Color.A != 0 {
			t.TextColor(st.Color)
		}
		return t
	}
	wants := room > 0 && over > 1 && box.Hovered()
	travel := time.Duration(over/runSpeed*float32(time.Second)) + 2*runPause
	period := 2 * travel
	// Climbs to 1 over its runs and then stands still, which stops the frames.
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

// runAt is how far running text has gone at p of its way there and back, from
// 0 to 1, resting for rest of each way at its end.
func runAt(p, rest float32) float32 {
	way := p * 2
	if way > 1 {
		way = 2 - way
	}
	moving := 1 - rest*2
	return min(max((way-rest)/moving, 0), 1)
}
