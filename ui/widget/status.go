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
	ui.Row(c).Gap(theme.S1).Shrink(0).Children(func() {
		Dot(c, s.Color, s.Hollow, 8)
		ui.Text(c, s.Label).FontSize(theme.TextLabel).TextColor(theme.Text).SingleLine()
	})
}

// Tag is a small badge, as New, Changed, git; col colors its words,
// theme.Text when zero.
func Tag(c *ui.Context, label string, col ui.Color) {
	if col.A == 0 {
		col = theme.Text
	}
	ui.Box(c).PaddingX(theme.S2).Height(20).Radius(theme.RadiusFull).Background(theme.Fade(theme.TextStrong, 0.1)).Center().Shrink(0).Children(func() {
		ui.Text(c, label).FontSize(theme.TextLabel).TextColor(col).SingleLine()
	})
}
