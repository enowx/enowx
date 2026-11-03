package ui

import (
	"fmt"
	"runtime"
	"slices"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/widget"
)

// Terminal text size: one for every terminal, kept between launches.
// ⌘+ and ⌘− (Ctrl on Windows and Linux), and the wheel with it held, zoom
// every terminal at once and say so, ⌘0 back to the default; Settings ›
// Terminal changes it too.

const (
	termSizeDefault = 13
	termSizeMin     = 8
	termSizeMax     = 32
)

// termSize is the terminals' text size.
func (a *App) termSize() float32 {
	if a.state.TermFontSize <= 0 {
		return termSizeDefault
	}
	return a.state.TermFontSize
}

// setTermSize changes every terminal's text size, 0 for the default.
func (a *App) setTermSize(size float32) {
	if size != 0 {
		size = min(max(size, termSizeMin), termSizeMax)
	}
	if size == termSizeDefault {
		size = 0
	}
	if a.state.TermFontSize == size {
		return
	}
	a.state.TermFontSize = size
	a.applyTermFont()
	a.saveState()
}

// zoomTerms is a terminal's OnZoom: every terminal zooms, not the one.
func (a *App) zoomTerms(z terminal.Zoom) bool {
	if z.Reset {
		a.setTermSize(0)
	} else {
		a.setTermSize(a.termSize() + float32(z.Steps))
	}
	a.notify(fmt.Sprintf("Terminal text %g · %s0 for the default", a.termSize(), zoomKey()))
	return true
}

func zoomKey() string {
	if runtime.GOOS == "darwin" {
		return "⌘"
	}
	return "Ctrl+"
}

// termSizeControl changes the terminals' text size, in Settings › Terminal:
// the sizes most use, as the whole range doesn't fit a row of choices. A size
// outside them (zoomed in a terminal, or saved earlier) joins them as long as
// it's the one in use, so that it shows and picking nothing keeps it.
func (a *App) termSizeControl(c *ui.Context) {
	sizes := []float32{11, 12, 13, 14, 15, 16}
	if cur := a.termSize(); !slices.Contains(sizes, cur) {
		sizes = append(sizes, cur)
		slices.Sort(sizes)
	}
	labels := make([]string, len(sizes))
	for i, s := range sizes {
		labels[i] = fmt.Sprintf("%g", s)
	}
	if i := widget.Segmented(c, labels, slices.Index(sizes, a.termSize()), nil); i >= 0 {
		a.setTermSize(sizes[i])
	}
}
