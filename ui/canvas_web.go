package ui

import (
	"crypto/rand"
	"errors"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/canvasview"
)

// Mockups show in web views of this device: the Canvas page's, and one
// made for a moment, behind the window's content, to render a frame for
// an agent (canvas screenshot) or an export.

// designOrigin is the address a mockup's page loads at, a new one each time.
// A page with no address counts as the app's own and may call into it (close
// the window, say); at this one it can't, and what it stores goes with it, so
// no design reaches another's.
func designOrigin() string {
	return "https://" + rand.Text() + ".design.invalid/"
}

// canvasPage is a web view as canvasview uses it.
type canvasPage struct{ wv *mygo.WebView }

func (p canvasPage) Show(c *ui.Context) ui.Element { return ui.WebView(c, p.wv) }
func (p canvasPage) LoadHTML(html string)          { p.wv.Page().LoadHTML(html, designOrigin()) }
func (p canvasPage) Eval(code string) (any, error) { return p.wv.Page().Eval(code) }
func (p canvasPage) SetZoom(f float64)             { p.wv.Page().SetZoomFactor(f) }
func (p canvasPage) Close()                        { p.wv.Destroy() }

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
					done(canvasPage{wv}, nil)
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

// renderCanvas renders req's page and replies to the daemon.
func (a *App) renderCanvas(req proto.CanvasRenderRequest) {
	cl, win := a.client, a.win
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
		_ = cl.CanvasRenderReply(out)
	}
	if !webViewAvailable() || win == nil {
		go reply(nil, errors.New("this device has no web view to render HTML"))
		return
	}
	go func() {
		renders.Lock()
		defer renders.Unlock()
		// A hidden window of its own at the page's size: a web view in the
		// main window flashed over its content, and wasn't laid out while
		// the window was behind others.
		hidden := mygo.NewWindow(mygo.WindowOptions{Hidden: true, Width: req.Width, Height: req.Height, UseContentSize: true,
			Frameless: true, SkipTaskbar: true, BackgroundColor: "#ffffff"})
		r := renderIn(hidden.Page(), hidden.CapturePage, req)
		hidden.Destroy()
		wider, text, motion = r.wider, r.text, r.motion
		reply(r.data, r.err)
	}()
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
	loaded := make(chan struct{}, 1)
	off := pg.OnDidFinishLoad(func() {
		select {
		case loaded <- struct{}{}:
		default:
		}
	})
	defer off()
	pg.LoadHTML(req.HTML, designOrigin())
	select {
	case <-loaded:
	case <-time.After(20 * time.Second):
		return rendered{err: errors.New("the page didn't load")}
	}
	time.Sleep(400 * time.Millisecond) // fonts and images settle
	// A web view is laid out only once its window draws it: until then
	// the page measures at no width.
	var r rendered
	for try := 0; try < 15 && !r.laidOut; try++ {
		if v, err := pg.Eval("window.innerWidth"); err == nil {
			if n, ok := v.(float64); ok && n > 0 {
				r.laidOut = true
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !r.laidOut {
		r.err = errors.New("the web view gave no picture: it was never laid out")
		return r
	}
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
	// The web view may not be painted yet at the first try.
	png, err := capture()
	for try := 0; try < 3 && (err != nil || len(png) == 0); try++ {
		time.Sleep(500 * time.Millisecond)
		png, err = capture()
	}
	if err == nil && len(png) == 0 {
		err = errors.New("the web view gave no picture")
	}
	r.data, r.err = png, err
	return r
}
