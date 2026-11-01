package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// Dot draws a status mark: filled, or a ring when hollow.
func Dot(c *ui.Context, col ui.Color, hollow bool, size float32) {
	d := ui.Box(c).Size(size, size).Radius(size / 2).Shrink(0)
	if hollow {
		d.Border(1.2, col)
	} else {
		d.Background(col)
	}
}

// Status is what a status badge says: its words and color, and whether
// nothing is happening (a ring instead of a dot).
type Status struct {
	Label  string
	Color  ui.Color
	Hollow bool
}

// Badge is a status: a dot then its words, without a filled block, so a
// list of many stays calm.
func Badge(c *ui.Context, s Status) {
	ui.Row(c).Gap(6).Shrink(0).Children(func() {
		Dot(c, s.Color, s.Hollow, 6)
		ui.Text(c, s.Label).FontSize(11.5).TextColor(theme.TextSoft).SingleLine()
	})
}

// Tag is a small outlined label, as New, Changed, git; col colors its
// words, theme.TextSoft when zero.
func Tag(c *ui.Context, label string, col ui.Color) {
	border := theme.BorderStrong
	if col.A == 0 {
		col = theme.TextSoft
	} else {
		border = theme.Fade(col, 0.35)
	}
	ui.Box(c).PaddingX(8).Height(18).Radius(9).Border(1, border).Center().Shrink(0).Children(func() {
		ui.Text(c, label).FontSize(10.5).TextColor(col).SingleLine()
	})
}
