package canvasview

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// recordingPage keeps what it loads and answers as a page would.
type recordingPage struct {
	livePage
	loaded []string
	answer string
}

func (p *recordingPage) LoadHTML(html string)     { p.loaded = append(p.loaded, html) }
func (p *recordingPage) Eval(string) (any, error) { return p.answer, nil }
func (p *recordingPage) last() string             { return p.loaded[len(p.loaded)-1] }

// What a prototype's page leaves goes to the page a link opens, in Interact
// alone, and an old page's answer doesn't undo a Reset.
func TestPrototypeStateCarriesAcrossPages(t *testing.T) {
	page := &recordingPage{}
	w := &Web{page: page, Update: func(fn func()) { fn() }}
	cv := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1"}}
	home := proto.CanvasFrame{ID: "f1", Width: 100, Height: 100, HTML: `<button data-set="plan=pro">Pro</button>`}
	pay := proto.CanvasFrame{ID: "f2", Width: 100, Height: 100, HTML: `<b data-bind="plan"></b>`}
	poll := func(state, target string, load int) string {
		got := ""
		page.answer = `{"t":"` + target + `","s":` + jsonString(state) + `,"l":` + strconv.Itoa(load) + `}`
		w.polled = time.Time{}
		tt := ui.NewTester(func(c *ui.Context) { w.pollGoto(c, func(t string) { got = t }) }, 100, 100)
		tt.Frame()
		for range 200 {
			if !w.polling {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		return got
	}

	w.load(cv, home)
	if strings.Contains(page.last(), "window.__enowxSeed=") {
		t.Fatal("a first page was given state")
	}
	if got := poll(`{"plan":"pro"}`, "design-2", w.loads); got != "design-2" {
		t.Fatalf("link went to %q", got)
	}
	w.load(cv, pay)
	if !strings.Contains(page.last(), `window.__enowxSeed={"plan":"pro"}`) {
		t.Fatal("the next page didn't get what the first left")
	}

	w.setDesign(true)
	if strings.Contains(page.last(), "window.__enowxSeed=") {
		t.Fatal("the board shows a state, not the design as it starts")
	}
	w.setDesign(false)
	if !strings.Contains(page.last(), "window.__enowxSeed=") {
		t.Fatal("back in Interact, its state is gone")
	}

	loads := w.loads
	w.ResetState()
	w.load(cv, home)
	if strings.Contains(page.last(), "window.__enowxSeed=") {
		t.Fatal("Reset left state")
	}
	poll(`{"plan":"pro"}`, "", loads)
	if w.state != "" {
		t.Fatalf("the old page's state came back: %s", w.state)
	}

	other := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c2"}}
	w.state = `{"x":"y"}`
	w.load(other, home)
	if strings.Contains(page.last(), "window.__enowxSeed=") {
		t.Fatal("state went to another canvas")
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Interact has a Reset, which forgets the state and returns to the first
// design; Edit doesn't.
func TestInteractResetsThePrototype(t *testing.T) {
	cv := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1", Kind: proto.CanvasMockup}, Frames: []proto.CanvasFrame{
		{ID: "f1", Name: "Home", Width: 800, Height: 600, HTML: `<button data-set="a=b">x</button>`},
		{ID: "f2", Name: "Pay", Width: 800, Height: 600, HTML: `<b data-bind="a"></b>`},
	}}
	page := &recordingPage{answer: `{}`}
	var later []func()
	o := Options{
		Load:     func(id string, done func(proto.Canvas, error)) { done(cv, nil) },
		Comments: func(id string, done func([]proto.CanvasComment, error)) { done(nil, nil) },
		Web:      &Web{NewPage: func(done func(Page, error)) { done(page, nil) }, Update: func(fn func()) { later = append(later, fn) }},
	}
	v := &View{}
	v.Open(o, "c1")
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1280).Height(800).Children(func() { v.Draw(c, o) })
	}, 1280, 800)
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
	frames(3)
	const reset = "Reset the prototype: its state, and back to the first design"
	if _, ok := tt.Find(reset); ok {
		t.Fatal("Edit has a Reset")
	}
	v.mode, v.frame = modeInteract, "f2"
	o.Web.state, o.Web.stateOf = `{"a":"b"}`, "c1"
	frames(3)
	tt.Click(reset)
	frames(4)
	if v.frame != "f1" || o.Web.state != "" {
		t.Fatalf("after Reset: on %s, state %q", v.frame, o.Web.state)
	}
	if strings.Contains(page.last(), "window.__enowxSeed=") {
		t.Fatal("the page was loaded with the old state")
	}
}

// Moving between designs keeps the zoom the user set, whatever their sizes,
// both ways, with the new design in view; a canvas's first design fits, and
// coming back to a canvas finds its zoom.
func TestCameraKeepsTheZoom(t *testing.T) {
	var cam camera
	cam.place("c1", "a", "a/1440", 1200, 1440)
	first := cam.z
	if first >= 1 {
		t.Fatalf("a canvas's first design should fit: %v", first)
	}
	cam.z = 1 // the user's 100%
	cam.place("c1", "b", "b/390", 1200, 390)
	if cam.z != 1 || cam.x != (1200-390)/2 || cam.y != 40 {
		t.Fatalf("narrow design: %+v", cam)
	}
	cam.place("c1", "a", "a/1440", 1200, 1440)
	if cam.z != 1 {
		t.Fatalf("back: %v", cam.z)
	}
	if cam.x != (1200-1440)/2 {
		t.Fatalf("wider than the stage, it should stay centered: %v", cam.x)
	}
	cam.place("c1", "a", "a/768", 1200, 768) // a preview's width
	if cam.z != 1 && cam.z != min((1200-96)/float32(768), 1) {
		t.Fatalf("preview: %v", cam.z)
	}
	cam.z = 0.7
	cam.place("c2", "x", "x/1440", 1200, 1440)
	if cam.z == 0.7 {
		t.Fatal("another canvas fits, the first time")
	}
	cam.place("c1", "a", "a/1440", 1200, 1440)
	if cam.z != 0.7 {
		t.Fatalf("coming back: %v", cam.z)
	}
}

// Interact mode's box: whole and centered with its margin, at a small, default
// and large window, for a desktop and a phone design; never above 100%.
func TestInteractBox(t *testing.T) {
	for _, tc := range []struct{ aw, ah, w, h float32 }{
		{700, 400, 1440, 900}, {1200, 700, 1440, 900}, {2400, 1300, 1440, 900},
		{700, 400, 390, 844}, {1200, 700, 390, 844}, {2400, 1300, 390, 844},
	} {
		b := interactBox(tc.aw, tc.ah, tc.w, tc.h)
		if b.X < interactMargin || b.Y < interactMargin || b.X+b.W > tc.aw-interactMargin+0.01 || b.Y+b.H > tc.ah-interactMargin+0.01 {
			t.Errorf("%+v: %+v touches the stage's edge", tc, b)
		}
		if b.Zoom > 1 || b.Zoom < liveZoom {
			t.Errorf("%+v: zoom %v", tc, b.Zoom)
		}
		if b.Zoom > liveZoom && (b.W/b.H-tc.w/tc.h > 0.01 || tc.w/tc.h-b.W/b.H > 0.01) {
			t.Errorf("%+v: lost its proportions: %+v", tc, b)
		}
	}
	if b := interactBox(2400, 1300, 1440, 900); b.W != 1440 || b.H != 900 {
		t.Errorf("large window: %+v", b)
	}
}
