package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/canvasview"
)

// fakePage stands in for a web view: it says the element at any point is
// #hero.
type fakePage struct{}

func (fakePage) Show(c *ui.Context) ui.Element { return ui.Box(c) }
func (fakePage) LoadHTML(string)               {}
func (fakePage) SetZoom(float64)               {}
func (fakePage) Close()                        {}
func (fakePage) Eval(code string) (any, error) {
	if strings.HasPrefix(code, "(function(x,y){var e=document.elementFromPoint") {
		return "hero|100|60|600|200", nil
	}
	if code == canvasview.WiderScript {
		return float64(0), nil
	}
	return nil, nil
}

// A click on a mockup picks the component under it: a bar over it offers
// asking the AI, a redesign, and a note, which the host keeps on it.
func TestCanvasComponentPickedAndNoted(t *testing.T) {
	h := newCanvasHarness(t)
	h.app.canvases.WebOf(func() *canvasview.Web {
		return &canvasview.Web{NewPage: func(done func(canvasview.Page, error)) { done(fakePage{}, nil) }, Update: h.app.onMain}
	})
	cv, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Landing", Kind: proto.CanvasMockup, Frames: []proto.CanvasFrame{{Name: "Home"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, HTML: `<section id="hero"><h1>Hi</h1></section>`}); err != nil {
		t.Fatal(err)
	}
	h.settle("the design", func() bool { return h.tt.HasText("Design Home") })
	d, _ := h.tt.Find("Design Home")
	h.tt.Press(d.X+40, d.Y+40)
	h.tt.Release(d.X+40, d.Y+40)
	h.tt.Frame()
	h.settle("the bar", func() bool { _, ok := h.tt.Find("About #hero"); return ok })
	if !h.tt.HasText("Redesign") {
		t.Fatalf("no redesign: %q", h.tt.Texts())
	}
	shot(t, h.tt, "canvas-component.png")
	h.click(t, "Write a note instead of asking the AI")
	h.click(t, "About #hero")
	h.tt.Type("make the headline bigger")
	h.tt.Key(0, ui.KeyEnter)
	h.settle("the note kept", func() bool {
		list, _ := h.cl.CanvasComments(cv.ID)
		return len(list) == 1 && list[0].Element == "hero" && list[0].Text == "make the headline bigger"
	})
	h.tt.Key(0, ui.KeyEscape)
	h.settle("the bar gone", func() bool { _, ok := h.tt.Find("About #hero"); return !ok })
}

// While an agent works on another design the user stays on theirs, wherever
// it works.
func TestCanvasUserStaysOnTheDesignPicked(t *testing.T) {
	h := newCanvasHarness(t)
	cv, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Landing", Kind: proto.CanvasWireframe,
		Frames: []proto.CanvasFrame{{Name: "One"}, {Name: "Two"}}})
	if err != nil {
		t.Fatal(err)
	}
	agent := func(frame, id string) {
		t.Helper()
		el := proto.CanvasElement{ID: id, Type: proto.ShapeRect, X: 10, Y: 10, W: 100, H: 50, Text: id}
		if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Frame: frame, Op: proto.CanvasOpElementSet, Element: &el, Session: "agent-1"}); err != nil {
			t.Fatal(err)
		}
	}
	h.settle("open", func() bool { return h.tt.HasText("Frame One") })
	agent(cv.Frames[1].ID, "b0")
	n := 0
	h.settle("a moment", func() bool { n++; return n > 10 })
	if !h.tt.HasText("Frame One") {
		t.Fatalf("pulled to the agent's design: %q", h.tt.Texts())
	}
	agent(cv.Frames[0].ID, "a1")
	h.settle("the agent's work", func() bool { return h.tt.HasText("a1") })
	h.click(t, "Design after (→)")
	h.settle("design two", func() bool { return h.tt.HasText("Frame Two") })
	agent(cv.Frames[0].ID, "a2")
	n = 0
	h.settle("a moment", func() bool { n++; return n > 10 })
	if !h.tt.HasText("Frame Two") {
		t.Fatalf("pulled back to the agent's design: %q", h.tt.Texts())
	}
}
