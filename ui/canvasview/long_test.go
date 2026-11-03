package canvasview

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/framecheck"
)

// longPage is about eleven megabytes of HTML: an element with a line of a megabyte, and
// ten more of rows.
func longPage() string {
	return `<main id="top"><p id="one">` + framecheck.Line() + `</p>` + strings.Repeat("<p>row</p>", 1<<20) + `</main>`
}

// offMain runs work off the main thread, what follows it waiting in queue.
func offMain(queue chan func()) func(work, then func()) {
	return func(work, then func()) {
		go func() {
			work()
			queue <- then
		}()
	}
}

func waitFor(t *testing.T, queue chan func()) {
	t.Helper()
	select {
	case fn := <-queue:
		fn()
	case <-time.After(60 * time.Second):
		t.Fatal("what was asked for off the main thread never came")
	}
}

// Previews of long designs don't read their whole HTML each frame.
func TestLongPreviewsStayQuick(t *testing.T) {
	page := longPage()
	var frames []proto.CanvasFrame
	for _, id := range []string{"f1", "f2", "f3", "f4", "f5", "f6"} {
		frames = append(frames, proto.CanvasFrame{ID: id, Name: "Home", Width: 1440, Height: 900, HTML: page})
	}
	v := &View{canvas: &proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1"}, Frames: frames}}
	o := Options{Render: func(id, frame, format string, done func([]byte, error)) {}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Column(c).Children(func() {
			for _, f := range frames {
				v.designPreview(c.Key(f.ID), o, f, 40)
			}
		})
	}, 400, 800)
	framecheck.Quick(t, "previews", tt)
}

// The layers of a long page, an element of a megabyte selected, are parsed once and stay
// quick; that element is too long to edit there, and says so.
func TestLongLayersStayQuick(t *testing.T) {
	queue := make(chan func(), 16)
	v := &View{frame: "f1", sel: "one", canvas: &proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1"},
		Frames: []proto.CanvasFrame{{ID: "f1", Name: "Home", Width: 1440, Height: 900, HTML: longPage()}}}}
	o := Options{OffMain: offMain(queue)}
	tt := ui.NewTester(func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Scroll(c).Width(320).Height(700).Children(func() {
			ui.Column(c).FillWidth().Children(func() { v.layers(c, o) })
		})
	}, 320, 700)
	tt.Image()
	waitFor(t, queue)
	framecheck.Quick(t, "layers", tt)
	if len(v.outline) < 2 {
		t.Errorf("the outline: %+v", len(v.outline))
	}
	if !tt.HasText("Too long to edit here") {
		t.Errorf("no word that it's too long: %q", tt.Texts())
	}
}

// The whole page with a long design open, in the stage this device has without a web view.
func TestLongCanvasStaysQuick(t *testing.T) {
	queue := make(chan func(), 16)
	cv := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1", Title: "Long", Kind: proto.CanvasMockup},
		Frames: []proto.CanvasFrame{{ID: "f1", Name: "Home", Width: 1440, Height: 900, HTML: longPage()}}}
	o := Options{
		OffMain:  offMain(queue),
		Load:     func(id string, done func(proto.Canvas, error)) { done(cv, nil) },
		Comments: func(id string, done func([]proto.CanvasComment, error)) { done(nil, nil) },
		Edit:     func(e proto.CanvasEdit, done func(proto.Canvas, error)) { done(cv, nil) },
	}
	v := &View{}
	v.Open(o, "c1")
	tt := ui.NewTester(func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Column(c).Width(1280).Height(800).Children(func() { v.Draw(c, o) })
	}, 1280, 800)
	tt.Image()
	select { // when it shows the page's elements
	case fn := <-queue:
		fn()
	case <-time.After(15 * time.Second):
	}
	framecheck.Quick(t, "canvas", tt)
}
