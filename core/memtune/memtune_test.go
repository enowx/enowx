package memtune

import (
	"runtime"
	"runtime/debug"
	"testing"
)

var burst [][]byte

// What a burst took goes back to the system, and the limit stays as the user set it.
func TestFreeHeldFallsAfterABurst(t *testing.T) {
	for range 128 {
		burst = append(burst, make([]byte, 1<<20))
	}
	burst = nil
	runtime.GC()
	if FreeHeld() < trimOver {
		t.Fatalf("free %d after a burst", FreeHeld())
	}
	debug.FreeOSMemory()
	if FreeHeld() >= trimOver {
		t.Fatalf("free %d after returning it", FreeHeld())
	}
}

func TestStartLetsTheUsersLimitStand(t *testing.T) {
	t.Setenv("GOMEMLIMIT", "123MiB")
	was := debug.SetMemoryLimit(-1)
	done := make(chan struct{})
	defer close(done)
	Start(done)
	if now := debug.SetMemoryLimit(-1); now != was {
		t.Fatalf("limit changed from %d to %d", was, now)
	}
}
