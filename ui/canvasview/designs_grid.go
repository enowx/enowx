package canvasview

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Picked is the designs picked to compare (frame IDs).
func (v *View) Picked() []string {
	if v.canvas == nil {
		return nil
	}
	var out []string
	for _, f := range v.comparing(*v.canvas) {
		out = append(out, f.ID)
	}
	return out
}

// comparing is the designs picked to compare that the canvas still has, in
// its order.
func (v *View) comparing(cv proto.Canvas) []proto.CanvasFrame {
	var out []proto.CanvasFrame
	for _, f := range cv.Frames {
		if v.cmpSel[f.ID] {
			out = append(out, f)
		}
	}
	return out
}

// grid is every design at once, to look over, open, star and pick to
// compare.
func (v *View) grid(c *ui.Context, o Options, cv proto.Canvas) {
	area := widget.Scroll(c.Key("grid")).Grow(1).MinWidth(0).FillHeight()
	room := area.Bounds().W - widget.ScrollGutter
	area.Children(func() {
		ui.Column(c).FillWidth().Gap(theme.S3).Children(func() {
			if n := len(v.comparing(cv)); n > 0 {
				widget.Running(c.Key("picked"), fmt.Sprintf("%d picked to compare (2 to 4): Compare above shows them side by side", n), widget.RunStyle{Color: theme.TextSoft})
			}
			// As many cards a row as fit 260 wide, as wide as the room
			// shares out: the grid fills it at any width.
			const cell, gap = 260, theme.S3
			cw := float32(cell)
			if room > 0 {
				n := max(1, int((room+gap)/(cell+gap)))
				cw = (room - gap*float32(n-1)) / float32(n)
			}
			// A row's cards as tall as its tallest, their tops in line.
			ui.Row(c.Key("cards")).FillWidth().Gap(gap).Wrap().AlignItems(ui.Stretch).Children(func() {
				for i, f := range cv.Frames {
					v.gridCard(c.Key(f.ID), o, cv, i, f, cw)
				}
			})
		})
	})
}

func (v *View) gridCard(c *ui.Context, o Options, cv proto.Canvas, i int, f proto.CanvasFrame, w float32) {
	pw := w - 2*theme.S2
	card := widget.Raise(ui.Column(c).Width(w).Padding(theme.S2).Gap(theme.S2).Radius(theme.RadiusCard).Background(theme.Raised))
	if v.working(f) {
		card.Border(1.5, theme.Accent)
	}
	card.Children(func() {
		pv := ui.ButtonBase(c.Key("open")).Cursor(ui.CursorPointer).Label(fmt.Sprintf("Open design %d", i+1)).Radius(theme.RadiusSmall).Clip()
		pv.Children(func() {
			// One shape of box per row of cards, whatever a design's own shape.
			ui.Row(c.Key("box")).Size(pw, pw*10/16).Shrink(0).Background(theme.Sunken).Center().Clip().Children(func() {
				pvw := pw
				if f.Width > 0 && f.Height > 0 {
					pvw = min(pw, pw*10/16*float32(f.Width)/float32(f.Height))
				}
				v.designPreview(c.Key("pv"), o, f, pvw)
			})
		})
		if pv.Clicked() {
			v.frame, v.sel, v.view = f.ID, "", viewDesign
		}
		ui.Row(c.Key("name")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, fmt.Sprintf("%d %s", i+1, f.Name), widget.RunStyle{Weight: 500})
			})
			if cv.Winner == f.ID {
				widget.Tag(c.Key("winner"), "Winner", theme.Done)
			}
			v.starButton(c, o, f)
		})
		if g := groupOf(f); g != "" || f.Description != "" {
			desc := strings.Trim(g+" — "+f.Description, " —")
			ui.Text(c.Key("about"), desc).FontSize(theme.TextLabel).TextColor(theme.TextSoft).MaxLines(3).Tooltip(desc)
		}
		// The button at the card's foot, whatever the text above.
		ui.Spacer(c)
		on := v.cmpSel[f.ID]
		label, svg := "Pick to compare", icon.Layers
		if on {
			label, svg = "Picked to compare", icon.Check
		}
		b := widget.Button(c.Key("cmp"), widget.Secondary, svg, label, "").Label(fmt.Sprintf("Compare design %d", i+1))
		if on {
			b.Background(theme.Selected)
		}
		if b.Clicked() {
			if v.cmpSel == nil {
				v.cmpSel = map[string]bool{}
			}
			if !on && len(v.comparing(cv)) >= 4 {
				v.err = "Compare up to 4 designs"
			} else {
				v.cmpSel[f.ID] = !on
			}
		}
	})
}
