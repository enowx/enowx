package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The sidebar is one column: the host at its top, the pages as tabs, the
// list of the page shown. On Canvas it lists the canvases and the page
// leaves its own list out; the sessions at work stay in sight.
func TestOneSidebar(t *testing.T) {
	h := newCanvasHarness(t)
	app := h.app
	app.state.RailWide = true
	if _, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Ledger pricing", Kind: proto.CanvasMockup}); err != nil {
		t.Fatal(err)
	}
	app.sessions = []proto.SessionInfo{{ID: "s1", Name: "Sinkronisasi branch payroll", Agent: "claude", State: proto.StateWorking}}
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1280).Height(800).Background(theme.Ground).TextColor(theme.Text).Children(func() {
			app.titleBar(c)
			ui.Row(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				app.navSidebar(c)
				ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() { app.canvasPage(c) })
			})
		})
	}, 1280, 800)
	h.settle("the canvas in the sidebar", func() bool {
		_, ok := h.tt.Find("Host: this Mac, " + func() string { _, s := app.connectionState(); return s }())
		return ok && h.tt.HasText("Ledger pricing") && h.tt.HasText("At work") && h.tt.HasText("Sinkronisasi branch payroll")
	})
	shot(t, h.tt, "nav-canvas.png")
	n := 0
	for _, x := range h.tt.Texts() {
		if x == "Ledger pricing" {
			n++
		}
	}
	if n > 2 { // the list and the page's title: not the page's own list too
		t.Fatalf("the canvas shows %d times: %q", n, h.tt.Texts())
	}
	if err := h.tt.Click("Sessions"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if app.page != "" || h.tt.HasText("At work") {
		t.Fatalf("on Sessions: page %q", app.page)
	}
	if err := h.tt.Click("Notes"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if app.page != pageNotes {
		t.Fatalf("on Notes: page %q", app.page)
	}
}
