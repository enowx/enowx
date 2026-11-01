package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
)

// Asked for ten designs, an agent makes them one by one: each shows in the
// strip as it comes; the user steps through them with the keys, looks at
// them all as a grid, compares two side by side, stars one, picks one,
// drags one first, and follows a link to one.
func TestCanvasManyDesigns(t *testing.T) {
	h := newCanvasHarness(t)
	cv, _ := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Landing pages", Kind: proto.CanvasWireframe, Frames: []proto.CanvasFrame{{Name: "Design 1"}}})
	h.settle("the canvas open", func() bool { return h.app.canvases.Selected() == cv.ID })
	agent := func(e proto.CanvasEdit) {
		t.Helper()
		e.ID, e.Session = cv.ID, "agent-3"
		if _, err := h.cl.EditCanvas(e); err != nil {
			t.Fatal(err)
		}
	}
	for n := 1; n <= 10; n++ {
		name := fmt.Sprintf("Landing %d", n)
		if n == 1 {
			agent(proto.CanvasEdit{Op: proto.CanvasOpFrameMeta, Frame: "1", Name: &name})
		} else {
			agent(proto.CanvasEdit{Op: proto.CanvasOpFrameAdd, FrameInfo: &proto.CanvasFrame{Name: name, Page: "Home", Variant: string(rune('A' + n - 1)), Description: "Idea " + name}})
		}
		hero := proto.CanvasElement{ID: "hero", Type: proto.ShapeRect, X: 100, Y: 100, W: 600, H: 200, Text: "Hero of " + name}
		agent(proto.CanvasEdit{Op: proto.CanvasOpElementSet, Frame: fmt.Sprint(n), Element: &hero})
		h.settle(name+" in the strip", func() bool { return h.tt.HasText(name) && h.tt.HasText(fmt.Sprintf("Design %d", n)) })
		if n == 3 {
			shot(t, h.tt, "canvas-designs-strip.png")
		}
	}

	if err := h.tt.Click("Design 4"); err != nil {
		t.Fatal(err)
	}
	h.settle("design 4 shown", func() bool { return h.tt.HasText("Design 4 of 10") })
	h.tt.Key(0, ui.KeyRight)
	h.settle("design 5 by the key", func() bool { return h.tt.HasText("Design 5 of 10") })
	h.tt.Key(0, ui.Key2)
	h.settle("design 2 by its number", func() bool { return h.tt.HasText("Design 2 of 10") })
	if err := h.tt.Click("Star Landing 2"); err != nil {
		t.Fatal(err)
	}
	h.settle("starred", func() bool { c, _ := h.cl.Canvas(cv.ID); return c.Frames[1].Favorite })

	if err := h.tt.Click("All designs"); err != nil {
		t.Fatal(err)
	}
	h.settle("the grid", func() bool { return h.tt.HasText("10 Landing 10") && h.tt.HasText("Open design 4") })
	card, _ := h.tt.Find("Open design 1")
	h.tt.Scroll(card.X+card.W/2, card.Y+card.H/2, 0, 300) // the third card is a row down
	h.tt.Frame()
	for _, n := range []string{"3", "4"} {
		if err := h.tt.Click("Compare design " + n); err != nil {
			t.Fatal(err)
		}
		h.tt.Frame()
	}
	h.settle("two picked", func() bool {
		return h.tt.HasText("2 picked to compare (2 to 4): Compare above shows them side by side")
	})
	shot(t, h.tt, "canvas-designs-grid.png")
	if err := h.tt.Click("Compare"); err != nil {
		t.Fatal(err)
	}
	h.settle("side by side", func() bool {
		return h.tt.HasText("3 Landing 3") && h.tt.HasText("4 Landing 4") && !h.tt.HasText("Open design 5")
	})
	shot(t, h.tt, "canvas-designs-compare.png")
	if err := h.tt.Click("Pick Landing 4 as the winner"); err != nil {
		t.Fatal(err)
	}
	h.settle("the winner", func() bool { c, _ := h.cl.Canvas(cv.ID); return c.Winner == c.Frames[3].ID })

	if err := h.tt.Click("Design"); err != nil {
		t.Fatal(err)
	}
	h.settle("one design", func() bool { return h.tt.HasText("Design 2 of 10") })
	h.dragTo("Design 4", "Design 1")
	h.settle("4 first", func() bool { c, _ := h.cl.Canvas(cv.ID); return c.Frames[0].Name == "Landing 4" })

	h.app.page = pageNotes
	if !h.app.openCanvasLink(canvas.Link(cv.ID, 6)) {
		t.Fatal("the link wasn't the canvas's")
	}
	h.settle("design 6 from its link", func() bool { return h.tt.HasText("Design 6 of 10") })
}

// dragTo drags the element labeled from onto the one labeled to.
func (h *canvasHarness) dragTo(from, to string) {
	h.t.Helper()
	f, ok := h.tt.Find(from)
	g, ok2 := h.tt.Find(to)
	if !ok || !ok2 {
		h.t.Fatalf("no %q or %q", from, to)
	}
	x0, y0 := f.X+f.W/2, f.Y+f.H/2
	x1, y1 := g.X+g.W/2, g.Y+g.H/2
	h.tt.Press(x0, y0)
	for k := 1; k <= 8; k++ {
		h.tt.Move(x0+(x1-x0)*float32(k)/8, y0+(y1-y0)*float32(k)/8)
		h.tt.Frame()
		time.Sleep(5 * time.Millisecond)
	}
	h.tt.Release(x1, y1)
	h.tt.Frame()
}

// Deleting a design asks first, and it stays until the user agrees.
func TestDeletingADesignAsksFirst(t *testing.T) {
	h := newCanvasHarness(t)
	cv, _ := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Two pages", Kind: proto.CanvasWireframe, Frames: []proto.CanvasFrame{{Name: "First"}}})
	h.settle("the canvas open", func() bool { return h.app.canvases.Selected() == cv.ID })
	if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpFrameAdd, FrameInfo: &proto.CanvasFrame{Name: "Second"}}); err != nil {
		t.Fatal(err)
	}
	h.settle("both designs", func() bool { return h.tt.HasText("Second") })
	frames := func() int {
		got, err := h.cl.Canvas(cv.ID)
		if err != nil {
			t.Fatal(err)
		}
		return len(got.Frames)
	}
	label := ""
	for _, name := range []string{"Second", "First"} {
		if _, ok := h.tt.Find("Design " + name); ok {
			label = "Design " + name
			break
		}
	}
	if label == "" {
		t.Fatalf("no design menu: %q", h.tt.Texts())
	}
	if err := h.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	h.settle("its menu", func() bool { return h.tt.HasText("Delete design") })
	if err := h.tt.Click("Delete design"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if h.app.confirming == nil || frames() != 2 {
		t.Fatalf("asked %v, %d designs", h.app.confirming, frames())
	}
	q := h.app.confirming
	h.app.confirming = nil
	q.do()
	h.settle("deleted once agreed", func() bool { return frames() == 1 })
}
