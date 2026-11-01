package canvasview

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/unslopview"
)

// A mockup's HTML shows in a web view of this device, at the frame's size
// zoomed to fit; the shapes above it are drawn natively. An agent's change
// to one element goes into the page as it is, so the page doesn't reload
// at each, and the element changed is outlined a moment.

// Page is a web view showing a mockup.
type Page interface {
	Show(c *ui.Context) ui.Element
	LoadHTML(html string)
	// Eval runs JavaScript in the page and returns its value; it waits,
	// so it runs off the main thread.
	Eval(code string) (any, error)
	SetZoom(f float64)
	Close()
}

// Web makes and keeps the web view mockups show in.
type Web struct {
	// NewPage makes a page and hands it to done on the main thread.
	NewPage func(done func(Page, error))
	// Asset reads a canvas's file, off the main thread.
	Asset func(canvas, name string) ([]byte, error)
	// Update runs a function on the main thread.
	Update func(func())

	page    Page
	making  bool
	shown   string // canvas/frame/version loaded
	html    string // the frame's HTML loaded, its shapes left out
	pending string // the frame's document, loaded once the page is made (compose)
	zoom    float64
	sel     string // the element outlined as selected
	polling bool   // a link's design is being asked for
	// state is what a prototype's pages hold between them, as JSON, of
	// canvas stateOf (core/canvas/prototype.go).
	state   string
	stateOf string
	loads   int // documents composed, so an old page's answer isn't taken for this one's
	polled  time.Time
	design  bool // the page shows whole on the board: no scrolling of its own
	// wider is how far the page reaches past its frame's width, in CSS
	// pixels: a horizontal scroll the design mustn't have.
	wider     int
	measured  time.Time
	measuring bool
	// scanned is the redesign's read last drawn in the page; pass the
	// pass under way, tagging what changes (unslop_web.go).
	scanned    string
	pass       string
	outlined   string                    // the regions last outlined
	regionTags map[string]unslopview.Tag // by a region's agent's session
}

// WiderScript is JavaScript giving how far a page reaches past its
// width, in CSS pixels: what a horizontal scroll scrolls.
const WiderScript = "(function(){var d=document.documentElement,b=document.body;return Math.max(d.scrollWidth,b?b.scrollWidth:0)-d.clientWidth})()"

// measureWidth asks the page, now and then, how far it reaches past its
// width (wider).
func (w *Web) measureWidth(c *ui.Context) {
	c.After(time.Second)
	if w.page == nil || w.measuring || time.Since(w.measured) < 900*time.Millisecond {
		return
	}
	w.measuring, w.measured = true, time.Now()
	page := w.page
	go func() {
		v, err := page.Eval(WiderScript)
		n, _ := v.(float64)
		w.Update(func() {
			w.measuring = false
			if err == nil {
				w.wider = int(n)
			}
		})
	}()
}

// Wider is how far the design's page reaches past its frame's width, 0
// when it fits.
func (w *Web) Wider() int { return max(w.wider, 0) }

// Close closes the page, as the page goes.
func (w *Web) Close() {
	if w.page != nil {
		w.page.Close()
		w.page = nil
	}
	w.shown, w.html = "", ""
}

func key(c proto.Canvas, f proto.CanvasFrame) string { return c.ID + "/" + f.ID }

// show loads frame f of c when it isn't the one loaded.
func (w *Web) show(c proto.Canvas, f proto.CanvasFrame, _ *proto.Canvas) {
	if w.shown == key(c, f) && w.html == f.HTML {
		return
	}
	w.load(c, f)
}

// load loads frame f's HTML whole, its files fetched first.
func (w *Web) load(c proto.Canvas, f proto.CanvasFrame) {
	if w.stateOf != c.ID {
		w.state, w.stateOf = "", c.ID
	}
	w.shown, w.html, w.scanned = key(c, f), f.HTML, "" // the marks go with the page
	names := canvas.AssetNames(f.HTML)
	bare := proto.CanvasFrame{Width: f.Width, Height: f.Height, Background: f.Background, HTML: f.HTML}
	if len(names) == 0 || w.Asset == nil {
		w.setDoc(canvas.FrameDocument(bare, nil))
		return
	}
	shown := w.shown
	go func() {
		files := map[string][]byte{}
		for _, n := range names {
			if data, err := w.Asset(c.ID, n); err == nil {
				files[n] = data
			}
		}
		doc := canvas.FrameDocument(bare, func(name string) ([]byte, error) {
			if data, ok := files[name]; ok {
				return data, nil
			}
			return nil, errNoAsset
		})
		w.Update(func() {
			if w.shown == shown {
				w.setDoc(doc)
			}
		})
	}()
}

var errNoAsset = errors.New("canvas: no such asset")

// gotoScript makes a mockup's links to other designs (href="#design-2",
// data-goto="Login") say where they go, for the page to show it.
const gotoScript = `<script>document.addEventListener('click',function(e){var a=e.target.closest&&e.target.closest('[data-goto],a[href^="#"]');if(!a)return;var t=a.getAttribute('data-goto')||a.getAttribute('href').slice(1);if(!t)return;e.preventDefault();window.__enowxGoto=t},true)</script>`

// pageGoto is what a page says: the design a link clicked went to, "" for none,
// its prototype's state, and which load it is.
type pageGoto struct {
	T, S string
	L    int
}

// takeGoto asks the page; it waits for it, so it runs off the main thread.
func takeGoto(page Page) pageGoto {
	v, err := page.Eval("(function(){var t=window.__enowxGoto||'';window.__enowxGoto='';return JSON.stringify({t:t,s:window.__enowxState||'',l:window.__enowxLoad||0})})()")
	if err != nil {
		return pageGoto{}
	}
	text, _ := v.(string)
	var got pageGoto
	if json.Unmarshal([]byte(text), &got) != nil {
		return pageGoto{}
	}
	return got
}

// ResetState empties the prototype's state and shows the page anew.
func (w *Web) ResetState() {
	w.state, w.shown = "", ""
}

// pollGoto asks the page for a design a link went to, now and then, while
// asked; done gets it on the main thread.
func (w *Web) pollGoto(c *ui.Context, done func(target string)) {
	c.After(400 * time.Millisecond)
	if w.page == nil || w.polling || time.Since(w.polled) < 350*time.Millisecond {
		return
	}
	w.polling, w.polled = true, time.Now()
	page := w.page
	go func() {
		got := takeGoto(page)
		w.Update(func() {
			w.polling = false
			if got.L != w.loads {
				return
			}
			if got.S != "" {
				w.state = got.S
			}
			if got.T != "" {
				done(got.T)
			}
		})
	}()
}

// designStyle keeps a page shown whole on the board from scrolling, and
// its scrollbars away: the board moves it instead.
const designStyle = `<style>html.enowx-design,html.enowx-design body{overflow:hidden!important}html.enowx-design ::-webkit-scrollbar{display:none}</style>`

// setDesign sets whether the page shows whole on the board (Design mode)
// or as a page to use, scrolling as one (Interact mode).
func (w *Web) setDesign(on bool) {
	if w.design == on {
		return
	}
	w.design = on
	if w.page == nil {
		return
	}
	// A prototype's state shows in Interact alone, so its page starts over.
	if w.state != "" && w.state != "{}" && w.pending != "" {
		w.page.LoadHTML(w.compose())
		return
	}
	page := w.page
	code := "document.documentElement.classList.toggle('enowx-design'," + map[bool]string{true: "true", false: "false"}[on] + ");window.scrollTo(0,0)"
	go func() { _, _ = page.Eval(code) }()
}

func (w *Web) setDoc(doc string) {
	w.pending, w.wider = doc, 0
	if w.page != nil {
		w.page.LoadHTML(w.compose())
	}
}

// compose is the document to load: the frame's, with the prototype's runtime
// and state, the links' script and the mode as it is now. On the board a design
// shows as it starts, whatever a prototype was left at.
func (w *Web) compose() string {
	seed := w.state
	if w.design {
		seed = ""
	}
	doc := canvas.WithHead(w.pending, canvas.PrototypeSeed(seed))
	w.loads++
	mode := fmt.Sprintf("<script>window.__enowxLoad=%d;document.documentElement.classList.remove('enowx-design')</script>", w.loads)
	if w.design {
		mode = fmt.Sprintf("<script>window.__enowxLoad=%d;document.documentElement.classList.add('enowx-design')</script>", w.loads)
	}
	if i := strings.Index(strings.ToLower(doc), "</head>"); i >= 0 {
		doc = doc[:i] + designStyle + doc[i:]
	} else {
		doc = designStyle + doc
	}
	if i := strings.LastIndex(strings.ToLower(doc), "</body>"); i >= 0 {
		doc = doc[:i] + gotoScript + mode + doc[i:]
	} else {
		doc += gotoScript + mode
	}
	return doc
}

// apply puts an agent's or the user's change into the page: one element
// in place, else the frame anew.
func (w *Web) apply(c proto.Canvas, f proto.CanvasFrame, _ *proto.Canvas, ch proto.CanvasChange) {
	if w.shown != key(c, f) || w.page == nil || ch.Frame != f.ID || ch.Element == "" {
		w.show(c, f, nil)
		return
	}
	if f.HTML == w.html {
		return
	}
	var code string
	switch ch.Op {
	case proto.CanvasOpHTMLRemove:
		code = "(function(){var e=document.getElementById(" + js(ch.Element) + ");if(e)e.remove();return true})()"
	case proto.CanvasOpHTMLPatch, proto.CanvasOpHTMLText:
		outer, err := canvas.ElementHTML(f.HTML, ch.Element)
		if err != nil || strings.Contains(outer, "assets/") {
			w.load(c, f)
			return
		}
		code = "(function(id,html,parent){var e=document.getElementById(id);" +
			"if(e){e.outerHTML=html}else{var p=parent?document.getElementById(parent):document.body;if(!p)return false;p.insertAdjacentHTML('beforeend',html)}" +
			"var n=document.getElementById(id);if(n){n.scrollIntoView({block:'nearest'});n.style.outline='2px solid #6366f1';n.style.outlineOffset='2px';" +
			"setTimeout(function(){n.style.outline='';n.style.outlineOffset=''},1600)}return true})(" +
			js(ch.Element) + "," + js(outer) + "," + js(canvas.ParentID(f.HTML, ch.Element)) + ")"
	default:
		w.load(c, f)
		return
	}
	w.html = f.HTML
	page, shown := w.page, w.shown
	tag := ch.Op != proto.CanvasOpHTMLRemove
	go func() {
		v, err := page.Eval(code)
		if ok, _ := v.(bool); ok && err == nil && tag {
			w.Update(func() { w.tagPass(ch.Element, ch.Session) })
		}
		if ok, _ := v.(bool); err != nil || !ok {
			w.Update(func() {
				if w.shown == shown {
					w.load(c, f)
				}
			})
		}
	}()
}

// outline marks element id as the one selected, "" none.
func (w *Web) outline(id string) {
	if w.page == nil || w.sel == id {
		return
	}
	w.sel = id
	page := w.page
	code := "(function(id){document.querySelectorAll('[data-enowx-sel]').forEach(function(e){e.removeAttribute('data-enowx-sel');e.style.boxShadow=''});" +
		"var e=id&&document.getElementById(id);if(e){e.setAttribute('data-enowx-sel','');e.style.boxShadow='0 0 0 2px #6366f1';e.scrollIntoView({block:'nearest'})}})(" + js(id) + ")"
	go func() { _, _ = page.Eval(code) }()
}

func js(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// view places the page, w×h at zoom, making it the first time.
func (w *Web) view(c *ui.Context, width, height float32, zoom float64) ui.Element {
	if w.page == nil {
		if !w.making && w.NewPage != nil {
			w.making = true
			w.NewPage(func(p Page, err error) {
				w.making = false
				if err != nil {
					return
				}
				w.page = p
				if w.pending != "" {
					p.LoadHTML(w.compose())
				}
			})
		}
		return ui.Box(c).Size(width, height)
	}
	if zoom != w.zoom {
		w.zoom = zoom
		w.page.SetZoom(zoom)
	}
	return w.page.Show(c).Size(width, height)
}

// pickScript finds the element at x, y (CSS pixels) that has an id, or an
// ancestor that has one: its id and box, "" for none.
const pickScript = `(function(x,y){var e=document.elementFromPoint(x,y);while(e&&e!==document.body&&!e.id)e=e.parentElement;if(!e||!e.id)return "";var r=e.getBoundingClientRect();return e.id+"|"+r.left+"|"+r.top+"|"+r.width+"|"+r.height})`

// pick finds the element of the page at x, y (CSS pixels): done gets its
// id and box on the main thread, "" when there is none.
func (w *Web) pick(x, y float64, done func(id string, box ui.Rect)) {
	if w.page == nil {
		return
	}
	page := w.page
	go func() {
		v, err := page.Eval(fmt.Sprintf("%s(%g,%g)", pickScript, x, y))
		s, _ := v.(string)
		parts := strings.Split(s, "|")
		w.Update(func() {
			if err != nil || len(parts) != 5 {
				done("", ui.Rect{})
				return
			}
			var n [4]float64
			for i := range n {
				n[i], _ = strconv.ParseFloat(parts[i+1], 64)
			}
			done(parts[0], ui.Rect{X: float32(n[0]), Y: float32(n[1]), W: float32(n[2]), H: float32(n[3])})
		})
	}()
}
