package ui

import (
	"sync"
	"testing"
	"time"
)

type nopConn struct{}

func (nopConn) Read([]byte) (int, error)    { select {} }
func (nopConn) Write(p []byte) (int, error) { return len(p), nil }
func (nopConn) Close() error                { return nil }

// A window going through many sizes resizes the session once, to the last.
func TestTerminalSizeIsPassedOnOnceSettled(t *testing.T) {
	var mu sync.Mutex
	var got [][2]int
	s := &settledResize{ReadWriteCloser: nopConn{}, resize: func(cols, rows, _, _ int) error {
		mu.Lock()
		got = append(got, [2]int{cols, rows})
		mu.Unlock()
		return nil
	}}
	for i := range 30 { // an animation's frames
		_ = s.ResizePixels(80+i, 24+i/2, 8, 16)
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(3 * resizeSettle)
	_ = s.ResizePixels(200, 50, 8, 16) // later, on its own
	time.Sleep(3 * resizeSettle)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || got[0] != [2]int{109, 38} || got[1] != [2]int{200, 50} {
		t.Fatalf("resized %v", got)
	}
}

// A view back in sight asks for its size again when another view of the session has
// changed it meanwhile, and only then: the terminal itself asks only when its own size
// changes, so the session stayed at the other view's size (issue #44).
func TestViewBackInSightAsksItsSizeAgain(t *testing.T) {
	sent := make(chan [2]int, 4)
	s := &settledResize{resize: func(cols, rows, _, _ int) error { sent <- [2]int{cols, rows}; return nil }}
	start := time.Now()
	_ = s.ResizePixels(120, 40, 8, 16)
	s.Shown(start)
	if got := <-sent; got != [2]int{120, 40} {
		t.Fatalf("first ask: %v", got)
	}
	s.Heard(120, 40)

	// Drawn on: nothing more.
	s.Shown(start.Add(500 * time.Millisecond))
	// Out of sight a while, the session still at this size: nothing either.
	s.Shown(start.Add(5 * time.Second))
	// Another view made it small, and this one comes back.
	s.Heard(40, 12)
	s.Shown(start.Add(10 * time.Second))
	select {
	case got := <-sent:
		if got != [2]int{120, 40} {
			t.Fatalf("asked %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("back in sight, it didn't ask for its size")
	}
	select {
	case got := <-sent:
		t.Fatalf("asked again: %v", got)
	case <-time.After(300 * time.Millisecond):
	}
}
