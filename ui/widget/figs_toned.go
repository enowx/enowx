package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// FigsBandToned is a FigsBand whose values may stand out: tones holds, by
// figure's index, the color of those that do.
func FigsBandToned(c *ui.Context, figs []Fig, tones map[int]ui.Color) {
	ui.Column(c.Key("figs-band")).FillWidth().Shrink(0).Padding(theme.S4, theme.S5, 0, theme.S5).Children(func() {
		ui.Row(c.Key("figs")).FillWidth().Shrink(0).Radius(theme.RadiusCard).Clip().Background(figGround()).AlignItems(ui.Stretch).Children(func() {
			for i, f := range figs {
				cell := ui.Column(c.Key(f.Label)).Grow(1).Basis(0).MinWidth(0).Padding(theme.S3, theme.S4)
				if i > 0 {
					cell.BorderWidth(0, 0, 0, 0.5).BorderColor(ui.RGBA(255, 255, 255, 0.14))
				}
				cell.Children(func() {
					Running(c.Key("l"), f.Label, RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
					Running(c.Key("v"), f.Value, RunStyle{Size: 24, Weight: 500, Color: tones[i]})
				})
			}
		})
	})
}
