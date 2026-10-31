package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// Bar is one bar of a BarChart: its label under it (shown when there is
// room), its tooltip, and its parts' values, stacked from the bottom.
type Bar struct {
	Label, Tip string
	Parts      []float64
}

// BarChart draws bars of stacked parts, each part its color of colors,
// scaled to the tallest bar, height high.
func BarChart(c *ui.Context, bars []Bar, colors []ui.Color, height float32) {
	top := 0.0
	for _, b := range bars {
		sum := 0.0
		for _, p := range b.Parts {
			sum += p
		}
		top = max(top, sum)
	}
	// Labels on at most about eight bars, so they don't crowd.
	every := max(1, (len(bars)+7)/8)
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		ui.Row(c).FillWidth().Height(height).Gap(3).AlignItems(ui.End).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
			for i, b := range bars {
				col := ui.Column(c.Key(i)).Grow(1).MinWidth(0).Height(height).Justify(ui.End).Radius(2).Clip()
				if b.Tip != "" {
					col.Tooltip(b.Tip).Label(b.Tip)
				}
				col.Children(func() {
					for j := len(b.Parts) - 1; j >= 0; j-- {
						if top <= 0 || b.Parts[j] <= 0 {
							continue
						}
						h := float32(b.Parts[j] / top * float64(height))
						ui.Box(c.Key(j)).FillWidth().Height(max(h, 1)).Background(colors[j%len(colors)])
					}
				})
			}
		})
		ui.Row(c).FillWidth().Gap(3).Children(func() {
			for i, b := range bars {
				label := ""
				if i%every == 0 {
					label = b.Label
				}
				ui.Text(c.Key(i), label).FontSize(10).TextColor(theme.Muted).SingleLine().Grow(1).MinWidth(0).TextAlign(ui.Start)
			}
		})
	})
}
