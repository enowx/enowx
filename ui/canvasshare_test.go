package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// The user shares a canvas from Export: the dialog shows its link, to
// copy; Stop sharing takes it away on the host.
func TestCanvasShareLinkDialog(t *testing.T) {
	h := newCanvasHarness(t)
	cv, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Landing", Kind: proto.CanvasDiagram})
	if err != nil {
		t.Fatal(err)
	}
	h.settle("the canvas open", func() bool { return h.app.canvases.Selected() == cv.ID && h.tt.HasText("Export") })
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatalf("%v; shows %q", err, h.tt.Texts())
		}
		h.tt.Frame()
	}
	click("Export")
	click("Share a link…")
	h.settle("the link", func() bool {
		for _, s := range h.tt.Texts() {
			if strings.HasPrefix(s, "http://127.0.0.1:") && strings.Contains(s, "/c/") {
				return true
			}
		}
		return false
	})
	if os.Getenv("SHOTS") != "" {
		// The whole window, where the dialog shows over the page.
		full := ui.NewTester(func(c *ui.Context) { h.app.view(c) }, 1280, 800)
		for range 30 {
			full.Frame()
			time.Sleep(10 * time.Millisecond)
		}
		shot(t, full, "canvas-share.png")
	}
	if list, _ := h.cl.CanvasShares(); len(list) != 1 || list[0].Canvas != cv.ID {
		t.Fatalf("the host's links: %+v", list)
	}
	click("Copy")
	h.settle("copied", func() bool { return h.tt.HasText("Copied") })
	click("Stop sharing")
	// It asks first: the link holds until the user says yes.
	q := h.app.confirming
	if q == nil {
		t.Fatal("stopped sharing without asking")
	}
	if list, _ := h.cl.CanvasShares(); len(list) != 1 {
		t.Fatalf("stopped before the answer: %+v", list)
	}
	h.app.confirming = nil
	q.do()
	h.settle("the dialog gone", func() bool { return !h.tt.HasText("Share a link") })
	if list, _ := h.cl.CanvasShares(); len(list) != 0 {
		t.Fatalf("still shared: %+v", list)
	}
}
