package daemon

import (
	"path/filepath"
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
	root := s.canvasHistoryBackups()
	type place struct {
		store *canvas.Store
		name  string
	}
	places := []place{{s.canvases, "personal"}}
	for _, sp := range s.spaces() {
		if st, err := s.canvasesIn(sp.Team); err == nil {
			places = append(places, place{st, sp.Team})
		}
	}
	var freed int64
	for _, pl := range places {
		list, err := pl.store.List()
		if err != nil {
			continue
		}
		for _, c := range list {
			// Nothing is converted without its way back: a backup that fails leaves the
			// canvas as it was, and it's tried again at the next start.
			dest := filepath.Join(root, pl.name, c.ID)
			todo, err := pl.store.BackupHistory(c.ID, dest)
			switch {
			case err != nil:
				s.logger().Warn("canvas: versions not converted: their backup failed", "canvas", c.ID, "err", err)
			case todo:
				s.logger().Info("canvas: versions backed up before converting", "canvas", c.ID, "backup", dest)
				n, err := pl.store.ConvertHistory(c.ID)
				if err != nil {
					s.logger().Warn("canvas: convert versions", "err", err)
				}
				freed += n
			}
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

// canvasHistoryBackups is where canvases' versions are kept from before their conversion: a
// folder of its own in the data folder, which neither sync nor Clean cache know of.
func (s *Server) canvasHistoryBackups() string {
	return filepath.Join(s.dataDir, canvasBackupFolder)
}

// canvasBackupFolder is that folder's name.
const canvasBackupFolder = "canvas-history-backup"
