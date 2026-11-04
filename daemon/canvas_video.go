package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
)

const (
	// videoFirstWord is how soon the app says it took a video on: one that doesn't
	// is too old to make them.
	videoFirstWord = 30 * time.Second
	// videoQuiet is how long a video may go with no frame done.
	videoQuiet = 2 * time.Minute
)

func init() { slowMethods[proto.MethodCanvasVideo] = true }

// videoWaits are the videos the app is making, by request.
type videoWaits struct {
	mu   sync.Mutex
	done map[string]chan proto.CanvasVideoReply
	word map[string]chan struct{}
}

// canvasVideo has the app render a moving design to an MP4.
func (s *Server) canvasVideo(p proto.CanvasVideoParams) (proto.CanvasVideoResult, error) {
	c, err := s.canvasFor(p.ID).Get(p.ID)
	if err != nil {
		return proto.CanvasVideoResult{}, err
	}
	f, err := renderFrame(c, p.Frame)
	if err != nil {
		return proto.CanvasVideoResult{}, err
	}
	if strings.TrimSpace(f.HTML) == "" || !canvas.UsesMotion(f.HTML) {
		return proto.CanvasVideoResult{}, errors.New("canvas: this design doesn't move: export a PNG instead")
	}
	fps := p.FPS
	switch fps {
	case 0:
		fps = 30
	case 30, 60:
	default:
		return proto.CanvasVideoResult{}, errors.New("canvas: a video is 30 or 60 frames a second")
	}
	b := &s.browsers
	b.mu.Lock()
	if len(b.serving) == 0 {
		b.mu.Unlock()
		return proto.CanvasVideoResult{}, canvas.ErrNeedsBrowser
	}
	app := b.serving[len(b.serving)-1]
	b.mu.Unlock()
	assets := func(name string) ([]byte, error) { return s.canvasFor(c.ID).Asset(c.ID, name) }
	req := proto.CanvasVideoRequest{ID: newRenderID(), HTML: canvas.FrameDocument(f, assets), Name: c.Title + " " + f.Name,
		Width: f.Width, Height: f.Height, OutWidth: p.Width, FPS: fps, From: p.From, To: p.To, Warning: canvas.VideoWarning(f), Preview: p.Preview}
	done, word := make(chan proto.CanvasVideoReply, 1), make(chan struct{}, 1)
	w := &s.videos
	w.mu.Lock()
	if w.done == nil {
		w.done, w.word = map[string]chan proto.CanvasVideoReply{}, map[string]chan struct{}{}
	}
	w.done[req.ID], w.word[req.ID] = done, word
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.done, req.ID)
		delete(w.word, req.ID)
		w.mu.Unlock()
	}()
	raw, _ := json.Marshal(req)
	ev, _ := json.Marshal(proto.Event{Name: proto.EventCanvasVideo, Data: raw})
	if err := app.send(proto.Frame{Type: proto.TypeEvent, Payload: ev}); err != nil {
		return proto.CanvasVideoResult{}, fmt.Errorf("canvas: the page can't reach the app to record it (%d MB with its files): %w", len(req.HTML)>>20, err)
	}
	wait := time.NewTimer(videoFirstWord)
	defer wait.Stop()
	heard := false
	for {
		select {
		case r := <-done:
			if r.Error != "" {
				return proto.CanvasVideoResult{}, errors.New(r.Error)
			}
			return r.Result, nil
		case <-word:
			heard = true
			wait.Reset(videoQuiet)
		case <-s.done:
			return proto.CanvasVideoResult{}, errors.New("canvas: the daemon is stopping")
		case <-wait.C:
			if !heard {
				return proto.CanvasVideoResult{}, errors.New("canvas: the app didn't take the video on: it may need updating")
			}
			return proto.CanvasVideoResult{}, errors.New("canvas: the app stopped making the video")
		}
	}
}

func (w *videoWaits) progress(p proto.CanvasVideoProgress) {
	w.mu.Lock()
	ch := w.word[p.ID]
	w.mu.Unlock()
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (w *videoWaits) reply(r proto.CanvasVideoReply) {
	w.mu.Lock()
	ch := w.done[r.ID]
	w.mu.Unlock()
	if ch != nil {
		select {
		case ch <- r:
		default:
		}
	}
}
