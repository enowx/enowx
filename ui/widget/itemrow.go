package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// Item is what an ItemRow says.
type Item struct {
	Dot        ui.Color // zero leaves it out
	Hollow     bool     // a ring instead of a dot
	Title, Sub string
	TitleColor ui.Color // zero inherits
	SubColor   ui.Color // zero is the quieter text
	Mono       bool     // the title is code: a URL, a pattern
	On         bool
	Label      string // what the row is called, its title by default
}

// ItemRow is a ListItem with controls of its own at its right: they sit beside
// the row's button, not in it, so that using a switch or a menu there doesn't
// also pick the row. It returns the button.
func ItemRow(c *ui.Context, it Item, right func()) ui.Element {
	row := ui.Row(c).FillWidth().MinHeight(theme.RowHeight).Shrink(0).Radius(theme.RadiusCard).Gap(theme.S2)
	bg := theme.Fade(theme.Selected, 0)
	switch {
	case it.On:
		bg = theme.Selected
	case row.Hovered():
		bg = theme.Hover
	}
	label := it.Label
	if label == "" {
		label = it.Title
	}
	var b ui.Element
	row.Background(QuickTint(row, "bg", bg)).Children(func() {
		b = ui.ButtonBase(c.Key("item")).Grow(1).Basis(0).MinWidth(0).MinHeight(theme.RowHeight).Padding(theme.S1, 0, theme.S1, theme.S3).Gap(theme.S2).
			Justify(ui.Start).Label(label).Cursor(ui.CursorPointer)
		b.Children(func() {
			if it.Dot.A != 0 {
				Dot(c, it.Dot, it.Hollow, 8)
			}
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				Running(c, it.Title, RunStyle{Always: it.On, Mono: it.Mono, Color: it.TitleColor})
				if it.Sub != "" {
					col := it.SubColor
					if col.A == 0 {
						col = theme.SoftOn(it.On)
					}
					Running(c.Key("sub"), it.Sub, RunStyle{Size: theme.TextLabel, Color: col, Always: it.On})
				}
			})
		})
		ui.Row(c).Shrink(0).Gap(theme.S2).PaddingX(theme.S2).Children(func() {
			if right != nil {
				right()
			}
		})
	})
	return b
}
