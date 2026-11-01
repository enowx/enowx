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
	area := ui.Scroll(c.Key("grid")).Grow(1).MinWidth(0).FillHeight().Background(theme.Sunken)
	room := area.Bounds().W
	area.Children(func() {
		ui.Column(c).FillWidth().Padding(20).Gap(12).Children(func() {
			if n := len(v.comparing(cv)); n > 0 {
				ui.Text(c, fmt.Sprintf("%d picked to compare (2 to 4): Compare above shows them side by side", n)).FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
			// As many cards a row as fit 280 wide, as wide as the room
			// shares out: the grid fills it at any width.
			const cell, gap = 280, 16
			cw := float32(cell)
			if w := room - 40; w > 0 {
				n := max(1, int((w+gap)/(cell+gap)))
				cw = (w - gap*float32(n-1)) / float32(n)
			}
			// A row's cards as tall as its tallest, their tops in line.
			ui.Row(c).FillWidth().Gap(gap).Wrap().AlignItems(ui.Stretch).Children(func() {
				for i, f := range cv.Frames {
					v.gridCard(c.Key(f.ID), o, cv, i, f, cw)
				}
			})
		})
	})
}

func (v *View) gridCard(c *ui.Context, o Options, cv proto.Canvas, i int, f proto.CanvasFrame, w float32) {
	pw := w - 22
	card := ui.Column(c).Width(w).Padding(10).Gap(8).Radius(theme.RadiusSmall).Background(theme.Raised).Border(1, theme.Border)
	if v.working(f) {
		card.Border(1.5, theme.Accent)
	}
	card.Children(func() {
		pv := ui.ButtonBase(c.Key("open")).Cursor(ui.CursorPointer).Label(fmt.Sprintf("Open design %d", i+1))
		pv.Children(func() { v.designPreview(c.Key("pv"), o, f, pw) })
		if pv.Clicked() {
			v.frame, v.sel, v.view = f.ID, "", viewDesign
		}
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%d · %s", i+1, f.Name)).FontSize(theme.TextUI).FontWeight(600).SingleLine().Shrink(1).Grow(1).MinWidth(0)
			if cv.Winner == f.ID {
				widget.Tag(c, "Winner", theme.Done)
			}
			v.starButton(c, o, f)
		})
		if g := groupOf(f); g != "" || f.Description != "" {
			desc := strings.Trim(g+" — "+f.Description, " —")
			ui.Text(c, desc).FontSize(theme.TextLabel).TextColor(theme.Muted).MaxLines(3).Tooltip(desc)
		}
		// The button at the card's foot, whatever the text above.
		ui.Spacer(c)
		on := v.cmpSel[f.ID]
		label, svg := "Pick to compare", icon.Layers
		if on {
			label, svg = "Picked to compare", icon.Check
		}
		b := widget.Button(c.Key("cmp"), widget.Secondary, svg, label, "").Height(26).PaddingX(10).Label(fmt.Sprintf("Compare design %d", i+1))
		if on {
			b.Border(1, theme.Accent)
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
