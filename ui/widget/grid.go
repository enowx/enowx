package widget

import "github.com/egoist/mygo/ui"

// FitGrid is a grid of as many columns as fit cells at least cell wide,
// from one up to most: cards wrap onto rows as the room narrows rather
// than squeezing their text.
func FitGrid(c *ui.Context, cell float32, most int) ui.Element {
	g := ui.Grid(c).FillWidth()
	n := most
	if w := g.Bounds().W; w > 0 {
		n = max(1, min(most, int(w/cell)))
	}
	return Columns(g, n)
}
