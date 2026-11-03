package daemon

import (
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// bigDesign makes a canvas whose one mockup, with its picture written into its
// page, is a document over a frame's limit: as a design holding its assets is.
func bigDesign(t *testing.T, srv *Server, c *client.Client) proto.Canvas {
	t.Helper()
	cv, err := c.CreateCanvas(proto.CanvasCreateParams{Title: "Film", Kind: proto.CanvasMockup})
	if err != nil {
		t.Fatal(err)
	}
	name, err := srv.canvasFor(cv.ID).AddAsset(cv.ID, "film.png", make([]byte, 13<<20))
	if err != nil {
		t.Fatal(err)
	}
	cv, err = c.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLPatch, HTML: `<img id="film" src="assets/` + name + `">`})
	if err != nil {
		t.Fatal(err)
	}
	return cv
}

// Asking for the picture of a design too large for one frame, what the Canvas
// page does for the designs it shows, must leave the connection up.
func TestRenderOfLargeDesignKeepsConnection(t *testing.T) {
	srv := startServer(t)
	var app *client.Client
	asked := make(chan proto.CanvasRenderRequest, 1)
	app = connect(t, srv, func(e proto.Event) {
		var req proto.CanvasRenderRequest
		if e.Name == proto.EventCanvasRender && json.Unmarshal(e.Data, &req) == nil {
			asked <- req
		}
	})
	if err := app.ServeBrowsers(); err != nil {
		t.Fatal(err)
	}
	go func() {
		for req := range asked {
			_ = app.CanvasRenderReply(proto.CanvasRenderReply{ID: req.ID, Data: []byte(strings.Repeat("p", len(req.HTML)>>20))})
		}
	}()
	cv := bigDesign(t, srv, app)
	_, err := app.RenderCanvas(proto.CanvasRenderParams{ID: cv.ID, Frame: cv.Frames[0].ID, Format: proto.RenderPNG})
	select {
	case <-app.Done():
		t.Fatalf("the render closed the connection: render error %v, connection: %v", err, app.Err())
	case <-time.After(100 * time.Millisecond):
	}
	if err != nil {
		t.Fatalf("render: %v", err)
	}
}

// oldApp is an app from before parts: it says hello without them. It gives what
// it reads to frames until the connection ends.
func oldApp(t *testing.T, srv *Server) (net.Conn, chan proto.Frame) {
	t.Helper()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	t.Cleanup(func() { b.Close() })
	hello, _ := json.Marshal(proto.Hello{Protocol: proto.Version, Agent: "enowx/old"})
	if err := proto.WriteFrame(b, proto.Frame{Type: proto.TypeHello, Payload: hello}); err != nil {
		t.Fatal(err)
	}
	f, err := proto.ReadFrame(b)
	if err != nil || f.Type != proto.TypeHello {
		t.Fatalf("hello: %v", err)
	}
	frames := make(chan proto.Frame, 64)
	go func() {
		defer close(frames)
		for {
			f, err := proto.ReadFrame(b)
			if err != nil {
				return
			}
			frames <- f
		}
	}()
	return b, frames
}

// oldCall sends a request and waits for its answer, parts being a failure.
func oldCall(t *testing.T, nc net.Conn, frames chan proto.Frame, id uint64, method string, params any) proto.Response {
	t.Helper()
	raw, _ := json.Marshal(params)
	req, _ := json.Marshal(proto.Request{ID: id, Method: method, Params: raw})
	if err := proto.WriteFrame(nc, proto.Frame{Type: proto.TypeRequest, Payload: req}); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatal("the connection closed")
			}
			if f.Type == proto.TypePart {
				t.Fatal("an old app was sent parts")
			}
			var resp proto.Response
			if f.Type == proto.TypeResponse && json.Unmarshal(f.Payload, &resp) == nil && resp.ID == id {
				return resp
			}
		case <-time.After(10 * time.Second):
			t.Fatal("no answer")
		}
	}
}

// An app from before parts gets a clear error for that call, at once, and stays
// connected: it isn't left waiting for a render's timeout.
func TestLargeDesignFailsAloneForAnOldApp(t *testing.T) {
	srv := startServer(t)
	maker := connect(t, srv, nil)
	cv := bigDesign(t, srv, maker)
	nc, frames := oldApp(t, srv)
	if r := oldCall(t, nc, frames, 1, proto.MethodBrowserServe, struct{}{}); r.Error != "" {
		t.Fatal(r.Error)
	}
	start := time.Now()
	_, err := maker.RenderCanvas(proto.CanvasRenderParams{ID: cv.ID, Frame: cv.Frames[0].ID, Format: proto.RenderPNG})
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("render: %v", err)
	}
	if took := time.Since(start); took > canvasRenderTimeout/2 {
		t.Fatalf("waited %s for an answer that couldn't come", took)
	}
	if r := oldCall(t, nc, frames, 2, proto.MethodDaemonInfo, struct{}{}); r.Error != "" {
		t.Fatal(r.Error)
	}
}

// A canvas over a frame's limit can be written and read, its change reaches apps
// whole, and an old app connected beside them misses the change but not the
// connection.
func TestCanvasOverAFrameTravelsInParts(t *testing.T) {
	srv := startServer(t)
	changed := make(chan proto.CanvasChange, 4)
	app := connect(t, srv, func(e proto.Event) {
		var ch proto.CanvasChange
		if e.Name == proto.EventCanvasChanged && json.Unmarshal(e.Data, &ch) == nil {
			changed <- ch
		}
	})
	nc, frames := oldApp(t, srv)
	cv, err := app.CreateCanvas(proto.CanvasCreateParams{Title: "Site", Kind: proto.CanvasMockup})
	if err != nil {
		t.Fatal(err)
	}
	html := `<div id="site">` + strings.Repeat("x", 20<<20) + `</div>`
	if _, err := app.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLPatch, HTML: html}); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case ch := <-changed:
		if len(ch.Canvas.Frames[0].HTML) < 20<<20 {
			t.Fatalf("the change holds %d bytes", len(ch.Canvas.Frames[0].HTML))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no change told")
	}
	got, err := app.Canvas(cv.ID)
	if err != nil || len(got.Frames[0].HTML) < 20<<20 {
		t.Fatalf("read: %d bytes, %v", len(got.Frames[0].HTML), err)
	}
	if r := oldCall(t, nc, frames, 1, proto.MethodCanvasGet, proto.CanvasRef{ID: cv.ID}); !strings.Contains(r.Error, "frame limit") {
		t.Fatalf("an old app's read: %q", r.Error)
	}
	if r := oldCall(t, nc, frames, 2, proto.MethodDaemonInfo, struct{}{}); r.Error != "" {
		t.Fatal(r.Error)
	}
}

// A picture too large for one frame gets back to the daemon that asked for it.
func TestLargePictureReachesTheDaemon(t *testing.T) {
	srv := startServer(t)
	var app *client.Client
	const size = 20 << 20
	app = connect(t, srv, func(e proto.Event) {
		var req proto.CanvasRenderRequest
		if e.Name == proto.EventCanvasRender && json.Unmarshal(e.Data, &req) == nil {
			go func() { _ = app.CanvasRenderReply(proto.CanvasRenderReply{ID: req.ID, Data: make([]byte, size)}) }()
		}
	})
	if err := app.ServeBrowsers(); err != nil {
		t.Fatal(err)
	}
	cv := bigDesign(t, srv, app)
	out, err := app.RenderCanvas(proto.CanvasRenderParams{ID: cv.ID, Frame: cv.Frames[0].ID, Format: proto.RenderPNG})
	if err != nil || len(out.Data) != size {
		t.Fatalf("%d bytes, %v", len(out.Data), err)
	}
}

// What can't go at all is refused for that frame alone.
func TestFrameOverEveryLimitIsRefused(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	huge := make([]byte, proto.MaxMessage+1)
	if err := c.Call(proto.MethodDaemonInfo, huge, nil); !errors.Is(err, proto.ErrTooLarge) {
		t.Fatalf("call: %v", err)
	}
	if _, err := c.Info(); err != nil {
		t.Fatalf("the connection: %v", err)
	}
}
