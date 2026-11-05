package session

import (
	"runtime"
	"testing"
	"time"
)

// A session keeps about Scrollback of what it prints, not the terminal
// library's 10 MiB default: the daemon's memory is mostly one scrollback a
// session (docs/performance.md).
func TestASessionsScrollbackIsBounded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	m := NewManager()
	defer m.CloseAll()
	// About 12 MB of text: more than the old default, five times the new one.
	out := `awk 'BEGIN{for(i=0;i<300000;i++) printf "%040d\n", i}'`
	s, err := m.Create(Spec{ID: "a", Command: []string{"sh", "-c", out}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Minute)
	for !s.Info().Exited {
		if time.Now().After(deadline) {
			t.Fatal("the program didn't finish")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The text is kept in VT sequences, a little longer than what was printed:
	// 6 MB leaves that room and is still half what the old 10 MiB default of
	// the terminal library would have kept.
	if got := len(s.History()); got > 6<<20 {
		t.Fatalf("kept %d bytes of history, for %d of text at most", got, Scrollback)
	}
}
