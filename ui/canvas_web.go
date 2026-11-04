package ui

import (
	"errors"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/canvasview"
)

// Mockups show in web views of this device: the Canvas page's, and one
// made for a moment, behind the window's content, to render a frame for
// an agent (canvas screenshot) or an export.

// canvasPage is a web view as canvasview uses it.
type canvasPage struct {
	wv   *mygo.WebView
	diag *diag.Log
}

func (p canvasPage) Show(c *ui.Context) ui.Element { return ui.WebView(c, p.wv) }
func (p canvasPage) LoadHTML(html string)          { p.wv.Page().LoadHTML(html, designOrigin()) }
func (p canvasPage) Eval(code string) (any, error) { return p.wv.Page().Eval(code) }
func (p canvasPage) SetZoom(f float64)             { p.wv.Page().SetZoomFactor(f) }
func (p canvasPage) Close() {
	p.diag.Event("webview.destroyed", "use", "canvas")
	p.wv.Destroy()
}

// canvasWeb makes the Canvas page's web view of mockups; nil without one.
func (a *App) canvasWeb() *canvasview.Web {
	if !webViewAvailable() || a.win == nil {
		return nil
	}
	return &canvasview.Web{
		NewPage: func(done func(canvasview.Page, error)) {
			win := a.win
			go func() {
				wv, err := win.NewWebView(mygo.WebViewOptions{})
				a.onMain(func() {
					if err != nil {
						done(nil, err)
						return
					}
					a.diag.Event("webview.created", "use", "canvas")
					done(canvasPage{wv, a.diag}, nil)
				})
			}()
		},
		Asset: func(id, name string) ([]byte, error) {
			cl := a.client
			if cl == nil {
				return nil, errors.New("not connected")
			}
			return cl.CanvasAsset(id, name)
		},
		Update: a.onMain,
	}
}

// motionPaint is what a page takes to paint a moment it was just moved to.
const motionPaint = 100 * time.Millisecond

// renders runs one render at a time.
var renders sync.Mutex

// renderCanvas renders req's page and replies to the daemon that asked, through cl.
func (a *App) renderCanvas(cl *client.Client, req proto.CanvasRenderRequest) {
	win := a.win
	if cl == nil {
		return
	}
	wider, text := 0, ""
	var motion *proto.CanvasMotion
	reply := func(data []byte, err error) {
		out := proto.CanvasRenderReply{ID: req.ID, Data: data, Wider: wider, Text: text, Motion: motion}
		if err != nil {
			out.Error = err.Error()
		}
		if err := cl.CanvasRenderReply(out); errors.Is(err, proto.ErrTooLarge) {
			// A picture this connection can't carry: the daemon's call ends now, not at its timeout.
			_ = cl.CanvasRenderReply(proto.CanvasRenderReply{ID: req.ID, Error: "canvas: the picture is too large to send: " + err.Error()})
		}
	}
	if !webViewAvailable() || win == nil {
		go reply(nil, errors.New("this device has no web view to render HTML"))
		return
	}
	go func() {
		renders.Lock()
		defer renders.Unlock()
		if r, ok := renderKept(req); ok {
			wider, text, motion = r.wider, r.text, r.motion
			reply(r.data, r.err)
			return
		}
		// A hidden window of its own at the page's size: a web view in the
		// main window flashed over its content, and wasn't laid out while
		// the window was behind others.
		a.diag.Every("webview.render", webViewLogEvery, "webview.render", "width", req.Width, "height", req.Height, "pageBytes", len(req.HTML))
		hidden := mygo.NewWindow(mygo.WindowOptions{Hidden: true, Width: req.Width, Height: req.Height, UseContentSize: true,
			Frameless: true, SkipTaskbar: true, BackgroundColor: "#ffffff"})
		r := renderIn(hidden.Page(), hidden.CapturePage, req)
		if req.Keep != "" && r.err == nil && r.motion != nil {
			keepRender(req, hidden)
		} else {
			hidden.Destroy()
		}
		wider, text, motion = r.wider, r.text, r.motion
		reply(r.data, r.err)
	}()
}

// loadRenderPage loads html in pg, and waits for it to be laid out.
func loadRenderPage(pg *mygo.Page, html string) error {
	loaded := make(chan struct{}, 1)
	off := pg.OnDidFinishLoad(func() {
		select {
		case loaded <- struct{}{}:
		default:
		}
	})
	defer off()
	pg.LoadHTML(html, designOrigin())
	select {
	case <-loaded:
	case <-time.After(20 * time.Second):
		return errors.New("the page didn't load")
	}
	time.Sleep(400 * time.Millisecond) // fonts and images settle
	// A web view is laid out only once its window draws it: until then
	// the page measures at no width.
	for try := 0; try < 15; try++ {
		if v, err := pg.Eval("window.innerWidth"); err == nil {
			if n, ok := v.(float64); ok && n > 0 {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("the web view gave no picture: it was never laid out")
}

// keptFor is how long a page rendered for a run of moments waits for the next.
const keptFor = 15 * time.Second

// keptRender is the page of a run of renders of one design at several moments (a
// design's frames): loading a large page for each is what ran past the time a render
// gets. renders guards it.
var keptPage struct {
	key   string
	win   *mygo.Window
	timer *time.Timer
}

// keepRender keeps win for run req.Keep, and the one before goes.
func keepRender(req proto.CanvasRenderRequest, win *mygo.Window) {
	k := &keptPage
	dropKept()
	k.key, k.win = req.Keep, win
	k.timer = time.AfterFunc(keptFor, func() {
		renders.Lock()
		defer renders.Unlock()
		if k.key == req.Keep {
			dropKept()
		}
	})
}

// dropKept lets go of the page kept. The caller holds renders.
func dropKept() {
	k := &keptPage
	if k.win == nil {
		return
	}
	k.timer.Stop()
	k.win.Destroy()
	k.key, k.win, k.timer = "", nil, nil
}

// renderKept renders req on the page kept for its run, moved to its moment; false when
// there is none, for it to be loaded.
func renderKept(req proto.CanvasRenderRequest) (rendered, bool) {
	k := &keptPage
	if req.Keep == "" || req.At == "" || k.win == nil || k.key != req.Keep {
		return rendered{}, false
	}
	script, err := canvas.SeekScript(req.At)
	if err != nil {
		return rendered{err: err}, true
	}
	k.timer.Reset(keptFor)
	r := rendered{laidOut: true}
	v, err := k.win.Page().Eval(script)
	if err != nil {
		return rendered{err: err}, true
	}
	r.motion = canvas.ParseMotion(evalText(v))
	time.Sleep(motionPaint)
	r.data, r.err = capturePainted(k.win.CapturePage)
	return r, true
}

// capturePainted captures a web view, which may not be painted at the first try.
func capturePainted(capture func() ([]byte, error)) ([]byte, error) {
	png, err := capture()
	for try := 0; try < 3 && (err != nil || len(png) == 0); try++ {
		time.Sleep(500 * time.Millisecond)
		png, err = capture()
	}
	if err == nil && len(png) == 0 {
		err = errors.New("the web view gave no picture")
	}
	return png, err
}

// rendered is a page's render: its picture or PDF, how much wider than
// its frame it is, its script's value, the motion it stands in, whether it was
// laid out at all.
type rendered struct {
	data    []byte
	wider   int
	text    string
	motion  *proto.CanvasMotion
	laidOut bool
	err     error
}

// renderIn loads req's page in pg, then measures it, runs its script, or
// captures it with capture.
func renderIn(pg *mygo.Page, capture func() ([]byte, error), req proto.CanvasRenderRequest) rendered {
	var r rendered
	if err := loadRenderPage(pg, req.HTML); err != nil {
		r.err = err
		return r
	}
	r.laidOut = true
	// A moving page shows one moment, the same each time, whatever the load took.
	if v, err := pg.Eval(canvas.SettleScript); err == nil {
		if r.motion = canvas.ParseMotion(evalText(v)); r.motion != nil {
			time.Sleep(motionPaint)
		}
	}
	if v, err := pg.Eval(canvasview.WiderScript); err == nil {
		if n, ok := v.(float64); ok && n > 0 {
			r.wider = int(n)
		}
	}
	if req.Script != "" {
		v, err := pg.Eval(req.Script)
		if err == nil {
			r.text = evalText(v)
		}
		r.err = err
		return r
	}
	if req.Format == proto.RenderPDF {
		size := mygo.PageSize{Width: float64(req.Width) / 96, Height: float64(req.Height) / 96}
		r.data, r.err = pg.PrintToPDF(mygo.PDFOptions{PageSize: size, Margins: &mygo.Margins{}, Background: true})
		return r
	}
	r.data, r.err = capturePainted(capture)
	return r
}
