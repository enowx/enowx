package ui

import (
	"net"
	"strings"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// A watched host's render request is answered through that host's connection, not the
// one of the host shown: here, with no web view, by saying so.
func TestWatchedHostsRenderRequestGoesBackToIt(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	dial := func(on func(proto.Event)) *client.Client {
		a, b := net.Pipe()
		go srv.ServeConn(a)
		c, err := client.New(b, on)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	app := New(Options{})
	app.onMain = func(fn func()) { fn() }
	w := &hostWatch{host: hosts.Host{ID: "server", Name: "server"}}
	w.cl = dial(func(e proto.Event) {
		if e.Name == proto.EventCanvasRender {
			app.watchEvent(w, e)
		}
	})
	if err := w.cl.ServeCanvasRenders(); err != nil {
		t.Fatal(err)
	}
	caller := dial(nil)
	cv, err := caller.CreateCanvas(proto.CanvasCreateParams{Title: "Page", Kind: proto.CanvasMockup})
	if err != nil {
		t.Fatal(err)
	}
	cv, err = caller.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLPatch, HTML: `<p id="a">hi</p>`})
	if err != nil {
		t.Fatal(err)
	}
	_, err = caller.RenderCanvas(proto.CanvasRenderParams{ID: cv.ID, Frame: cv.Frames[0].ID, Format: proto.RenderPNG})
	if err == nil || !strings.Contains(err.Error(), "no web view") {
		t.Fatalf("the app's answer: %v", err)
	}
}
