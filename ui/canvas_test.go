package ui

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// canvasHarness is the Canvas page on a real daemon, its events applied
// between frames as the app's main thread would.
type canvasHarness struct {
	t     *testing.T
	app   *App
	cl    *client.Client
	tt    *ui.Tester
	later []func()
}

func newCanvasHarness(t *testing.T) *canvasHarness {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a, b := net.Pipe()
	go srv.ServeConn(a)
	h := &canvasHarness{t: t, app: New(Options{})}
	h.app.onMain = func(fn func()) { h.later = append(h.later, fn) }
	h.cl, err = client.New(b, func(ev proto.Event) {
		apply := h.app.canvasEvent(ev)
		switch ev.Name {
		case proto.EventCanvasChat:
			apply = h.app.canvasChatEvent(ev.Data)
		case proto.EventCanvasUnslop:
			apply = h.app.unslopEvent(ev.Data)
		case proto.EventSessionUpdated:
			var info proto.SessionInfo
			if json.Unmarshal(ev.Data, &info) == nil {
				apply = func() { h.app.upsert(info) }
			}
		}
		if apply != nil {
			h.app.onMain(apply)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.cl.Close() })
	h.app.client, h.app.page, h.app.host.Name = h.cl, pageCanvas, "this Mac"
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Width(1280).Height(800).Background(theme.Background).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() { h.app.canvasPage(c) })
			h.app.canvasAgentPanel(c)
			h.app.asksView(c)
		})
	}, 1280, 800)
	h.settle("the canvases listed", func() bool { return h.app.canvasList.loaded })
	return h
}

func (h *canvasHarness) settle(what string, done func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.tt.Frame()
		if done() {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("%s: never; shows %q", what, h.tt.Texts())
		}
		time.Sleep(10 * time.Millisecond)
		for _, fn := range h.later {
			fn()
		}
		h.later = nil
	}
}

// An agent designs a diagram while the user watches: each shape appears
// as the agent writes it, the agent's bar says it designs, the user moves
// a shape meanwhile and pauses the agent, whose next change is refused.
func TestCanvasBuiltLiveWhileTheUserWatches(t *testing.T) {
	h := newCanvasHarness(t)
	cv, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Login flow", Kind: proto.CanvasDiagram})
	if err != nil {
		t.Fatal(err)
	}
	h.settle("the canvas open", func() bool { return h.app.canvases.Selected() == cv.ID && h.tt.HasText("Login flow") })

	agent := func(el proto.CanvasElement) error {
		_, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpElementSet, Element: &el, Session: "agent-7"})
		return err
	}
	steps := []proto.CanvasElement{
		{ID: "user", Type: proto.ShapeEllipse, X: 60, Y: 300, W: 180, H: 90, Text: "User", Fill: "#dbeafe"},
		{ID: "form", Type: proto.ShapeRect, X: 360, Y: 300, W: 200, H: 90, Text: "Login form", Radius: 10},
		{ID: "ok", Type: proto.ShapeDiamond, X: 680, Y: 280, W: 180, H: 130, Text: "Valid?"},
		{Type: proto.ShapeArrow, From: "user", To: "form", Text: "opens"},
		{Type: proto.ShapeArrow, From: "form", To: "ok"},
	}
	for i, el := range steps {
		if err := agent(el); err != nil {
			t.Fatal(err)
		}
		want := el.Text
		if want == "" {
			want = "Valid?"
		}
		h.settle("step "+want, func() bool {
			return h.tt.HasText(want) && h.frameShapes() == i+1
		})
		if i == 1 {
			if !h.tt.HasText("An agent is designing · form") {
				t.Fatalf("no agent bar: %q", h.tt.Texts())
			}
			shot(t, h.tt, "canvas-live.png")
		}
	}

	// The user drags the form while the agent works on.
	h.drag("form", 120, 40)
	h.settle("the form moved on the host", func() bool {
		got, _ := h.cl.Canvas(cv.ID)
		for _, el := range got.Frames[0].Elements {
			if el.ID == "form" {
				return el.X > 400 && el.Y > 320
			}
		}
		return false
	})
	if err := agent(proto.CanvasElement{ID: "done", Type: proto.ShapeSticky, X: 980, Y: 300, Text: "Dashboard"}); err != nil {
		t.Fatal(err)
	}
	h.settle("the agent's sticky", func() bool { return h.tt.HasText("Dashboard") })
	got, _ := h.cl.Canvas(cv.ID)
	if el := got.Frames[0].Elements[1]; el.ID != "form" || el.X < 400 {
		t.Fatalf("the user's move was lost: %+v", el)
	}

	if err := h.tt.Click("Pause"); err != nil {
		t.Fatal(err)
	}
	h.settle("paused", func() bool {
		return h.tt.HasText("Resume")
	})
	shot(t, h.tt, "canvas-paused.png")
	if err := agent(proto.CanvasElement{ID: "late", Type: proto.ShapeRect}); err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("an agent's change while paused: %v", err)
	}
	if err := h.tt.Click("Resume"); err != nil {
		t.Fatal(err)
	}
	h.settle("resumed", func() bool { return !h.tt.HasText("Resume") })
}

// frameShapes counts the shapes of the canvas open, as the page has it.
func (h *canvasHarness) frameShapes() int {
	cv := h.app.canvases.Canvas()
	if cv == nil || len(cv.Frames) == 0 {
		return 0
	}
	return len(cv.Frames[0].Elements)
}

// drag drags the element labeled label by dx, dy.
func (h *canvasHarness) drag(label string, dx, dy float32) {
	h.t.Helper()
	f, ok := h.tt.Find(label)
	if !ok {
		h.t.Fatalf("no %q in %q", label, h.tt.Texts())
	}
	x0, y0 := f.X+f.W/2, f.Y+f.H/2
	h.tt.Press(x0, y0)
	for k := 1; k <= 8; k++ {
		h.tt.Move(x0+dx*float32(k)/8, y0+dy*float32(k)/8)
		h.tt.Frame()
	}
	h.tt.Release(x0+dx, y0+dy)
	h.tt.Frame()
}

// A mockup's elements show as an agent writes them (here, without a web
// view, as their outline), and the user comments on one and looks at the
// versions.
func TestCanvasMockupCommentsAndVersions(t *testing.T) {
	h := newCanvasHarness(t)
	cv, _ := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Pricing", Kind: proto.CanvasMockup})
	h.settle("the canvas open", func() bool { return h.app.canvases.Selected() == cv.ID })
	parts := []struct{ into, html, id string }{
		{"", `<main id="page"></main>`, "page"},
		{"page", `<h1 id="headline">Simple pricing</h1>`, "headline"},
		{"page", `<button id="cta">Start free</button>`, "cta"},
	}
	for _, p := range parts {
		if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLPatch, Into: p.into, HTML: p.html, Session: "agent-1"}); err != nil {
			t.Fatal(err)
		}
		h.settle("#"+p.id, func() bool { return h.tt.HasText("#" + p.id) })
	}
	if !h.tt.HasText("Simple pricing") || !h.tt.HasText("Mockup") && !h.tt.HasText("mockup · v4") {
		t.Fatalf("mockup: %q", h.tt.Texts())
	}

	if err := h.tt.Click("Layers"); err != nil {
		t.Fatal(err)
	}
	h.settle("layers", func() bool { _, ok := h.tt.Find("Layer cta"); return ok })
	h.tt.Advance(time.Second) // the panel slid in
	if err := h.tt.Click("Layer cta"); err != nil {
		t.Fatal(err)
	}
	h.settle("cta's HTML", func() bool { return h.tt.HasText("Apply") })
	shot(t, h.tt, "canvas-mockup-layers.png")
	if err := h.tt.Click("Comments"); err != nil {
		t.Fatal(err)
	}
	h.tt.Advance(time.Second)
	h.settle("comments", func() bool {
		return h.tt.HasText("No comments yet. Select an element and say what to change: an agent asked to work on the comments reads them.")
	})
	if err := h.tt.Click("New comment"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("Make it green")
	h.tt.Frame()
	if err := h.tt.Click("Add comment"); err != nil {
		t.Fatal(err)
	}
	h.settle("the comment on the host", func() bool {
		list, _ := h.cl.CanvasComments(cv.ID)
		return len(list) == 1 && list[0].Element == "cta" && list[0].Text == "Make it green"
	})
	h.settle("the comment shown", func() bool { return h.tt.HasText("Make it green") && h.tt.HasText("#cta") })
	shot(t, h.tt, "canvas-comments.png")

	if err := h.tt.Click("Versions"); err != nil {
		t.Fatal(err)
	}
	h.settle("versions", func() bool { return h.tt.HasText("Version 4") && h.tt.HasText("Version 1") })
	if err := h.tt.Click("Compare v2 with the one shown"); err != nil {
		t.Fatal(err)
	}
	h.settle("the comparison", func() bool { return h.tt.HasText("Version 2 → 4") && h.tt.HasText("#cta") })
	shot(t, h.tt, "canvas-versions.png")
	if err := h.tt.Click("Restore v2"); err != nil {
		t.Fatal(err)
	}
	h.settle("restored", func() bool {
		got, _ := h.cl.Canvas(cv.ID)
		return got.Version == 5 && !strings.Contains(got.Frames[0].HTML, "cta")
	})
}

// The user makes a board from the New dialog, adds a sticky from the
// palette and writes on it in place.
func TestCanvasMadeAndEditedByTheUser(t *testing.T) {
	h := newCanvasHarness(t)
	if err := h.tt.Click("New canvas"); err != nil {
		t.Fatal(err)
	}
	h.settle("the dialog", func() bool { return h.tt.HasText("New canvas") })
	h.tt.Type("Retro")
	if err := h.tt.Click("Board"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.Click("Create canvas"); err != nil {
		t.Fatal(err)
	}
	var id string
	h.settle("the board", func() bool {
		list, _ := h.cl.Canvases()
		if len(list) == 1 && list[0].Kind == proto.CanvasBoard && list[0].Title == "Retro" {
			id = list[0].ID
		}
		return id != "" && h.tt.HasText("Design 1 of 1")
	})
	if err := h.tt.Click("Add sticky note"); err != nil {
		t.Fatal(err)
	}
	h.settle("the sticky", func() bool { return h.tt.HasText("Note") })
	f, _ := h.tt.Find("sticky-1")
	h.tt.ClickAt(f.X+f.W/2, f.Y+f.H/2)
	h.tt.ClickAt(f.X+f.W/2, f.Y+f.H/2)
	h.settle("editing", func() bool { return h.tt.Focused("Text of sticky-1") })
	for range 4 {
		h.tt.Key(0, ui.KeyBackspace)
	}
	h.tt.Type("Ship weekly")
	h.tt.Key(0, ui.KeyEnter)
	h.settle("the text on the host", func() bool {
		got, _ := h.cl.Canvas(id)
		return len(got.Frames[0].Elements) == 1 && got.Frames[0].Elements[0].Text == "Ship weekly"
	})
	h.settle("the text shown", func() bool { return h.tt.HasText("Ship weekly") })
	h.tt.Advance(time.Second)
	h.tt.Frame()
	shot(t, h.tt, "canvas-board.png")
}

// The user inserts a component into a mockup from the Insert menu.
func TestCanvasComponentInserted(t *testing.T) {
	h := newCanvasHarness(t)
	cv, _ := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Home", Kind: proto.CanvasMockup})
	h.settle("the canvas open", func() bool { return h.app.canvases.Selected() == cv.ID && h.tt.HasText("Insert") })
	if err := h.tt.Click("Insert"); err != nil {
		t.Fatal(err)
	}
	h.settle("the menu", func() bool { return h.tt.HasText("Navbar") })
	if err := h.tt.Click("Card"); err != nil {
		t.Fatal(err)
	}
	h.settle("the card in the mockup", func() bool {
		got, _ := h.cl.Canvas(cv.ID)
		return strings.Contains(got.Frames[0].HTML, `<article id="card"`) && h.tt.HasText("#card")
	})
}
