package canvasview

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A design's layout grid: set in the Grid panel (columns, rows, gaps,
// margin), shown on the stage as guides, areas drawn on it by dragging
// across cells, its CSS to copy, and the mockup laid out on it.

// gridForm is the Grid panel's fields, as typed, for design frame.
type gridForm struct {
	frame                              string
	cols, rows, colGap, rowGap, margin string
	err                                string
	drawing                            bool
	fromCol, fromRow, toCol, toRow     int
	pressed                            bool
}

// defaultGrid is the grid a design gets first: 12 columns.
func defaultGrid(f proto.CanvasFrame) proto.CanvasGrid {
	g := proto.CanvasGrid{Cols: 12, Rows: 6, ColGap: 24, RowGap: 24, Margin: 48, Snap: true}
	switch {
	case f.Width <= 480:
		g.Cols, g.ColGap, g.RowGap, g.Margin = 4, 16, 16, 16
	case f.Width <= 900:
		g.Cols, g.ColGap, g.RowGap, g.Margin = 8, 16, 16, 32
	}
	return g
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// form is the panel's fields for f, filled from its grid when another
// design shows.
func (v *View) form(f proto.CanvasFrame) *gridForm {
	if v.gridF == nil || v.gridF.frame != f.ID {
		g := defaultGrid(f)
		if f.Grid != nil {
			g = *f.Grid
		}
		v.gridF = &gridForm{frame: f.ID, cols: strconv.Itoa(g.Cols), rows: strconv.Itoa(g.Rows), colGap: num(g.ColGap), rowGap: num(g.RowGap), margin: num(g.Margin)}
	}
	return v.gridF
}

// setGrid gives the design shown grid g.
func (v *View) setGrid(o Options, g *proto.CanvasGrid) {
	v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameGrid, Grid: g})
}

// gridPanel is the Grid panel.
func (v *View) gridPanel(c *ui.Context, o Options) {
	f := v.frameOf()
	gf := v.form(f)
	if f.Grid == nil {
		ui.Text(c, "A layout grid keeps a design's blocks on the same columns and spacing: shapes snap to it, and its CSS lays a mockup out.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		g := defaultGrid(f)
		if widget.Button(c, widget.Primary, icon.Grid, fmt.Sprintf("Add a %d-column grid", g.Cols), "").Clicked() {
			v.setGrid(o, &g)
		}
		return
	}
	g := *f.Grid
	field := func(key, label string, value *string) {
		ui.Column(c.Key(key)).Grow(1).Basis(0).MinWidth(64).Gap(4).Children(func() {
			ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.Muted)
			if widget.Input(c, value, "").Label("Grid " + strings.ToLower(label)).Submitted() {
				v.applyGrid(o, g)
			}
		})
	}
	widget.FitGrid(c, 110, 3).Gap(8).Children(func() {
		field("cols", "Columns", &gf.cols)
		field("rows", "Rows", &gf.rows)
		field("margin", "Margin", &gf.margin)
		field("colgap", "Column gap", &gf.colGap)
		field("rowgap", "Row gap", &gf.rowGap)
	})
	ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
		if widget.Button(c, widget.Primary, icon.Check, "Apply", "").Height(28).PaddingX(10).Label("Apply grid").Clicked() {
			v.applyGrid(o, g)
		}
		snap := g.Snap
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if widget.Switch(c, &snap, "Snap shapes") {
				g.Snap = snap
				v.setGrid(o, &g)
			}
			ui.Text(c, "Snap").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		})
		show := !v.hideGrid
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if widget.Switch(c, &show, "Guides") {
				v.setGridHidden(o, !show)
			}
			ui.Text(c, "Guides").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		})
	})
	if gf.err != "" {
		ui.Text(c, gf.err).FontSize(theme.TextLabel).TextColor(theme.Error)
	}
	widget.Divider(c)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Areas").FontSize(theme.TextLabel).FontWeight(600).Grow(1)
		label := "Draw an area"
		if gf.drawing {
			label = "Drawing: drag across cells"
		}
		if widget.Button(c, widget.Secondary, icon.Square, label, "").Height(26).PaddingX(10).Label("Draw an area").Clicked() {
			gf.drawing = !gf.drawing
			if gf.drawing {
				v.setGridHidden(o, false) // what's drawn on is the grid
			}
		}
	})
	if len(g.Areas) == 0 {
		ui.Text(c, "Name the regions of the layout: draw one by dragging across the grid's cells on the design.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	for i, a := range g.Areas {
		ui.Row(c.Key("area:" + a.Name)).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "#"+a.Name).FontSize(theme.TextLabel).TextColor(theme.Text).Grow(1).SingleLine()
			ui.Text(c, fmt.Sprintf("col %d–%d · row %d–%d", a.Col, a.Col+a.ColSpan-1, a.Row, a.Row+a.RowSpan-1)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			if widget.IconButton(c.Key("rm"), icon.Close, "Remove area "+a.Name, false).Clicked() {
				g.Areas = slices.Delete(slices.Clone(g.Areas), i, i+1)
				v.setGrid(o, &g)
			}
		})
	}
	widget.Divider(c)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, "CSS").FontSize(theme.TextLabel).FontWeight(600).Grow(1)
		if widget.IconButton(c, icon.Copy, "Copy the grid's CSS", false).Clicked() {
			c.WriteClipboard(canvas.GridCSS(g))
		}
	})
	widget.CodeBlock(c, "css", canvas.GridCSS(g))
	ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
		if widget.Button(c, widget.Secondary, icon.Layers, "Lay the mockup out on it", "").Height(28).PaddingX(10).
			Tooltip("The design's HTML becomes the grid: its CSS, and an empty element per area to fill (undo brings it back)").Clicked() {
			v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpHTMLSet, HTML: canvas.GridHTML(g)})
		}
		if widget.GhostButton(c, "Remove grid").Clicked() {
			v.setGrid(o, nil)
		}
	})
}

// applyGrid gives the design the grid its fields say, its areas kept.
func (v *View) applyGrid(o Options, g proto.CanvasGrid) {
	gf := v.gridF
	ints := []*int{&g.Cols, &g.Rows}
	for i, s := range []string{gf.cols, gf.rows} {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			gf.err = "Columns and rows are whole numbers"
			return
		}
		*ints[i] = n
	}
	floats := []*float64{&g.ColGap, &g.RowGap, &g.Margin}
	for i, s := range []string{gf.colGap, gf.rowGap, gf.margin} {
		n, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "px"), 64)
		if err != nil {
			gf.err = "Gaps and margin are numbers of pixels"
			return
		}
		*floats[i] = n
	}
	// Areas past the new grid's cells go.
	g.Areas = slices.DeleteFunc(slices.Clone(g.Areas), func(a proto.GridArea) bool {
		return a.Col+a.ColSpan-1 > g.Cols || a.Row+a.RowSpan-1 > g.Rows
	})
	if err := canvas.CheckGrid(&g, v.frameOf()); err != nil {
		gf.err = strings.TrimPrefix(err.Error(), "canvas: ")
		return
	}
	gf.err = ""
	v.setGrid(o, &g)
}

// gridGuides draws the design's grid over it: its columns tinted, its rows
// lined, its areas outlined and named; while drawing an area, a drag
// across cells makes one.
func (v *View) gridGuides(c *ui.Context, o Options, f proto.CanvasFrame, s float64) {
	if f.Grid == nil || v.hideGrid {
		return
	}
	g := *f.Grid
	gf := v.form(f)
	drawing := v.panel == "grid" && gf.drawing
	cols, rows := canvas.Tracks(g, f)
	guides := ui.Box(c.Key("grid-guides")).Absolute().Left(0).Top(0).FillWidth().FillHeight().Label("Grid guides")
	if !drawing {
		guides.PassThrough() // seen, not in the way of the design's pointer
	}
	guides.Draw(func(p *ui.Painter, r ui.Rect) {
		for _, t := range cols {
			p.Fill(ui.Rect{X: r.X + float32(t.Start*s), Y: r.Y, W: float32(t.Size * s), H: r.H}, ui.RGBA(239, 68, 68, 0.07), 0)
		}
		for _, t := range rows {
			for _, y := range []float64{t.Start, t.Start + t.Size} {
				p.Line(r.X, r.Y+float32(y*s), r.X+r.W, r.Y+float32(y*s), 1, ui.RGBA(239, 68, 68, 0.22))
			}
		}
		for _, a := range g.Areas {
			x, y, w, h := canvas.AreaBox(g, f, a)
			box := ui.Rect{X: r.X + float32(x*s), Y: r.Y + float32(y*s), W: float32(w * s), H: float32(h * s)}
			p.Fill(box, ui.RGBA(99, 102, 241, 0.08), 4)
			p.Stroke(box, ui.RGBA(99, 102, 241, 0.7), 4, 1.5)
			p.Text(box.X+6, box.Y+4, "#"+a.Name, 11, ui.RGB(79, 70, 229))
		}
		if drawing && gf.pressed {
			a := gf.area()
			x, y, w, h := canvas.AreaBox(g, f, a)
			p.Stroke(ui.Rect{X: r.X + float32(x*s), Y: r.Y + float32(y*s), W: float32(w * s), H: float32(h * s)}, ui.RGB(99, 102, 241), 4, 2)
		}
	})
	if !drawing {
		return
	}
	guides.Cursor(ui.CursorCrosshair)
	px, py, over := guides.PointerPosition()
	col, row := canvas.CellAt(g, f, float64(px)/s, float64(py)/s)
	switch pressed := guides.Pressed(); {
	case pressed && !gf.pressed && over:
		gf.pressed, gf.fromCol, gf.fromRow = true, col, row
		gf.toCol, gf.toRow = col, row
	case pressed && gf.pressed:
		gf.toCol, gf.toRow = col, row
	case !pressed && gf.pressed:
		gf.pressed = false
		a := gf.area()
		for n := len(g.Areas) + 1; ; n++ {
			a.Name = fmt.Sprintf("area-%d", n)
			if !slices.ContainsFunc(g.Areas, func(x proto.GridArea) bool { return x.Name == a.Name }) {
				break
			}
		}
		g.Areas = append(slices.Clone(g.Areas), a)
		v.setGrid(o, &g)
	}
}

// area is the area dragged across, from the first cell to the last.
func (gf *gridForm) area() proto.GridArea {
	c0, c1 := min(gf.fromCol, gf.toCol), max(gf.fromCol, gf.toCol)
	r0, r1 := min(gf.fromRow, gf.toRow), max(gf.fromRow, gf.toRow)
	return proto.GridArea{Col: c0, Row: r0, ColSpan: c1 - c0 + 1, RowSpan: r1 - r0 + 1}
}

// snapped is shape el where it goes on the design's grid as it is dragged.
func snapped(f proto.CanvasFrame, el proto.CanvasElement) proto.CanvasElement {
	if f.Grid == nil || !f.Grid.Snap || el.Type == proto.ShapeLine || el.Type == proto.ShapeArrow {
		return el
	}
	el.X, el.Y, el.W, el.H = canvas.Snap(*f.Grid, f, el.X, el.Y, math.Max(el.W, 1), math.Max(el.H, 1))
	return el
}

// setGridHidden shows or hides the guides of the canvas open: a choice of this
// device's view, apart from the grid, its areas and its snapping, which stay.
func (v *View) setGridHidden(o Options, hidden bool) {
	v.hideGrid = hidden
	if o.SetGridHidden != nil && v.selected != "" {
		o.SetGridHidden(v.selected, hidden)
	}
}

// gridToggle is the toolbar's button for the guides, for a design with a grid.
func (v *View) gridToggle(c *ui.Context, o Options, f proto.CanvasFrame) {
	if f.Grid == nil {
		return
	}
	svg, tip := icon.Eye, "Hide the grid (Shift+G): it stays, with its settings"
	if v.hideGrid {
		svg, tip = icon.EyeOff, "Show the grid (Shift+G)"
	}
	if widget.IconButton(c.Key("grid-eye"), svg, tip, v.hideGrid).Clicked() || c.Shortcut(ui.Shift, ui.KeyG) {
		v.setGridHidden(o, !v.hideGrid)
	}
}
