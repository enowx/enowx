package daemon

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
)

// canvasState is what the daemon keeps of canvases while it runs: those
// the user paused, and renders waiting for the app. Safe from any
// goroutine.
type canvasState struct {
	mu      sync.Mutex
	paused  map[string]bool
	pending map[string]chan proto.CanvasRenderReply
}

func (cs *canvasState) isPaused(id string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.paused[id]
}

func (cs *canvasState) setPaused(id string, on bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.paused == nil {
		cs.paused = map[string]bool{}
	}
	if on {
		cs.paused[id] = true
	} else {
		delete(cs.paused, id)
	}
}

// errCanvasPaused is an agent's change to a canvas the user paused.
var errCanvasPaused = errors.New("canvas: the user paused agents on this canvas: wait until they resume it (enowx-app canvas show tells), then go on from what it shows")

// withState is c as the daemon tells it: paused or not, and its space.
func (s *Server) withState(c proto.Canvas) proto.Canvas {
	c.Paused = s.canvasState.isPaused(c.ID)
	c.Team = s.teamOfCanvas(c.ID)
	return c
}

// canvasEdit makes e, an agent's refused while the canvas is paused, and
// tells every app what changed.
func (s *Server) canvasEdit(e proto.CanvasEdit) (proto.Canvas, error) {
	if e.Session != "" && s.canvasState.isPaused(e.ID) {
		return proto.Canvas{}, errCanvasPaused
	}
	if err := s.checkLease(e); err != nil {
		return proto.Canvas{}, err
	}
	if err := s.checkBuildLease(e); err != nil {
		return proto.Canvas{}, err
	}
	c, frame, element, err := s.canvasFor(e.ID).Edit(e)
	if err != nil {
		return proto.Canvas{}, err
	}
	c = s.withState(c)
	s.broadcast(proto.EventCanvasChanged, proto.CanvasChange{Canvas: c, Op: e.Op, Frame: frame, Element: element, Session: e.Session})
	if e.Op == proto.CanvasOpMeta {
		s.canvasesChanged()
	}
	return c, nil
}

// canvasShown tells every app canvas c after a change of op not an edit:
// an undo, a redo, a restore.
func (s *Server) canvasShown(c proto.Canvas, op string, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	c = s.withState(c)
	s.broadcast(proto.EventCanvasChanged, proto.CanvasChange{Canvas: c, Op: op})
	return c, nil
}

// canvasesChanged tells every app the canvases as they are now.
func (s *Server) canvasesChanged() {
	if list, err := s.allCanvases(); err == nil {
		for i := range list {
			list[i].Paused = s.canvasState.isPaused(list[i].ID)
		}
		s.broadcast(proto.EventCanvasesChanged, list)
	}
	s.teamSync.kick()
}

// canvasControl pauses, resumes or stops the agents designing a canvas:
// stopping interrupts the agent that changed it last, as Esc does, and
// pauses it.
func (s *Server) canvasControl(p proto.CanvasControl) (proto.CanvasInfo, error) {
	c, err := s.canvasFor(p.ID).Get(p.ID)
	if err != nil {
		return proto.CanvasInfo{}, err
	}
	switch p.Action {
	case proto.CanvasPause:
		s.canvasState.setPaused(p.ID, true)
	case proto.CanvasResume:
		s.canvasState.setPaused(p.ID, false)
	case proto.CanvasOpen, proto.CanvasCompare:
		// Designs by number or name, as the apps know them: by ID.
		ch := proto.CanvasChange{Op: p.Action}
		if p.Frame != "" {
			i, err := canvas.FrameIndex(c, p.Frame)
			if err != nil {
				return proto.CanvasInfo{}, err
			}
			ch.Frame = c.Frames[i].ID
		}
		for _, ref := range p.Frames {
			i, err := canvas.FrameIndex(c, ref)
			if err != nil {
				return proto.CanvasInfo{}, err
			}
			ch.Frames = append(ch.Frames, c.Frames[i].ID)
		}
		if p.Action == proto.CanvasCompare && (len(ch.Frames) < 2 || len(ch.Frames) > 4) {
			return proto.CanvasInfo{}, errors.New("canvas: compare 2 to 4 designs")
		}
		ch.Canvas = s.withState(c)
		s.broadcast(proto.EventCanvasChanged, ch)
		return ch.Canvas.CanvasInfo, nil
	case proto.CanvasStop:
		s.canvasState.setPaused(p.ID, true)
		if sess, err := s.sessions.Get(c.Agent); err == nil {
			key := "\x03"
			if preset, ok := agent.Find(sess.Meta().Agent); ok {
				key = preset.InterruptKey()
			}
			s.sessions.Input(sess, []byte(key))
		}
	default:
		return proto.CanvasInfo{}, errors.New("canvas: pause, resume, stop, open or compare")
	}
	c = s.withState(c)
	s.broadcast(proto.EventCanvasChanged, proto.CanvasChange{Canvas: c, Op: p.Action})
	return c.CanvasInfo, nil
}

// canvasRender renders a frame: shapes alone here, a mockup's HTML by the
// app serving Browsers.
func (s *Server) canvasRender(p proto.CanvasRenderParams) (proto.CanvasRendered, error) {
	c, err := s.canvasFor(p.ID).Get(p.ID)
	if err != nil {
		return proto.CanvasRendered{}, err
	}
	assets := func(name string) ([]byte, error) { return s.canvasFor(c.ID).Asset(c.ID, name) }
	format := p.Format
	if format == "" {
		format = proto.RenderPNG
	}
	if format == proto.RenderHTML && p.Frame == "" {
		return proto.CanvasRendered{Format: format, Data: []byte(canvas.Standalone(c, assets)), By: "daemon"}, nil
	}
	f, err := renderFrame(c, p.Frame)
	if err != nil {
		return proto.CanvasRendered{}, err
	}
	if p.Width > 0 && p.Width != f.Width && strings.TrimSpace(f.HTML) != "" {
		f.Width, f.Height = p.Width, max(844, f.Height*p.Width/max(f.Width, 1))
	}
	out := proto.CanvasRendered{Format: format, Width: f.Width, Height: f.Height, By: "daemon"}
	switch format {
	case proto.RenderHTML:
		out.Data = []byte(canvas.FrameDocument(f, assets))
		return out, nil
	case proto.RenderSVG:
		out.Data = []byte(canvas.SVG(f, assets, true))
		return out, nil
	case proto.RenderPNG, proto.RenderPDF:
	default:
		return proto.CanvasRendered{}, errors.New("canvas: render as png, pdf, svg or html")
	}
	if strings.TrimSpace(f.HTML) == "" {
		if f.Background == "" {
			f.Background = canvas.BoardColor // as the board shows it: shapes off a sheet show so
		}
		img := canvas.Raster(f, assets, 1)
		if format == proto.RenderPDF {
			out.Data, err = canvas.PDF(img)
		} else {
			out.Data, err = encodePNG(img)
		}
		return out, err
	}
	out.By = "app"
	doc := canvas.FrameDocument(f, assets)
	if p.At != "" {
		if doc, err = canvas.MotionAt(doc, p.At); err != nil {
			return proto.CanvasRendered{}, err
		}
	}
	r, err := s.appRender(proto.CanvasRenderRequest{HTML: doc, Width: f.Width, Height: f.Height, Format: format})
	out.Data, out.Wider, out.Motion = r.Data, r.Wider, r.Motion
	return out, err
}

// renderFrame is c's frame ref: a frame by ID or name, or a device's
// size ("desktop", "tablet", "mobile") of the first frame.
func renderFrame(c proto.Canvas, ref string) (proto.CanvasFrame, error) {
	i, err := canvas.FrameIndex(c, ref)
	if err == nil {
		return c.Frames[i], nil
	}
	w, h, ok := canvas.Device(ref)
	if !ok {
		return proto.CanvasFrame{}, err
	}
	f := c.Frames[0]
	// Shapes keep their place; the page reflows at the device's width.
	f.Width, f.Height = w, max(h, f.Height*w/max(f.Width, 1))
	return f, nil
}

func encodePNG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	err := png.Encode(&b, img)
	return b.Bytes(), err
}

// canvasRenderTimeout bounds a render by the app: a page loading.
const canvasRenderTimeout = 40 * time.Second

// appRender has the latest app serving Browsers render req.
func (s *Server) appRender(req proto.CanvasRenderRequest) (proto.CanvasRenderReply, error) {
	b := &s.browsers
	b.mu.Lock()
	if len(b.serving) == 0 {
		b.mu.Unlock()
		return proto.CanvasRenderReply{}, canvas.ErrNeedsBrowser
	}
	app := b.serving[len(b.serving)-1]
	b.mu.Unlock()
	req.ID = newRenderID()
	ch := make(chan proto.CanvasRenderReply, 1)
	cs := &s.canvasState
	cs.mu.Lock()
	if cs.pending == nil {
		cs.pending = map[string]chan proto.CanvasRenderReply{}
	}
	cs.pending[req.ID] = ch
	cs.mu.Unlock()
	raw, _ := json.Marshal(req)
	ev, _ := json.Marshal(proto.Event{Name: proto.EventCanvasRender, Data: raw})
	app.send(proto.Frame{Type: proto.TypeEvent, Payload: ev})
	select {
	case r := <-ch:
		if r.Error != "" {
			return proto.CanvasRenderReply{}, errors.New(r.Error)
		}
		return r, nil
	case <-time.After(canvasRenderTimeout):
		cs.mu.Lock()
		delete(cs.pending, req.ID)
		cs.mu.Unlock()
		return proto.CanvasRenderReply{}, errors.New("canvas: the app didn't render it in time")
	}
}

func (cs *canvasState) reply(r proto.CanvasRenderReply) {
	cs.mu.Lock()
	ch := cs.pending[r.ID]
	delete(cs.pending, r.ID)
	cs.mu.Unlock()
	if ch != nil {
		ch <- r
	}
}

func newRenderID() string {
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	return hex.EncodeToString(id)
}

// canvasChatChanged tells every app a canvas's conversation after a change.
func (s *Server) canvasChatChanged(log proto.CanvasChatLog, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	s.broadcast(proto.EventCanvasChat, log)
	return log, nil
}
