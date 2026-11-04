package canvasview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/designbrief"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The canvas's brief (proto.DesignBrief): what its designs are for,
// written by its agent before designing or redesigning, read and changed
// here before the agents start and between rounds.

// briefForm is the brief being edited, its sections by heading.
type briefForm struct {
	open   bool
	values []string
}

// briefButton opens the canvas's brief; lit when it has one.
func (v *View) briefButton(c *ui.Context, cv proto.Canvas) {
	label := "Brief: what these designs are for"
	if cv.Brief != nil {
		label = "Brief: " + designbrief.Summary(*cv.Brief)
	}
	var btn ui.Element
	if v.compact() {
		btn = widget.IconButton(c.Key("brief"), icon.Target, label, cv.Brief != nil)
	} else {
		btn = widget.GhostButton(c.Key("brief"), "Brief").Label(label).Tooltip(label)
		if cv.Brief != nil {
			btn.Background(theme.Selected) // lit: it has one
		}
	}
	if btn.Clicked() {
		b := proto.DesignBrief{}
		if cv.Brief != nil {
			b = *cv.Brief
		}
		f := &briefForm{open: true}
		for _, kv := range designbrief.Fields(b) {
			f.values = append(f.values, kv[1])
		}
		v.briefing = f
	}
}

func (v *View) briefDialog(c *ui.Context, o Options) {
	f := v.briefing
	if f == nil {
		return
	}
	if !f.open {
		v.briefing = nil
		return
	}
	widget.FormSheet(c, &f.open, 480, "Brief", "What the canvas's designs are for. Agents write it before designing and judge every design against it.", func() {
		ui.Column(c).FillWidth().Gap(theme.S3).Children(func() {
			for i, kv := range designbrief.Fields(proto.DesignBrief{}) {
				heading := kv[0]
				ui.Column(c.Key(heading)).FillWidth().Gap(theme.S1).Children(func() {
					widget.Field(c, heading, func() {
						widget.Well(ui.TextArea(c, &f.values[i]).FillWidth().TextColor(theme.Text).Height(60).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).FontSize(theme.TextUI).
							Label(heading).Placeholder(designbrief.Asks(heading)), false)
					})
				})
			}
		})
		briefReferences(c, v.canvas)
	}, func() {
		if widget.PrimaryButton(c, nil, "Save", "").Clicked() {
			var b proto.DesignBrief
			empty := true
			for i, kv := range designbrief.Fields(b) {
				designbrief.Set(&b, kv[0], strings.TrimSpace(f.values[i]))
				empty = empty && strings.TrimSpace(f.values[i]) == ""
			}
			e := proto.CanvasEdit{Op: proto.CanvasOpBrief}
			if !empty {
				e.Brief = &b
			}
			v.edit(o, e)
			f.open = false
		}
	})
}
