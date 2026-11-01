package daemon

import (
	"sync/atomic"
	"testing"
	"time"
)

// Stopping waits for work that ends, gives up on work that doesn't, and
// starts none after.
func TestBackgroundWorkStop(t *testing.T) {
	var ends backgroundWork
	var ended atomic.Bool
	ends.start(func() {
		time.Sleep(20 * time.Millisecond)
		ended.Store(true)
	})
	ends.stop(time.Minute)
	if !ended.Load() {
		t.Fatal("stop returned before the work ended")
	}
	var late atomic.Bool
	ends.start(func() { late.Store(true) })
	time.Sleep(10 * time.Millisecond)
	if late.Load() {
		t.Fatal("work started after stop")
	}

	var stuck backgroundWork
	release := make(chan struct{})
	defer close(release)
	stuck.start(func() { <-release })
	stuck.stop(20 * time.Millisecond) // returning at all is the test
}
