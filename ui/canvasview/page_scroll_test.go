package canvasview

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// The designs' strip stays where the user scrolled it as an agent starts
// and stops changing the canvas (its bar showing over the page).
func TestStripKeepsItsScrollAsAnAgentWorks(t *testing.T) {
	cv := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1", Title: "Shop", Kind: proto.CanvasWireframe}}
	for i := range 12 {
		cv.Frames = append(cv.Frames, proto.CanvasFrame{ID: fmt.Sprint("f", i), Name: fmt.Sprint("Screen ", i+1), Width: 390, Height: 844})
	}
	o := Options{
		Load:     func(id string, done func(proto.Canvas, error)) { done(cv, nil) },
		Comments: func(id string, done func([]proto.CanvasComment, error)) { done(nil, nil) },
	}
	v := &View{}
	v.Open(o, "c1")
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1280).Height(800).Children(func() { v.Draw(c, o) })
	}, 1280, 800)
	for range 3 {
		tt.Frame()
	}
	first, ok := tt.Find("Design 1")
	if !ok {
		t.Fatalf("no strip: %q", tt.Texts())
	}
	start, _ := tt.Find("Design 10")
	for range 20 {
		tt.Scroll(first.X+first.W/2, first.Y+first.H/2, 300, 0)
		tt.Frame()
	}
	before, _ := tt.Find("Design 10")
	if before.X == start.X {
		t.Fatal("the strip didn't scroll")
	}
	v.canvas.Agent, v.canvas.AgentAt = "claude", time.Now() // its bar shows
	tt.Frame()
	tt.Frame()
	v.canvas.Agent = "" // and goes
	tt.Frame()
	tt.Frame()
	if after, _ := tt.Find("Design 10"); after.X != before.X {
		t.Fatalf("the strip went from %v to %v as the agent's bar came and went", before.X, after.X)
	}
}
