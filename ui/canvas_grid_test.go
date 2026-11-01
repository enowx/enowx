package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/canvasview"
)

// The user gives a design a grid from the Grid panel, changes its
// columns, draws an area across cells on the design, copies its CSS, lays
// the mockup out on it; a shape dragged lands on the grid's lines.
func TestCanvasGridBuilder(t *testing.T) {
	h := newCanvasHarness(t)
	cv, _ := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Home", Kind: proto.CanvasWireframe})
	box := proto.CanvasElement{ID: "box", Type: proto.ShapeRect, X: 300, Y: 300, W: 200, H: 120, Text: "Box"}
	if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpElementSet, Element: &box}); err != nil {
		t.Fatal(err)
	}
	h.settle("the canvas open", func() bool { return h.tt.HasText("Box") })
	if err := h.tt.Click("Grid"); err != nil {
		t.Fatal(err)
	}
	h.tt.Advance(time.Second)
	h.settle("the panel", func() bool { return h.tt.HasText("Add a 12-column grid") })
	if err := h.tt.Click("Add a 12-column grid"); err != nil {
		t.Fatal(err)
	}
	grid := func() *proto.CanvasGrid { c, _ := h.cl.Canvas(cv.ID); return c.Frames[0].Grid }
	h.settle("the grid", func() bool { return grid() != nil && h.tt.HasText("Column gap") })

	if err := h.tt.Click("Grid columns"); err != nil {
		t.Fatal(err)
	}
	h.tt.KeyAt(ui.Cmd, ui.KeyA)
	h.tt.Type("8")
	if err := h.tt.Click("Apply grid"); err != nil {
		t.Fatal(err)
	}
	h.settle("8 columns", func() bool {
		g, shown := grid(), h.app.canvases.Canvas().Frames[0].Grid
		return g != nil && g.Cols == 8 && shown != nil && shown.Cols == 8
	})

	if err := h.tt.Click("Draw an area"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	r, ok := h.tt.Find("Grid guides")
	if !ok {
		t.Fatal("no guides")
	}
	// From the top left cell to the third column of the second row.
	x0, y0 := r.X+r.W*0.06, r.Y+r.H*0.1
	x1, y1 := r.X+r.W*0.32, r.Y+r.H*0.3
	h.tt.Press(x0, y0)
	for k := 1; k <= 6; k++ {
		h.tt.Move(x0+(x1-x0)*float32(k)/6, y0+(y1-y0)*float32(k)/6)
		h.tt.Frame()
	}
	h.tt.Release(x1, y1)
	h.tt.Frame()
	h.settle("the area", func() bool {
		g := grid()
		return g != nil && len(g.Areas) == 1 && g.Areas[0].Name == "area-1" && g.Areas[0].Col == 1 && g.Areas[0].ColSpan >= 2 && g.Areas[0].RowSpan >= 2
	})
	h.settle("its CSS", func() bool { return h.tt.HasText("#area-1") })
	shot(t, h.tt, "canvas-grid.png")
	if err := h.tt.Click("Copy the grid's CSS"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.tt.Clipboard(), "grid-template-columns: repeat(8, minmax(0, 1fr));") {
		t.Fatalf("clipboard %q", h.tt.Clipboard())
	}
	if err := h.tt.Click("Lay the mockup out on it"); err != nil {
		t.Fatal(err)
	}
	h.settle("laid out", func() bool {
		c, _ := h.cl.Canvas(cv.ID)
		return strings.Contains(c.Frames[0].HTML, `<section id="area-1">`)
	})

	if err := h.tt.Click("Draw an area"); err != nil { // done drawing
		t.Fatal(err)
	}
	h.tt.Frame()
	h.drag("box", 37, 23)
	h.settle("the box on the grid", func() bool {
		c, _ := h.cl.Canvas(cv.ID)
		f := c.Frames[0]
		el := f.Elements[0]
		if el.X == 300 {
			return false
		}
		x, y, w, hh := canvas.Snap(*f.Grid, f, el.X, el.Y, el.W, el.H)
		return x == el.X && y == el.Y && w == el.W && hh == el.H
	})
}

// The toolbar's eye hides the guides in one click, with the panel open too,
// changing nothing of the canvas; the choice stays with the canvas, for
// reopening it and for a grid an agent adds; a design with no grid has no eye.
func TestCanvasGridHides(t *testing.T) {
	const hide, show = "Hide the grid (Shift+G): it stays, with its settings", "Show the grid (Shift+G)"
	h := newCanvasHarness(t)
	guides := func() bool { _, ok := h.tt.Find("Grid guides"); return ok }
	grid := &proto.CanvasGrid{Cols: 4, Rows: 2, Snap: true, Areas: []proto.GridArea{{Name: "heroarea", Col: 1, Row: 1, ColSpan: 2, RowSpan: 1}}}
	cv, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Home", Kind: proto.CanvasWireframe, Frames: []proto.CanvasFrame{{Name: "One"}, {Name: "Two"}}})
	if err != nil {
		t.Fatal(err)
	}
	h.settle("the canvas open", func() bool { return h.tt.HasText("Frame One") })
	if _, ok := h.tt.Find(hide); ok {
		t.Fatal("an eye with no grid")
	}
	if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Frame: cv.Frames[0].ID, Op: proto.CanvasOpFrameGrid, Grid: grid}); err != nil {
		t.Fatal(err)
	}
	h.settle("the guides", func() bool { return guides() })
	before, _ := h.cl.Canvas(cv.ID)

	if err := h.tt.Click(hide); err != nil {
		t.Fatal(err)
	}
	h.settle("hidden", func() bool { return !guides() })
	if !h.app.state.CanvasGridHidden[cv.ID] {
		t.Fatal("the choice wasn't kept")
	}
	after, _ := h.cl.Canvas(cv.ID)
	if after.Version != before.Version || after.Frames[0].Grid == nil || !after.Frames[0].Grid.Snap {
		t.Fatalf("hiding changed the canvas: %+v", after.Frames[0].Grid)
	}

	// An agent's grid on another design, and going there, doesn't bring it back.
	if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Frame: cv.Frames[1].ID, Op: proto.CanvasOpFrameGrid, Grid: grid, Session: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.Click("Design after (→)"); err != nil {
		t.Fatal(err)
	}
	h.settle("the other design", func() bool { return h.tt.HasText("Frame Two") })
	if guides() {
		t.Fatal("the guides came back on another design")
	}

	// Reopened.
	h.app.canvases = canvasview.View{}
	h.settle("the canvas again", func() bool { return h.tt.HasText("Frame One") })
	if guides() {
		t.Fatal("reopened, the guides came back")
	}
	if err := h.tt.Click(show); err != nil {
		t.Fatal(err)
	}
	h.settle("shown", func() bool { return guides() })
	if h.app.state.CanvasGridHidden[cv.ID] {
		t.Fatal("still kept as hidden")
	}
}
