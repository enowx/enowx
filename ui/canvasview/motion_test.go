package canvasview

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// orderedPage keeps every script run in it, and answers takeGoto's question.
type orderedPage struct {
	livePage
	mu     sync.Mutex
	loaded []string
	evals  []string
	answer string
	block  chan struct{}
}

func (p *orderedPage) LoadHTML(html string) { p.loaded = append(p.loaded, html) }
func (p *orderedPage) Eval(code string) (any, error) {
	if p.block != nil && strings.Contains(code, "enowx.motion.") {
		<-p.block
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.evals = append(p.evals, code)
	return p.answer, nil
}

func (p *orderedPage) ran(want string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.evals {
		if strings.Contains(e, want) {
			return true
		}
	}
	return false
}

// motionStage is a canvas open on html, its page reporting motion (JSON).
func motionStage(t *testing.T, html, motion string) (*View, *Web, *orderedPage, func(int), *ui.Tester) {
	t.Helper()
	cv := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1", Kind: proto.CanvasMockup}, Frames: []proto.CanvasFrame{
		{ID: "f1", Name: "Scene", Width: 800, Height: 600, HTML: html},
	}}
	page := &orderedPage{}
	var later []func()
	var mu sync.Mutex
	web := &Web{NewPage: func(done func(Page, error)) { done(page, nil) }, Update: func(fn func()) {
		mu.Lock()
		defer mu.Unlock()
		later = append(later, fn)
	}}
	o := Options{
		Load:     func(id string, done func(proto.Canvas, error)) { done(cv, nil) },
		Comments: func(id string, done func([]proto.CanvasComment, error)) { done(nil, nil) },
		Web:      web,
	}
	v := &View{}
	v.Open(o, "c1")
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1280).Height(800).Children(func() { v.Draw(c, o) })
	}, 1280, 800)
	frames := func(n int) {
		for range n {
			page.mu.Lock()
			page.answer = `{"t":"","s":"","l":` + strconv.Itoa(web.loads) + `,"m":` + motion + `}`
			page.mu.Unlock()
			tt.Frame()
			time.Sleep(2 * time.Millisecond)
			mu.Lock()
			fns := later
			later = nil
			mu.Unlock()
			for _, fn := range fns {
				fn()
			}
		}
	}
	frames(3)
	return v, web, page, frames, tt
}

// settleControls draws until the page's answer came back and was taken.
func settleControls(w *Web, frames func(int)) {
	for range 200 {
		frames(1)
		w.polled = time.Time{}
		if w.motion != nil {
			frames(2)
			return
		}
	}
}

// Interact's bar shows what the page reports, and sends what the user does to
// its runtime; Edit has no bar.
func TestInteractMotionBar(t *testing.T) {
	v, web, page, frames, tt := motionStage(t, `<style>@keyframes a{}</style>`, `{"d":8000,"t":2000,"p":true,"n":1}`)
	if _, ok := tt.Find("Pause"); ok {
		t.Fatal("Edit has motion's controls")
	}
	v.mode = modeInteract
	settleControls(web, frames)
	if _, ok := tt.Find("Move through the animation"); !ok {
		t.Fatal("no scrubber once the page reported motion")
	}
	tt.Click("Pause")
	if web.motion.P {
		t.Fatal("the bar still says playing")
	}
	for range 100 {
		tt.Frame()
		if page.ran("enowx.motion.pause()") {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !page.ran("enowx.motion.pause()") {
		t.Fatal("Pause didn't reach the page")
	}
	frames(1)
	tt.Click("Restart")
	for range 100 {
		frames(1)
		if page.ran("enowx.motion.restart()") {
			break
		}
	}
	if !page.ran("enowx.motion.restart()") {
		t.Fatal("Restart didn't reach the page")
	}
}

// A page with nothing to move, or no runtime, shows no controls.
func TestNoMotionNoBar(t *testing.T) {
	for _, motion := range []string{"null", `{"d":0,"t":0,"p":false,"n":0}`} {
		v, web, _, frames, tt := motionStage(t, `<p>plain</p>`, motion)
		v.mode = modeInteract
		for range 30 {
			frames(1)
			web.polled = time.Time{}
		}
		if _, ok := tt.Find("Move through the animation"); ok {
			t.Fatalf("%s: controls with nothing to move", motion)
		}
	}
}

// A scrub sends the last place it reached once the page has answered, not every
// place it passed, each one paused so it holds its picture.
func TestScrubSendsTheLatest(t *testing.T) {
	page := &orderedPage{block: make(chan struct{})}
	var later []func()
	var mu sync.Mutex
	w := &Web{page: page, Update: func(fn func()) {
		mu.Lock()
		defer mu.Unlock()
		later = append(later, fn)
	}}
	w.motion = &pageMotion{D: 8000}
	w.seekMotion(1000)
	w.seekMotion(2000)
	w.seekMotion(3000)
	close(page.block)
	for range 400 {
		mu.Lock()
		fns := later
		later = nil
		mu.Unlock()
		for _, fn := range fns {
			fn()
		}
		if page.ran("seek(3000)") {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !page.ran("seek(1000)") || !page.ran("seek(3000)") || page.ran("seek(2000)") {
		t.Fatalf("sent %q", page.evals)
	}
	if !page.ran("enowx.motion.pause();enowx.motion.seek(3000)") {
		t.Fatal("a scrub should hold still")
	}
}

// The board shows a moving design held; Interact lets it play. An element's
// script reaches the page by loading it, as outerHTML doesn't run scripts.
func TestBoardHoldsMotion(t *testing.T) {
	page := &orderedPage{}
	w := &Web{page: page, Update: func(fn func()) { fn() }}
	cv := proto.Canvas{CanvasInfo: proto.CanvasInfo{ID: "c1"}}
	f := proto.CanvasFrame{ID: "f1", Width: 100, Height: 100, HTML: `<div id="a"></div><style>@keyframes a{}</style>`}
	w.setDesign(true)
	w.load(cv, f)
	if !strings.Contains(page.loaded[len(page.loaded)-1], "window.__enowxMotionHold=1") {
		t.Fatal("the board's page plays")
	}
	w.design = false
	w.load(cv, f)
	if strings.Contains(page.loaded[len(page.loaded)-1], "window.__enowxMotionHold=1") {
		t.Fatal("Interact's page is held")
	}
	loads := len(page.loaded)
	f2 := f
	f2.HTML = `<div id="a"><script>x()</script></div><style>@keyframes a{}</style>`
	w.apply(cv, f2, nil, proto.CanvasChange{Frame: "f1", Element: "a", Op: proto.CanvasOpHTMLPatch})
	if len(page.loaded) != loads+1 {
		t.Fatal("a patch with a script didn't load the page anew")
	}
}
