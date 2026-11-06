package ui

import (
	"io"
	"sync"
	"time"
)

// settledResize passes a terminal's size on to its session once the size
// stopped changing for resizeSettle: a window growing or zooming goes
// through many sizes, and each would have the program (an agent's whole
// screen) redraw, a round trip away on a server. The terminal keeps its
// size until the session takes the last one.
type settledResize struct {
	io.ReadWriteCloser
	resize func(cols, rows, cellWidth, cellHeight int) error

	mu    sync.Mutex
	timer *time.Timer
	size  [4]int
	// heard is the size the session last said it has, and shown when a view last drew it.
	heard [2]int
	shown time.Time
}

// showGap is how long a view goes undrawn before its drawing counts as coming back.
const showGap = time.Second

// Heard keeps the size the session says it now has, which another view of it may have set.
func (s *settledResize) Heard(cols, rows int) {
	s.mu.Lock()
	s.heard = [2]int{cols, rows}
	s.mu.Unlock()
}

// Shown says a view drew the terminal at now. Back in sight with the session at another
// view's size, it asks for its own again: its own size didn't change, so the terminal,
// which asks only on a change, wouldn't.
func (s *settledResize) Shown(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	back := !s.shown.IsZero() && now.Sub(s.shown) > showGap
	s.shown = now
	if !back || s.size[0] == 0 || s.heard == [2]int{s.size[0], s.size[1]} {
		return
	}
	if s.timer == nil {
		s.timer = time.AfterFunc(resizeSettle, s.send)
	} else {
		s.timer.Reset(resizeSettle)
	}
}

// resizeSettle is how long a size must hold to be passed on.
const resizeSettle = 120 * time.Millisecond

// ResizePixels keeps the size, to pass on once it held.
func (s *settledResize) ResizePixels(cols, rows, cellWidth, cellHeight int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.size = [4]int{cols, rows, cellWidth, cellHeight}
	if s.timer == nil {
		s.timer = time.AfterFunc(resizeSettle, s.send)
	} else {
		s.timer.Reset(resizeSettle)
	}
	return nil
}

func (s *settledResize) send() {
	s.mu.Lock()
	size := s.size
	s.mu.Unlock()
	_ = s.resize(size[0], size[1], size[2], size[3])
}

// Close stops a size waiting to be passed on.
func (s *settledResize) Close() error {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.mu.Unlock()
	return s.ReadWriteCloser.Close()
}
