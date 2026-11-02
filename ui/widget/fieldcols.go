package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// FieldCols puts fields side by side in columns of the same width. Not in Cols:
// a Field fills what holds it, and a grid's cell holds it at the grid's whole
// width, so it sticks out over its neighbors.
func FieldCols(c *ui.Context, fields ...func()) {
	ui.Row(c.Key("field-cols")).FillWidth().Shrink(0).Gap(theme.S3).AlignItems(ui.Start).Children(func() {
		for i, field := range fields {
			ui.Column(c.Key(i)).Grow(1).Basis(0).MinWidth(0).Children(field)
		}
	})
}
