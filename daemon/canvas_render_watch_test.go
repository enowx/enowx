package daemon

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
)

// htmlDesign is a canvas with a mockup, which only an app renders.
func htmlDesign(t *testing.T, c *client.Client) proto.Canvas {
	t.Helper()
	cv, err := c.CreateCanvas(proto.CanvasCreateParams{Title: "Page", Kind: proto.CanvasMockup})
	if err != nil {
		t.Fatal(err)
	}
	cv, err = c.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLPatch, HTML: `<p id="a">hi</p>`})
	if err != nil {
		t.Fatal(err)
	}
	return cv
}

// An app only watching this host renders its frames, though it serves no Browsers:
// what an agent on a server hit with the app open on another host. An app showing the
// host goes first. With none connected, the error says that.
func TestCanvasRendersThroughAnAppWatching(t *testing.T) {
	srv := startServer(t)
	caller := connect(t, srv, nil)
	cv := htmlDesign(t, caller)
	render := func() (proto.CanvasRendered, error) {
		return caller.RenderCanvas(proto.CanvasRenderParams{ID: cv.ID, Frame: cv.Frames[0].ID, Format: proto.RenderPNG})
	}
	if _, err := render(); err == nil || !strings.Contains(err.Error(), "no app is connected") {
		t.Fatalf("no app: %v", err)
	}
	app := func(picture string) *client.Client {
		var c *client.Client
		c = connect(t, srv, func(e proto.Event) {
			var req proto.CanvasRenderRequest
			if e.Name == proto.EventCanvasRender && json.Unmarshal(e.Data, &req) == nil {
				go func() { _ = c.CanvasRenderReply(proto.CanvasRenderReply{ID: req.ID, Data: []byte(picture)}) }()
			}
		})
		return c
	}
	watching := app("from the app watching")
	if err := watching.ServeCanvasRenders(); err != nil {
		t.Fatal(err)
	}
	got, err := render()
	if err != nil || string(got.Data) != "from the app watching" {
		t.Fatalf("watching: %q, %v", got.Data, err)
	}
	// It serves no Browsers: what an agent asks of one still says none is there.
	if err := caller.Call(proto.MethodBrowserAct, proto.BrowserAction{}, nil); err == nil || !strings.Contains(err.Error(), "no enowX app") {
		t.Fatalf("a Browser's action reached it: %v", err)
	}
	showing := app("from the app showing")
	if err := showing.ServeBrowsers(); err != nil {
		t.Fatal(err)
	}
	if got, err := render(); err != nil || string(got.Data) != "from the app showing" {
		t.Fatalf("showing: %q, %v", got.Data, err)
	}
	showing.Close()
	<-showing.Done()
	waitFor(t, "the app showing dropped", func() bool {
		got, err := render()
		return err == nil && string(got.Data) == "from the app watching"
	})
	watching.Close()
	<-watching.Done()
	waitFor(t, "no app left", func() bool {
		_, err := render()
		return err != nil && strings.Contains(err.Error(), canvas.ErrNeedsBrowser.Error())
	})
}
