package widget

import (
	"time"

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

// FitScroll is a popup's list, at most room high. One that fits is a plain column,
// as it would be without this; a scroller keeps a gutter for its bar, which a
// short menu has no use for. One that doesn't fit scrolls from its second frame,
// once its height is known.
func FitScroll(c *ui.Context, room float32, items func()) {
	list := ui.Column(c.Key("popup-list")).FillWidth()
	tall := ui.Local(list, "tall", func() bool { return false })
	h := list.Bounds().H
	if h == 0 {
		c.After(time.Millisecond) // measured in this frame: read it in the next
	}
	if h > room {
		*tall = true
	}
	list.Children(func() {
		if !*tall {
			items()
			return
		}
		Scroll(c).FillWidth().MaxHeight(room).Children(func() {
			ui.Column(c).FillWidth().Children(items)
		})
	})
}

// AnchorNew says that anchor wasn't in the last frame: it just appeared, or came
// back. A pop-up open by then was left open by a user who has moved on, and
// shouldn't open by itself: call it every frame anchor is built, as it knows by
// the state that goes with the element. It is true in anchor's first frame too.
func AnchorNew(anchor ui.Element) bool {
	fresh := false
	ui.Local(anchor, "popup-anchor", func() struct{} {
		fresh = true
		return struct{}{}
	})
	return fresh
}
