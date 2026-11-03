package ui

import (
	"math"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// The board moves and zooms as a design tool's: a tall design opens at
// its width, scrolling moves it, pinching zooms at the pointer, dragging
// the board around it moves it, and the zoom's buttons fit it whole and
// show it at its own size.
func TestCanvasBoardMovesAndZooms(t *testing.T) {
	h := newCanvasHarness(t)
	cv, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Landing", Kind: proto.CanvasWireframe,
		Frames: []proto.CanvasFrame{{Name: "Home", Width: 1440, Height: 2800}}})
	if err != nil {
		t.Fatal(err)
	}
	box := proto.CanvasElement{ID: "hero", Type: proto.ShapeRect, X: 100, Y: 100, W: 400, H: 200, Text: "Hero"}
	if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpElementSet, Element: &box}); err != nil {
		t.Fatal(err)
	}
	h.settle("the design", func() bool { _, ok := h.tt.Find("Frame Home"); return ok })
	frame := func() (x, y, w float32) {
		t.Helper()
		f, ok := h.tt.Find("Frame Home")
		if !ok {
			t.Fatalf("no design in %q", h.tt.Texts())
		}
		return f.X, f.Y, f.W
	}
	x0, y0, w0 := frame()
	if w0 < 600 {
		t.Fatalf("a tall design opened %v wide, not at its width", w0)
	}

	h.tt.Scroll(x0+w0/2, y0+200, 0, 30) // two fingers, down the design
	h.tt.Frame()
	if _, y, _ := frame(); math.Abs(float64(y-(y0-30))) > 1 {
		t.Fatalf("scrolled 30 down: the design at %v, from %v", y, y0)
	}
	h.tt.Scroll(x0+w0/2, y0+200, 0, -30)
	h.tt.Frame()

	// Dragging the board beside the design moves it.
	x2, y2, _ := frame()
	px, py := x2+w0+16, y2+100
	h.tt.Press(px, py)
	h.tt.Move(px+40, py+30)
	h.tt.Release(px+40, py+30)
	h.tt.Frame()
	if x, y, _ := frame(); math.Abs(float64(x-x2-40)) > 1 || math.Abs(float64(y-y2-30)) > 1 {
		t.Fatalf("dragged the board by 40, 30: the design moved from %v,%v to %v,%v", x2, y2, x, y)
	}

	// Zoom shows in the shape's size: the design may be wider than the
	// stage shows.
	hero := func() (x, y, w float32) {
		t.Helper()
		f, _ := h.tt.Find("hero")
		return f.X, f.Y, f.W
	}
	hx, hy, hw := hero()
	h.tt.Pinch(hx, hy, 2)
	h.tt.Frame()
	if x, y, w := hero(); math.Abs(float64(w-hw*2)) > 2 || math.Abs(float64(x-hx)) > 1 || math.Abs(float64(y-hy)) > 1 {
		t.Fatalf("pinched to twice at its corner: %v wide at %v,%v, from %v at %v,%v", w, x, y, hw, hx, hy)
	}

	h.click(t, "Fit it all")
	if _, _, w := frame(); w >= w0 {
		t.Fatalf("fit it all: %v wide, the width fit was %v", w, w0)
	}
	h.click(t, "Show it at its own size")
	if !h.tt.HasText("100%") {
		t.Fatalf("its own size: shows %q", h.tt.Texts())
	}
	// The shape still selects after all that.
	h.click(t, "Fit its width")
	h.click(t, "hero")
	h.tt.Advance(time.Second)
	h.settle("the shape selected", func() bool { return h.tt.HasText("Ask agent") })
}

func (h *canvasHarness) click(t *testing.T, label string) {
	t.Helper()
	if err := h.tt.Click(label); err != nil {
		t.Fatalf("%v; shows %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	h.tt.Frame()
}
