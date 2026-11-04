package canvasview

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// Build: the dialog starts one with what was asked; the panel follows
// its stages and directions, and in Compare a direction is picked with
// what to keep and change.
func TestBuildAPage(t *testing.T) {
	cv := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1", Title: "Ledger", Kind: proto.CanvasMockup},
		Frames: []proto.CanvasFrame{
			{ID: "f1", Name: "Current", Width: 1440, Height: 900, HTML: `<main>Now</main>`},
			{ID: "f2", Name: "Direction A", Width: 1440, Height: 900, HTML: `<main>A</main>`,
				Direction: &proto.DesignDirection{NorthStar: "A ledger you trust", Source: "ruled paper", Skeleton: "type", Type: "Fraunces", Palette: []string{"#f4efe6", "#c2410c"}}},
			{ID: "f3", Name: "Direction B", Width: 1440, Height: 900, HTML: `<main>B</main>`},
		}}
	var started *proto.CanvasBuildParams
	var picked []string
	o := Options{
		CanAsk:   true,
		Load:     func(id string, done func(proto.Canvas, error)) { done(cv, nil) },
		Comments: func(id string, done func([]proto.CanvasComment, error)) { done(nil, nil) },
		Edit:     func(e proto.CanvasEdit, done func(proto.Canvas, error)) { done(cv, nil) },
		Build:    func(p proto.CanvasBuildParams, image []byte) { started = &p },
		PickDirection: func(cv proto.Canvas, frame, keep, change string) {
			picked = []string{frame, keep, change}
		},
	}
	v := &View{}
	v.Open(o, "c1")
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1280).Height(800).TextColor(theme.Text).Background(theme.Background).Children(func() { v.Draw(c, o) })
	}, 1280, 800)
	tt.Frame()
	if err := tt.Click("Build a UI"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	v.building.what = "A pricing page"
	v.building.count = 2
	tt.Frame()
	shot(t, tt, "build-dialog.png")
	if err := tt.Click("Start the build"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if started == nil || started.What != "A pricing page" || started.Directions != 3 || started.From != proto.BuildNew || len(started.Devices) != 2 {
		t.Fatalf("started: %+v", started)
	}

	o.BuildRun = &proto.CanvasBuildRun{ID: "c1", What: "A pricing page", Stage: proto.BuildCompare, Want: 2,
		Directions: []proto.BuildDirection{{Frame: "f2", Done: true}, {Frame: "f3", Done: true}},
		Pairs:      []proto.DirectionPair{{A: "f2", B: "f3", Distinct: 34, Shared: []string{"skeleton", "palette"}}}}
	tt.Frame()
	shot(t, tt, "build-panel.png")
	if !tt.HasText("Compare") || !tt.HasText("ruled paper") || !tt.HasText("A ↔ B · 34 distinct · shares skeleton, palette") {
		t.Fatalf("the build's panel: %q", tt.Texts())
	}
	v.CompareDesigns(o, "c1", []string{"f2", "f1", "f3"})
	tt.Frame()
	shot(t, tt, "build-compare.png")
	if !tt.HasText("1 Current · baseline") || !tt.HasText("From ruled paper") {
		t.Fatalf("compare: %q", tt.Texts())
	}
	if err := tt.Click("Pick Direction A"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	v.cmp.pick.keep, v.cmp.pick.change = "the type", "a warmer accent"
	tt.Frame()
	shot(t, tt, "build-pick.png")
	if err := tt.Click("Pick it, for the agent to refine"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(picked) != 3 || picked[0] != "f2" || picked[1] != "the type" || picked[2] != "a warmer accent" {
		t.Fatalf("picked: %v", picked)
	}
}

// The panel closes at any stage, and a long review stays inside the window, scrolling.
func TestBuildPanelClosesAndFits(t *testing.T) {
	cv := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1", Title: "Ledger", Kind: proto.CanvasMockup},
		Frames: []proto.CanvasFrame{{ID: "f1", Name: "One", Width: 1440, Height: 900, HTML: `<main>A</main>`}, {ID: "f2", Name: "Two", Width: 1440, Height: 900, HTML: `<main>B</main>`}}}
	o := Options{Load: func(id string, done func(proto.Canvas, error)) { done(cv, nil) },
		Comments: func(id string, done func([]proto.CanvasComment, error)) { done(nil, nil) },
		Edit:     func(e proto.CanvasEdit, done func(proto.Canvas, error)) { done(cv, nil) },
		Canvases: []proto.CanvasInfo{cv.CanvasInfo}}
	var issues []string
	for range 60 {
		issues = append(issues, "Polish: a long note of what the review found, that wraps over a few lines of the panel")
	}
	o.BuildRun = &proto.CanvasBuildRun{ID: "c1", What: "A pricing page", Stage: proto.BuildReviewing,
		Review: &proto.DesignReview{Round: 1, Done: true, Score: 6, Issues: issues, Keep: "the last words"}}
	v := &View{}
	v.Open(o, "c1")
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(900).Height(500).TextColor(theme.Text).Background(theme.Background).Children(func() {
			v.Draw(c, o)
		})
	}, 900, 500)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("Independent review · round 1") {
		t.Fatalf("no review: %q", tt.Texts())
	}
	head, ok := tt.Find("Independent review · round 1")
	if !ok || head.Y < 0 {
		t.Fatalf("its review is off the window: %+v, %v", head, ok)
	}
	if head, ok := tt.Find("Build"); !ok || head.Y < 0 {
		t.Fatalf("its head, with Close, is off the window: %+v, %v", head, ok)
	}
	if err := tt.Click("Close"); err != nil {
		t.Fatalf("no Close while it runs: %v", err)
	}
	tt.Frame()
	if tt.HasText("Independent review · round 1") {
		t.Fatal("still shown")
	}
}
