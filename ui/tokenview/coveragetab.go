package tokenview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
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

// layerColor says a layer at a glance.
func layerColor(layer string) ui.Color {
	switch layer {
	case proto.TokenLayerRecords:
		return theme.Done
	case proto.TokenLayerTraffic:
		return theme.Working
	case proto.TokenLayerScreen:
		return theme.Warning
	}
	return theme.Idle
}

// coverageCardHeight is the least an agent's card takes: its note on two lines
// and where its records are.
const coverageCardHeight = 168

// coverage is how each agent is tracked: its layer, what is counted, where, and
// whether that was checked against a real install.
func (v *View) coverage(c *ui.Context, d Data) {
	widget.CardNote(c, "Each agent is tracked by the best layer it allows: its own records; else its model API's answers through the interceptor; else what it prints.", 0)
	v.cards(c, len(d.Coverage), coverageCardHeight, func() {
		for _, cv := range d.Coverage {
			widget.GridCard(c.Key(cv.Agent)).Children(func() {
				widget.CardTitle(c, nil, orText(cv.Name, cv.Agent), func() {
					if cv.Checked {
						widget.Tag(c, "checked", theme.Done)
					}
				})
				ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
					widget.Avatar(c, icon.Agent(cv.Agent), 32, theme.Text)
					widget.StatusLine(c, layerColor(cv.Layer), false, layerNames[cv.Layer])
				})
				widget.CardNote(c, cv.Records, 0)
				if cv.Where != "" {
					ui.Row(c).FillWidth().Shrink(0).MinWidth(0).Children(func() {
						widget.Running(c, cv.Where, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft, Mono: true})
					})
				}
			})
		}
	})
}
