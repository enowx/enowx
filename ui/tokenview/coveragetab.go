package tokenview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// layerNames say a layer in words.
var layerNames = map[string]string{
	proto.TokenLayerRecords: "Its records",
	proto.TokenLayerTraffic: "Its traffic",
	proto.TokenLayerScreen:  "Its screen",
	proto.TokenLayerNone:    "Not tracked",
}

// coverage is how each agent is tracked: its layer, what is counted,
// where, and whether that was checked against a real install.
func coverage(c *ui.Context, d Data) {
	ui.Text(c, "Each agent is tracked by the best layer it allows: its own records; else its model API's answers through the interceptor (Overview → How agents are tracked); else the lines it prints. Never guessed.").
		FontSize(12).TextColor(theme.Muted)
	widget.Card(c).Children(func() {
		for _, cv := range d.Coverage {
			ui.Row(c.Key(cv.Agent)).FillWidth().Padding(7, 10).Gap(12).Children(func() {
				ui.Text(c, orText(cv.Name, cv.Agent)).FontSize(12).Width(150).SingleLine().Shrink(0)
				col := theme.TextSoft
				switch cv.Layer {
				case proto.TokenLayerRecords:
					col = theme.Done
				case proto.TokenLayerNone:
					col = theme.Muted
				}
				ui.Text(c, layerNames[cv.Layer]).FontSize(12).TextColor(col).Width(90).SingleLine().Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, cv.Records).FontSize(11.5).TextColor(theme.TextSoft)
					if cv.Where != "" {
						ui.Text(c, cv.Where).Font(theme.MonoFont).FontSize(10.5).TextColor(theme.Muted)
					}
				})
				if cv.Checked {
					widget.Tag(c, "checked", theme.Done)
				}
			})
		}
	})
}
