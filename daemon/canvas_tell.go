package daemon

import (
	"sync"
	"time"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
)

// A change of a canvas tells the apps the whole canvas, which an agent's many small
// patches would send megabytes at a time to every app, each to decode. So a canvas is
// told at once, then at most this often: the latest change holds all the earlier ones.
// Protocol unchanged, as an older app expects the whole canvas.
const canvasTellEvery = 250 * time.Millisecond

// canvasTeller holds what waits to be told, by canvas.
type canvasTeller struct {
	mu      sync.Mutex
	last    map[string]time.Time
	waiting map[string]proto.CanvasChange
}

// tell tells ch now if none was told of its canvas just now, else soon.
func (t *canvasTeller) tell(s *Server, ch proto.CanvasChange) {
	t.mu.Lock()
	if t.last == nil {
		t.last, t.waiting = map[string]time.Time{}, map[string]proto.CanvasChange{}
	}
	id := ch.Canvas.ID
	wait := canvasTellEvery - time.Since(t.last[id])
	if wait <= 0 {
		t.last[id] = time.Now()
		t.mu.Unlock()
		s.broadcast(proto.EventCanvasChanged, ch)
		return
	}
	_, timer := t.waiting[id]
	t.waiting[id] = ch
	t.mu.Unlock()
	if !timer {
		time.AfterFunc(wait, func() { t.flush(s, id) })
	}
}

// flush tells what waits for canvas id.
func (t *canvasTeller) flush(s *Server, id string) {
	t.mu.Lock()
	ch, ok := t.waiting[id]
	delete(t.waiting, id)
	t.last[id] = time.Now()
	t.mu.Unlock()
	if ok {
		s.broadcast(proto.EventCanvasChanged, ch)
	}
}

// convertCanvasHistories turns every canvas's versions kept whole into versions sharing
// their frames, once, unhurried, at the daemon's start.
func (s *Server) convertCanvasHistories() {
	stores := []*canvas.Store{s.canvases}
	for _, sp := range s.spaces() {
		if st, err := s.canvasesIn(sp.Team); err == nil {
			stores = append(stores, st)
		}
	}
	var freed int64
	for _, st := range stores {
		list, err := st.List()
		if err != nil {
			continue
		}
		for _, c := range list {
			n, err := st.ConvertHistory(c.ID)
			if err != nil {
				s.logger().Warn("canvas: convert versions", "err", err)
			}
			freed += n
			select {
			case <-s.done:
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	if freed > 0 {
		s.logger().Info("canvas: versions now share their frames", "freedMB", freed>>20)
	}
}
