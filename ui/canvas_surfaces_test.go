//go:build !windows

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// canvasSurface is a state of the Canvas page to look at: in reaches it from
// the page, out leaves it again.
type canvasSurface struct {
	name    string
	in, out func()
}

// settled lets what's on its way arrive, and what moves finish moving.
func (h *canvasHarness) settled() {
	for range 6 {
		h.tt.Frame()
		time.Sleep(10 * time.Millisecond)
		for _, fn := range h.later {
			fn()
		}
		h.later = nil
	}
	h.tt.Advance(time.Second)
	h.tt.Frame()
}

// Every menu, dialog, panel and card of the Canvas page, reached as the user
// reaches them, in the whole window at each size: everything wrapped by its
// parent, nothing squeezed, nothing out of the window.
func TestCanvasSurfacesAudit(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h := newCanvasHarness(t)
	h.app.agents = []proto.AgentInfo{{ID: "claude", Name: "Claude Code", Command: "claude", Installed: true}}

	// No canvas yet: the list says so and the page offers one.
	var problems []string
	for _, size := range auditSizes {
		h.tt = ui.NewTester(func(c *ui.Context) { h.app.view(c) }, size[0], size[1])
		h.settled()
		if !h.tt.HasText("No canvases yet") || !h.tt.HasText("No canvas open") {
			t.Fatalf("empty: %q", h.tt.Texts())
		}
		for _, pr := range layoutProblems(h.tt, float32(size[0]), float32(size[1]), true) {
			problems = append(problems, fmt.Sprintf("empty-%dx%d: %s", size[0], size[1], pr))
		}
	}

	long := "Landing pages for the spring launch of the whole product line, every market"
	cv, _ := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: long, Kind: proto.CanvasMockup, Group: "Launch"})
	h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLPatch, HTML: `<main id="page"><h1 id="title">Spring launch</h1></main>`})
	for n := 2; n <= 3; n++ {
		h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpFrameAdd, FrameInfo: &proto.CanvasFrame{Name: fmt.Sprintf("Hero variant %d with a long name", n), Page: "Home", Variant: "B", Description: "Big type, one call to action, a product shot and a long line on why"}})
	}
	got, _ := h.cl.Canvas(cv.ID)
	shapes := got.Frames[1].ID
	for _, el := range []proto.CanvasElement{
		{ID: "box", Type: proto.ShapeRect, X: 60, Y: 60, W: 300, H: 120, Text: "A box"},
		{ID: "pic", Type: proto.ShapeImage, X: 60, Y: 300, W: 300, H: 120, Src: "hero.png"},
		{ID: "link", Type: proto.ShapeArrow, From: "box", To: "pic"},
	} {
		h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Frame: shapes, Op: proto.CanvasOpElementSet, Element: &el})
	}
	for _, text := range []string{"The headline could be a lot bigger on a phone, and say what the launch is about", "Keep the dot"} {
		h.cl.CommentCanvas(proto.CanvasCommentParams{ID: cv.ID, Op: proto.CanvasCommentAdd, Comment: proto.CanvasComment{Text: text, Element: "title", Frame: got.Frames[0].ID}})
	}
	h.settle("the canvas open", func() bool { c := h.app.canvases.Canvas(); return c != nil && len(c.Frames) == 3 })

	// Under 620 of room the toolbar's words turn to icons, labeled at length.
	short := map[string]string{"Insert": "Insert a component", "Interact": "Interact: use the mockup as the real thing", "Edit": "Interact: use the mockup as the real thing"}
	click := func(label string) {
		t.Helper()
		if _, ok := h.tt.Find(label); !ok && short[label] != "" {
			label = short[label]
		}
		if err := h.tt.Click(label); err != nil {
			all := h.tt.Texts()
			t.Fatalf("%v; ends with %q", err, all[max(len(all)-30, 0):])
		}
		h.tt.Frame()
	}
	settled := h.settled
	clicks := func(labels ...string) func() {
		return func() {
			for _, l := range labels {
				click(l)
				settled()
			}
		}
	}
	esc := func() { h.tt.Key(0, ui.KeyEscape); h.tt.Frame() }
	cancel := func() { // Esc may have closed the dialog with its menu
		if _, ok := h.tt.Find("Cancel"); ok {
			click("Cancel")
		}
	}
	showDesign := func(n int) func() { return clicks(fmt.Sprintf("Design %d", n)) }
	first := showDesign(1)
	var surfaces []canvasSurface
	add := func(name string, in, out func()) { surfaces = append(surfaces, canvasSurface{name, in, out}) }

	for _, m := range [][2]string{
		{"export-menu", "Export"}, {"make-code-menu", "Make code"}, {"more-menu", "More for this canvas"},
		{"add-design-menu", "Add a design"}, {"insert-menu", "Insert"}, {"preview-menu", "Preview width: Its own width"},
		{"design-menu", "Design Desktop"},
	} {
		add(m[0], clicks(m[1]), esc)
	}
	add("strip-menu", func() {
		if err := h.tt.RightClick("Design 2"); err != nil {
			t.Fatal(err)
		}
		settled()
	}, esc)
	add("row-menu", func() {
		if err := h.tt.RightClick(long); err != nil {
			t.Fatal(err)
		}
		settled()
	}, esc)
	add("layers", clicks("Design 2"), first)
	add("shape-inspector", clicks("Design 2", "box"), first)
	add("arrow-inspector", clicks("Design 2", "Layer link"), first)
	add("image-inspector", clicks("Design 2", "Layer pic"), first)
	add("html-inspector", clicks("Layer title"), func() { esc(); first() })
	add("comments", clicks("Comments 2"), clicks("Layers"))
	add("versions", clicks("Versions"), clicks("Layers"))
	add("versions-compare", clicks("Versions", "Compare v1 with the one shown"), clicks("Layers"))
	add("grid-none", clicks("Design 3", "Grid"), func() { click("Layers"); first() })
	add("shape-text-edit", func() {
		showDesign(2)()
		r, _ := h.tt.Find("box")
		h.tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		h.tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		settled()
		if !h.tt.Focused("Text of box") {
			t.Fatal("not editing the shape's text")
		}
	}, func() { esc(); first() })
	add("interact", clicks("Interact"), clicks("Edit"))
	add("new-dialog", clicks("New canvas"), cancel)
	add("new-kind-menu", clicks("New canvas", "Mockup"), func() { esc(); cancel() })
	add("rename-dialog", clicks("More for this canvas", "Rename…"), cancel)
	add("delete-confirm", clicks("More for this canvas", "Delete…"), cancel)
	add("describe-dialog", clicks("Design Desktop", "Name and describe…"), cancel)
	add("share-dialog", clicks("Share"), func() { esc(); settled() })
	add("brief-sheet", clicks("Brief: what these designs are for"), func() { esc(); settled() })
	add("build-sheet", clicks("Build a UI"), cancel)
	add("unslop-passes", clicks("Unslop with the passes you pick…"), cancel)
	add("agent-intro", clicks("Ask an agent"), clicks("Close the agent"))
	add("agent-menu", clicks("Ask an agent", "Agent"), func() { esc(); click("Close the agent") })
	add("agent-history", clicks("Ask an agent", "Messages and their versions (0)"), func() { click("Messages and their versions (0)"); click("Close the agent") })

	// A build under way, a direction done and one too close to it.
	add("build-hud", func() {
		c := h.app.canvases.Canvas()
		h.app.buildRuns = map[string]proto.CanvasBuildRun{c.ID: {ID: c.ID, What: "A settings page for notifications", From: "brief", Want: 2, Stage: proto.BuildCompare,
			Directions: []proto.BuildDirection{{Frame: c.Frames[0].ID, Done: true}, {Frame: c.Frames[1].ID, Session: "s1"}},
			Pairs:      []proto.DirectionPair{{A: c.Frames[0].ID, B: c.Frames[1].ID, Shared: []string{"type", "color"}, Alike: 9, Distinct: 1}},
			Review:     &proto.DesignReview{Round: 2, Session: "s2", Score: 7, Issues: []string{"Hero: the type is too small on a phone → it reads at arm's length"}, Keep: "The one bright frame", Done: true}}}
		settled()
	}, func() { h.app.buildRuns = nil })
	add("compare-build", func() {
		c := h.app.canvases.Canvas()
		h.app.buildRuns = map[string]proto.CanvasBuildRun{c.ID: {ID: c.ID, What: "A settings page", From: "brief", Want: 2, Stage: proto.BuildCompare,
			Directions: []proto.BuildDirection{{Frame: c.Frames[0].ID, Done: true}, {Frame: c.Frames[1].ID, Done: true}},
			Pairs:      []proto.DirectionPair{{A: c.Frames[0].ID, B: c.Frames[1].ID, Alike: 1, Distinct: 8}}}}
		h.app.canvases.CompareDesigns(h.app.canvasOptions(), c.ID, []string{c.Frames[0].ID, c.Frames[1].ID})
		settled()
	}, func() { h.app.buildRuns = nil; h.app.canvases.SetView(0) })
	add("pick-sheet", func() {
		c := h.app.canvases.Canvas()
		h.app.buildRuns = map[string]proto.CanvasBuildRun{c.ID: {ID: c.ID, What: "A settings page", From: "brief", Want: 2, Stage: proto.BuildCompare,
			Directions: []proto.BuildDirection{{Frame: c.Frames[0].ID, Done: true}, {Frame: c.Frames[1].ID, Done: true}}}}
		h.app.canvases.CompareDesigns(h.app.canvasOptions(), c.ID, []string{c.Frames[0].ID, c.Frames[1].ID})
		settled()
		click("Pick Desktop")
		settled()
	}, func() { cancel(); h.app.buildRuns = nil; h.app.canvases.SetView(0) })
	add("build-starting", func() { h.app.buildStarting = true; settled() }, func() { h.app.buildStarting = false })
	add("build-error", func() {
		h.app.buildErr = "The agent couldn't start: claude isn't installed on this host, or it isn't on its path"
		settled()
	}, func() { h.app.buildErr = "" })
	add("unslop-starting", func() { h.app.unslopping = true; settled() }, func() { h.app.unslopping = false })
	add("unslop-error", func() {
		h.app.unslopErr = "The design couldn't be copied: the canvas changed while it was read, try again"
		settled()
	}, func() { h.app.unslopErr = "" })
	add("agent-bar", func() {
		c := h.app.canvases.Canvas()
		el := proto.CanvasElement{ID: "box", Type: proto.ShapeRect, X: 60, Y: 60, W: 300, H: 120, Text: "A box"}
		h.cl.EditCanvas(proto.CanvasEdit{ID: c.ID, Frame: shapes, Op: proto.CanvasOpElementSet, Element: &el, Session: "agent-1"})
		settled()
	}, func() {})
	add("agent-paused", func() {
		h.cl.ControlCanvas(h.app.canvases.Canvas().ID, proto.CanvasPause)
		settled()
	}, func() {
		h.cl.ControlCanvas(h.app.canvases.Canvas().ID, proto.CanvasResume)
		settled()
	})

	shots := os.Getenv("SHOTS")
	look := func(name string, w, hgt int) {
		if shots != "" && w == 1280 {
			shot(t, h.tt, filepath.Join("surfaces", name+".png"))
		}
		for _, pr := range layoutProblems(h.tt, float32(w), float32(hgt), true) {
			problems = append(problems, fmt.Sprintf("%s-%dx%d: %s", name, w, hgt, pr))
		}
	}
	for _, size := range auditSizes {
		w, hgt := size[0], size[1]
		if shots != "" && w != 1280 {
			continue // pictures are slow: of one size
		}
		h.tt = ui.NewTester(func(c *ui.Context) { h.app.view(c) }, w, hgt)
		settled()
		for _, s := range surfaces {
			if only := os.Getenv("SURFACE"); only != "" && !strings.Contains(only, s.name) {
				continue
			}
			s.in()
			settled()
			look(s.name, w, hgt)
			s.out()
			settled()
		}
	}
	if len(problems) > 0 {
		t.Errorf("%d layout problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// A redesign's panel, its before and after and its replay in the whole window.
func TestUnslopSurfacesAudit(t *testing.T) {
	h := newCanvasHarness(t)
	cv, _ := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Dashboard for the whole team, every week and every market", Kind: proto.CanvasMockup})
	set := func(frame, html string) {
		h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, Frame: frame, HTML: html})
	}
	set(cv.Frames[0].ID, `<style>.c{padding:13px}</style><aside id="side">Home</aside><main id="main"><h1 id="t">Welcome back!!</h1></main>`)
	h.settle("the canvas open", func() bool { return h.app.canvases.Selected() == cv.ID })
	h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpBrief, Brief: &proto.DesignBrief{Product: "A team's dashboard, with what each lead needs each morning", Users: "Leads", Tone: "Calm, dense", FirstScreen: "This week"}})
	u, err := h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Start: true})
	if err != nil {
		t.Fatal(err)
	}
	h.app.canvases.Goto(h.app.canvasOptions(), cv.ID, u.Frame)

	h.settle("the copy open", func() bool { return h.app.canvases.Canvas() != nil })
	shots := os.Getenv("SHOTS")
	var problems []string
	look := func(name string) {
		if only := os.Getenv("SURFACE"); only != "" && !strings.Contains(only, name) {
			return
		}
		for _, size := range auditSizes {
			w, hgt := size[0], size[1]
			if shots != "" && w != 1280 {
				continue // pictures are slow: of one size
			}
			h.tt = ui.NewTester(func(c *ui.Context) { h.app.view(c) }, w, hgt)
			h.settled()
			if shots != "" && w == 1280 {
				shot(t, h.tt, filepath.Join("surfaces", name+".png"))
			}
			for _, pr := range layoutProblems(h.tt, float32(w), float32(hgt), true) {
				problems = append(problems, fmt.Sprintf("%s-%dx%d: %s", name, w, hgt, pr))
			}
		}
	}
	look("unslop-reading")
	h.settled()
	h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Frame: u.Frame, Pass: "1"})
	set(u.Frame, `<style>.c{padding:16px}</style><aside id="side">Home</aside><main id="main"><h1 id="t">Welcome back</h1></main>`)
	h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Frame: u.Frame, Pass: "3"})
	set(u.Frame, `<style>.c{padding:16px}</style><aside id="side">Home</aside><main id="main"><h1 id="t">This week</h1></main>`)
	look("unslop-passes")
	lead, err := h.cl.CreateSession(proto.SessionCreateParams{Agent: "shell", Name: "lead", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Frame: u.Frame, Regions: []string{"#side", "#main"}, Session: lead.ID})
	h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Frame: u.Frame, Region: "side", Pass: "7"})
	h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Frame: u.Frame, Region: "main", Pass: "done"})
	look("unslop-regions")
	h.settle("the regions", func() bool { r := h.app.unslopRunOf(nil); return r != nil && len(r.Regions) > 0 })
	h.app.viewRegionAgent(h.app.unslopRunOf(nil).Regions[0].Session)
	look("region-bar")
	if os.Getenv("SURFACE") == "" && !h.tt.HasText("Main agent") {
		t.Fatalf("no region bar: %q", h.tt.Texts())
	}
	h.app.canvases.CloseAssistant()
	h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Frame: u.Frame, Pass: "done"})
	look("unslop-done")

	run := h.app.unslopRunOf(nil)
	if run == nil {
		t.Fatal("no run")
	}
	h.app.compareCanvasUnslop(cv.ID, *run)
	look("unslop-compare-loading")
	h.app.unslopComparing = nil
	h.app.unslopComparing = &unslopCompare{err: "No pictures of this unslop were taken.", from: run.From, to: run.Score}
	look("unslop-compare-error")
	h.app.unslopComparing = nil

	h.app.openReplay(u.Frame, 1)
	look("unslop-replay")
	h.app.closeReplay()
	if len(problems) > 0 {
		t.Errorf("%d layout problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}
