package unslopview

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// A run's brief shows in a line, and in full once clicked.
func TestPanelShowsTheBrief(t *testing.T) {
	var p Panel
	run := &proto.CanvasUnslopRun{ID: "c", Frame: "f", From: 40, Score: 52, Scans: 2, Passes: []string{"01 Truth"},
		Brief: &proto.DesignBrief{Product: "A read-only API's docs", Users: "Developers wiring a client", Tone: "Calm, dense", FirstScreen: "The endpoints"}}
	tt := ui.NewTester(func(c *ui.Context) { p.Draw(c, run, PanelOptions{Reading: "Reading…"}) }, 900, 700)
	tt.Frame()
	if !tt.HasText("A read-only API's docs · Calm, dense") || tt.HasText("Developers wiring a client") {
		t.Fatalf("the brief in a line: %q", tt.Texts())
	}
	if err := tt.Click("The brief: what it is for"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	shot(t, tt, "panel-brief.png")
	if !tt.HasText("Developers wiring a client") || !tt.HasText("First screen") {
		t.Fatalf("the brief in full: %q", tt.Texts())
	}
}

// The independent review shows under way, its reviewer a click away, then
// its score and what it asks.
func TestPanelShowsTheReview(t *testing.T) {
	var p Panel
	opened := ""
	run := &proto.CanvasUnslopRun{ID: "c", Frame: "f", From: 40, Score: 90, Scans: 4, Done: true, Passes: []string{"01 Truth"},
		Review: &proto.DesignReview{Round: 1, Session: "rev1"}}
	tt := ui.NewTester(func(c *ui.Context) {
		p.Draw(c, run, PanelOptions{Reading: "Reading…", OpenReviewer: func(id string) { opened = id }})
	}, 900, 800)
	tt.Frame()
	if err := tt.Click("Show the reviewer"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	if tt.Frame(); opened != "rev1" {
		t.Fatalf("opened %q", opened)
	}
	run.Review = &proto.DesignReview{Round: 1, Session: "rev1", Done: true, Score: 6, Issues: []string{"hero: two CTAs → one"}, Keep: "the type"}
	tt.Frame()
	shot(t, tt, "panel-review.png")
	if !tt.HasText("6/10") || !tt.HasText("· hero: two CTAs → one") || !tt.HasText("Keep: the type") {
		t.Fatalf("the verdict: %q", tt.Texts())
	}
}

// A full run at a small window: the card stays inside it with its margin, its
// head in view, and the last agent's row is reached by scrolling inside the
// card, which leaves the scroll to it instead of the board below.
func TestPanelKeepsToItsRoom(t *testing.T) {
	var p Panel
	run := &proto.CanvasUnslopRun{ID: "c", Frame: "f", From: 40, Score: 52, Scans: 2}
	for i := range 12 {
		run.Passes = append(run.Passes, fmt.Sprintf("%02d Pass", i+1))
		run.Passed = append(run.Passed, proto.UnslopStep{Passes: []string{run.Passes[i]}})
	}
	for i := range 6 {
		run.Regions = append(run.Regions, proto.UnslopRegion{ID: fmt.Sprint("r", i), Session: fmt.Sprint("s", i)})
	}
	board := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Width(900).Height(420).HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputScroll {
				board++
			}
			return true
		}).Children(func() {
			p.Draw(c, run, PanelOptions{Reading: "Reading…", OpenSession: func(string) {}, Replay: func(int) {}})
		})
	}, 900, 420)
	tt.Frame()
	tt.Frame()
	last, ok := tt.Find("Show the agent of #r5")
	if !ok {
		t.Fatalf("no row: %q", tt.Texts())
	}
	if last.Y+last.H <= 420 {
		t.Fatalf("the rows should need scrolling at this size: row ends at %v", last.Y+last.H)
	}
	score, ok := tt.Find("/100")
	if !ok {
		t.Fatal("no score")
	}
	cx := float32(900 - 12 - 132)
	for range 30 {
		tt.Scroll(cx, 300, 0, 200)
		tt.Frame()
	}
	shot(t, tt, "panel-room.png")
	last, _ = tt.Find("Show the agent of #r5")
	if last.Y+last.H > 420-12 {
		t.Fatalf("the last row ends at %v: out of reach", last.Y+last.H)
	}
	if now, ok := tt.Find("/100"); !ok || now.Y != score.Y {
		t.Fatal("the head moved with the body")
	}
	if board != 0 {
		t.Fatalf("%d scrolls reached the board", board)
	}
	// New agents leave what's being read where it is.
	first, _ := tt.Find("Show the agent of #r4")
	run.Regions = append(run.Regions, proto.UnslopRegion{ID: "r6", Session: "s6"})
	tt.Frame()
	if now, _ := tt.Find("Show the agent of #r4"); now.Y != first.Y {
		t.Fatalf("a new agent moved the row from %v to %v", first.Y, now.Y)
	}
	// Folded, it leaves the design in view.
	if err := tt.Click("Fold this card to its head"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("Show the agent of #r4"); ok {
		t.Fatal("folded, the rows still show")
	}
}
