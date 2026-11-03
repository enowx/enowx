//go:build !windows

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// The Canvas page in the whole app, at a small, a common and a large
// window, in each of its states: a design with the strip, the grid of
// designs, two compared, the layout grid's panel, the agent beside it, and
// the New canvas dialog. Nothing squeezed, nothing out of the window.
func TestCanvasLayoutAudit(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h := newCanvasHarness(t)
	h.app.agents = []proto.AgentInfo{{ID: "claude", Name: "Claude Code", Command: "claude", Installed: true}}
	h.app.state.RailWide = true
	cv, _ := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Landing pages for the spring launch", Kind: proto.CanvasMockup})
	for n := 2; n <= 4; n++ {
		h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpFrameAdd, FrameInfo: &proto.CanvasFrame{Name: fmt.Sprintf("Hero variant %d", n), Page: "Home", Variant: string(rune('A' + n)), Description: "Big type, one call to action, a product shot"}})
	}
	g := proto.CanvasGrid{Cols: 12, Rows: 6, ColGap: 24, RowGap: 24, Margin: 48, Snap: true, Areas: []proto.GridArea{{Name: "header", Col: 1, Row: 1, ColSpan: 12}}}
	h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpFrameGrid, Grid: &g})
	h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLPatch, HTML: `<main id="page"><h1 id="title">Spring launch</h1></main>`})
	h.settle("the canvas open", func() bool {
		c := h.app.canvases.Canvas()
		return c != nil && len(c.Frames) == 4 && c.Frames[0].Grid != nil
	})

	states := []struct {
		name string
		set  func()
	}{
		{"design", func() {}},
		{"grid-panel", func() { h.app.canvases.TogglePanel(h.app.canvasOptions(), "grid") }},
		{"designs-grid", func() { h.app.canvases.SetView(1) }},
		{"compare", func() {
			c := h.app.canvases.Canvas()
			h.app.canvases.CompareDesigns(h.app.canvasOptions(), c.ID, []string{c.Frames[1].ID, c.Frames[2].ID})
		}},
		{"agent", func() { h.app.canvases.SetView(0); h.app.canvases.OpenAssistant() }},
		{"agent-panel", func() {
			h.app.state.Panel.Open, h.app.state.Panel.Tab, h.app.panelPage = true, panelFiles, pageCanvas
		}},
		{"panel", func() { h.app.canvases.CloseAssistant() }},
		{"new-canvas", func() {
			h.app.state.Panel.Open, h.app.panelPage = false, ""
			h.app.canvases.CloseAssistant()
			h.app.canvases.NewCanvas()
		}},
	}
	shots := os.Getenv("SHOTS")
	var problems []string
	for _, st := range states {
		st.set()
		for _, size := range drawSizes(auditSizes) {
			w, hgt := size[0], size[1]
			h.tt = auditTester(h.app, w, hgt)
			for range 6 {
				h.settle("", func() bool { return true })
			}
			h.tt.Advance(time.Second)
			h.tt.Frame()
			name := fmt.Sprintf("canvas-%s-%dx%d", st.name, w, hgt)
			if shots != "" {
				shot(t, h.tt, filepath.Join("audit", name+".png"))
			}
			for _, pr := range layoutProblems(h.tt, float32(w), float32(hgt), true) {
				problems = append(problems, name+": "+pr)
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("%d layout problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}
