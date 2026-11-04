package canvasview

import (
	"bytes"
	"image"
	imgcolor "image/color"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// livePage stands in for a web view, labeled so a test finds it.
type livePage struct{}

func (livePage) Show(c *ui.Context) ui.Element { return ui.Box(c).Label("Live page") }
func (livePage) LoadHTML(string)               {}
func (livePage) Eval(string) (any, error)      { return nil, nil }
func (livePage) SetZoom(float64)               {}
func (livePage) Close()                        {}

// A mockup zoomed out shows as its picture, rendered at its own width, so
// its content shrinks with its frame; zoomed in, the live page shows; a
// click on the picture zooms in to the page.
func TestMockupShrinksWholeWhenZoomedOut(t *testing.T) {
	cv := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1", Title: "Shop", Kind: proto.CanvasMockup},
		Frames: []proto.CanvasFrame{{ID: "f1", Name: "Home", Width: 1440, Height: 900, HTML: `<main id="hero">Hi</main>`}}}
	var later []func()
	rendered := 0
	o := Options{
		Load:     func(id string, done func(proto.Canvas, error)) { done(cv, nil) },
		Comments: func(id string, done func([]proto.CanvasComment, error)) { done(nil, nil) },
		Render: func(id, frame, format string, done func([]byte, error)) {
			rendered++
			img := image.NewRGBA(image.Rect(0, 0, 1440, 900))
			for i := range img.Pix {
				img.Pix[i] = 255
			}
			img.Set(10, 10, imgcolor.Black)
			var b bytes.Buffer
			png.Encode(&b, img)
			later = append(later, func() { done(b.Bytes(), nil) })
		},
		Web: &Web{NewPage: func(done func(Page, error)) { done(livePage{}, nil) }, Update: func(fn func()) { later = append(later, fn) }},
	}
	v := &View{}
	v.Open(o, "c1")
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1800).Height(800).Children(func() { v.Draw(c, o) })
	}, 1800, 800)
	frames := func(n int) {
		for range n {
			tt.Frame()
			fns := later
			later = nil
			for _, fn := range fns {
				fn()
			}
		}
	}
	frames(4)
	if _, ok := tt.Find("Live page"); !ok || v.cam.z < liveZoom {
		t.Fatalf("at %v the live page should show: %q", v.cam.z, tt.Texts())
	}
	v.cam.zoomAt(0, 0, 0.2/v.cam.z)
	frames(4)
	if _, ok := tt.Find("Picture of Home"); !ok || rendered == 0 {
		t.Fatalf("zoomed out to %v: no picture (rendered %d): %q", v.cam.z, rendered, tt.Texts())
	}
	if _, ok := tt.Find("Live page"); ok {
		t.Fatal("the live page shows zoomed out, laid out anew")
	}
	d, _ := tt.Find("Design Home")
	tt.Press(d.X+d.W/2, d.Y+d.H/2)
	tt.Release(d.X+d.W/2, d.Y+d.H/2)
	frames(3)
	if v.cam.z < liveZoom-0.001 {
		t.Fatalf("a click on the picture left the zoom at %v", v.cam.z)
	}
	if _, ok := tt.Find("Live page"); !ok {
		t.Fatalf("zoomed back in, no live page: %q", tt.Texts())
	}
}

// What failed shows over the page until dismissed, its words kept to the bar.
func TestErrorBarStaysInItsFrame(t *testing.T) {
	long := "The canvas couldn't be saved: the host's disk is full, or the folder it keeps canvases in can't be written to by the daemon any more, whichever came first"
	v := &View{selected: "c1", frame: "f1", err: long, canvas: &proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1", Title: "Shop", Kind: proto.CanvasWireframe},
		Frames: []proto.CanvasFrame{{ID: "f1", Name: "Home", Width: 1440, Height: 900}}}}
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, Options{Loaded: true}) }, 1000, 700)
	tt.Frame()
	r, ok := tt.Find(long)
	if !ok {
		t.Fatalf("no error: %q", tt.Texts())
	}
	if r.X+r.W > 1000 {
		t.Fatalf("out of the window: %+v", r)
	}
	if err := tt.Click("Dismiss"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if v.err != "" {
		t.Fatal("not dismissed")
	}
}

type closingPage struct {
	livePage
	closed *int
}

func (p closingPage) Close() { *p.closed++ }

// Left a while, the page's web view goes, and comes back on drawing again.
func TestUnloadClosesTheWebViewAndDrawingBringsItBack(t *testing.T) {
	cv := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1", Title: "Shop", Kind: proto.CanvasMockup},
		Frames: []proto.CanvasFrame{{ID: "f1", Name: "Home", Width: 400, Height: 300, HTML: `<main id="hero">Hi</main>`}}}
	closed, made := 0, 0
	o := Options{
		Load:     func(id string, done func(proto.Canvas, error)) { done(cv, nil) },
		Comments: func(id string, done func([]proto.CanvasComment, error)) { done(nil, nil) },
		Web: &Web{NewPage: func(done func(Page, error)) {
			made++
			done(closingPage{closed: &closed}, nil)
		}, Update: func(fn func()) { fn() }},
	}
	v := &View{}
	v.web = o.Web
	v.Open(o, "c1")
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1800).Height(800).Children(func() { v.Draw(c, o) })
	}, 1800, 800)
	for range 4 {
		tt.Frame()
	}
	if made != 1 || o.Web.page == nil {
		t.Fatalf("made %d pages", made)
	}
	v.Unload()
	if closed != 1 || o.Web.page != nil {
		t.Fatalf("closed %d", closed)
	}
	for range 4 {
		tt.Frame()
	}
	if made != 2 || o.Web.page == nil || o.Web.pending == "" {
		t.Fatalf("made %d, pending %d bytes", made, len(o.Web.pending))
	}
}
