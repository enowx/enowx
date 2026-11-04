package widget

import (
	"github.com/egoist/mygo/ui"
)

const (
	// popupChrome is what a panel takes besides its list: the window's margin
	// and the space from its anchor, its own padding, and some air.
	popupChrome = 24
	// popupLeast is the least a list shows, however little room there is.
	popupLeast = 3 * 32
	// popupAny asks PopupRoom for all the room there is.
	popupAny = 1 << 20
)

// PopupRoom is how high a list in a panel over or under anchor may be, want at
// most. MyGo puts such a panel on the side of anchor where it overflows less
// but, unlike one placed along its anchor, doesn't move it into the window, so
// a list that fits neither side runs past the window's edge.
func PopupRoom(c *ui.Context, anchor ui.Element, want float32) float32 {
	_, h := c.Size()
	b := anchor.Bounds()
	return max(min(want, max(b.Y, h-b.Y-b.H)-popupChrome), popupLeast)
}

// FitScroll is a popup's list, at most room high: it scrolls what doesn't fit,
// keeping room for its bar only then, so that a list that fits looks as it
// would without it.
func FitScroll(c *ui.Context, room float32, items func()) {
	s := ui.Scroll(c.Key("popup-list")).FillWidth().MaxHeight(room)
	if s.Bounds().H >= room {
		s.Padding(0, ScrollGutter, 0, 0)
	}
	s.Children(func() { ui.Column(c).FillWidth().Children(items) })
}
