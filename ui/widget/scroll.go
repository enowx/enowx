package widget

import (
	"github.com/egoist/mygo/ui"
)

// ScrollGutter is the room kept along a scroller's right edge for its bar,
// which MyGo draws over the content, 3 to 11 DIPs in from the edge (at the
// theme's ScrollbarWidth of 8). It is kept whether or not the bar shows, so
// nothing shifts when it does.
const ScrollGutter = 12

// Scroll is a column that scrolls, its content ending before the bar's gutter.
// A scroller whose content has at least the gutter as its own right padding
// can stay a ui.Scroll: this would only push it further from the edge.
func Scroll(c *ui.Context) ui.Element {
	return ui.Scroll(c).Padding(0, ScrollGutter, 0, 0)
}

// List is a ui.List whose rows end before the bar's gutter.
func List(c *ui.Context, s *ui.ListState, n int, row ...func(int)) ui.Element {
	return ui.List(c, s, n, row...).Padding(0, ScrollGutter, 0, 0)
}
