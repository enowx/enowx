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

// barMaxWidth is the most a bar takes, however few there are.
const barMaxWidth = 48

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
	// A few bars stand apart, as wide as is pleasant; a year's are slivers.
	var gap, partGap float32
	switch {
	case len(bars) <= 31:
		gap, partGap = theme.S2, 2
	case len(bars) <= 90:
		gap = 2
	}
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		ui.Row(c).FillWidth().Height(height).Gap(gap).AlignItems(ui.End).Children(func() {
			for i, b := range bars {
				cell := ui.Column(c.Key(i)).Grow(1).Basis(0).MinWidth(0).Height(height).AlignItems(ui.Center)
				if b.Tip != "" {
					cell.Tooltip(b.Tip)
				}
				cell.Children(func() {
					ui.Column(c).FillWidth().MaxWidth(barMaxWidth).Height(height).Justify(ui.End).Gap(partGap).Radius(theme.S2).Clip().Children(func() {
						room := float64(height) - float64(partGap)*float64(max(len(b.Parts)-1, 0))
						for j := len(b.Parts) - 1; j >= 0; j-- {
							if top <= 0 || b.Parts[j] <= 0 {
								continue
							}
							h := float32(b.Parts[j] / top * room)
							ui.Box(c.Key(j)).FillWidth().Shrink(0).Height(max(h, 1)).Background(colors[j%len(colors)])
						}
					})
				})
			}
		})
		// Each label has its bar's width, or the width of the bars up to the
		// next label, so it stays inside what it names.
		ui.Row(c).FillWidth().Gap(gap).Children(func() {
			for i := 0; i < len(bars); i += every {
				span := min(every, len(bars)-i)
				text := ui.Text(c.Key(i), bars[i].Label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Grow(float32(span)).Basis(0).MinWidth(0)
				if every == 1 {
					text.TextAlign(ui.Center)
				} else {
					text.TextAlign(ui.Start)
				}
			}
		})
	})
}
