package daemon

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// An agent's changes to a canvas reach every app one element at a time,
// marked as the agent's; paused, the agent's changes are refused and the
// user's still made.
func TestCanvasChangesStreamToEveryApp(t *testing.T) {
	srv := startServer(t)
	changes := make(chan proto.CanvasChange, 16)
	connect(t, srv, func(e proto.Event) {
		var ch proto.CanvasChange
		if e.Name == proto.EventCanvasChanged && json.Unmarshal(e.Data, &ch) == nil {
			changes <- ch
		}
	})
	c := connect(t, srv, nil)
	cv, err := c.CreateCanvas(proto.CanvasCreateParams{Title: "Signup", Kind: proto.CanvasMockup})
	if err != nil {
		t.Fatal(err)
	}
	next := func() proto.CanvasChange {
		t.Helper()
		select {
		case ch := <-changes:
			return ch
		case <-time.After(5 * time.Second):
			t.Fatal("no canvas event")
		}
		return proto.CanvasChange{}
	}
	for _, part := range []string{`<form id="signup"></form>`, `<input id="email">`, `<button id="go">Sign up</button>`} {
		e := proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLPatch, HTML: part, Session: "agent-1"}
		if part != `<form id="signup"></form>` {
			e.Into = "signup"
		}
		if _, err := c.EditCanvas(e); err != nil {
			t.Fatal(err)
		}
	}
	// Told at once, then at most so often: the latest holds what came between it and the last.
	if ch := next(); ch.Element != "signup" || ch.Session != "agent-1" || ch.Canvas.Agent != "agent-1" {
		t.Fatalf("change %q by %q, want signup", ch.Element, ch.Session)
	}
	last := next()
	if last.Element != "go" || last.Session != "agent-1" {
		t.Fatalf("change %q by %q, want go", last.Element, last.Session)
	}
	if html := last.Canvas.Frames[0].HTML; !strings.Contains(html, `id="email"`) || !strings.Contains(html, `id="go"`) {
		t.Fatalf("the latest change doesn't hold the earlier: %s", html)
	}
	if _, err := c.ControlCanvas(cv.ID, proto.CanvasPause); err != nil {
		t.Fatal(err)
	}
	if ch := next(); !ch.Canvas.Paused {
		t.Fatal("pause not told")
	}
	if _, err := c.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLRemove, Target: "go", Session: "agent-1"}); err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("agent's change while paused: %v", err)
	}
	if _, err := c.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLText, Target: "go", HTML: "Join"}); err != nil {
		t.Fatalf("the user's change while paused: %v", err)
	}
	next()
	if _, err := c.ControlCanvas(cv.ID, proto.CanvasResume); err != nil {
		t.Fatal(err)
	}
	next()
	got, _ := c.Canvas(cv.ID)
	if got.Paused || !strings.Contains(got.Frames[0].HTML, `<button id="go">Join</button>`) {
		t.Fatalf("canvas %+v", got)
	}
}

// A frame of shapes renders in the daemon; one with HTML in the app
// serving Browsers, a device's width asked for.
func TestCanvasRenders(t *testing.T) {
	srv := startServer(t)
	var app *client.Client
	asked := make(chan proto.CanvasRenderRequest, 1)
	app = connect(t, srv, func(e proto.Event) {
		var req proto.CanvasRenderRequest
		if e.Name == proto.EventCanvasRender && json.Unmarshal(e.Data, &req) == nil {
			asked <- req
			go func() { _ = app.CanvasRenderReply(proto.CanvasRenderReply{ID: req.ID, Data: []byte("PNG!")}) }()
		}
	})
	c := connect(t, srv, nil)
	cv, _ := c.CreateCanvas(proto.CanvasCreateParams{Title: "Flow", Kind: proto.CanvasDiagram})
	box := proto.CanvasElement{Type: proto.ShapeRect, X: 10, Y: 10, W: 100, H: 50, Text: "API"}
	if _, err := c.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpElementSet, Element: &box}); err != nil {
		t.Fatal(err)
	}
	out, err := c.RenderCanvas(proto.CanvasRenderParams{ID: cv.ID})
	if err != nil || out.By != "daemon" || !bytes.HasPrefix(out.Data, []byte("\x89PNG")) || out.Width != 1600 {
		t.Fatalf("shapes: %v %+v", err, out.By)
	}
	html := "<p id=hi>Hello</p>"
	if _, err := c.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, HTML: html}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RenderCanvas(proto.CanvasRenderParams{ID: cv.ID}); err == nil || !strings.Contains(err.Error(), "app") {
		t.Fatalf("html without an app: %v", err)
	}
	if err := app.ServeBrowsers(); err != nil {
		t.Fatal(err)
	}
	out, err = c.RenderCanvas(proto.CanvasRenderParams{ID: cv.ID, Frame: "mobile"})
	if err != nil || out.By != "app" || string(out.Data) != "PNG!" || out.Width != 390 {
		t.Fatalf("html: %v %+v", err, out)
	}
	req := <-asked
	if req.Width != 390 || !strings.Contains(req.HTML, "Hello") || !strings.Contains(req.HTML, "API") {
		t.Fatalf("asked %+v", req)
	}
}
