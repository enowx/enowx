package daemon

import (
	"sync"
	"time"
)

// closeWait is how long Close waits for work in the background: long enough
// for what's writing to finish, short of letting a client that stopped
// reading, or a long backfill, keep the daemon from stopping.
const closeWait = 5 * time.Second

// backgroundWork is work a request started that goes on after its answer
// and writes in the daemon's folder; Close waits for it, so nothing is
// written after the daemon stopped. The mutex orders start against stop,
// as a WaitGroup's Add must not race its Wait.
type backgroundWork struct {
	mu      sync.Mutex
	stopped bool
	running sync.WaitGroup
}

// start runs fn on a goroutine of its own, unless the daemon is stopping.
func (b *backgroundWork) start(fn func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return
	}
	b.running.Add(1)
	go func() {
		defer b.running.Done()
		fn()
	}()
}

// stop refuses new work and waits for what runs, for up to within.
func (b *backgroundWork) stop(within time.Duration) {
	b.mu.Lock()
	b.stopped = true
	b.mu.Unlock()
	finished := make(chan struct{})
	go func() {
		b.running.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(within):
	}
}
