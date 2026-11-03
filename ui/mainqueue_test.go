package ui

import "sync"

// mainQueue stands for the main thread in a test: what's posted from other goroutines waits
// here for the test's next run. A plain slice lost a reply posted between reading it and
// clearing it, and the test then waited for what would never come.
type mainQueue struct {
	mu  sync.Mutex
	fns []func()
}

func (q *mainQueue) post(fn func()) {
	q.mu.Lock()
	q.fns = append(q.fns, fn)
	q.mu.Unlock()
}

// run runs what's waiting. What that posts waits for the next run.
func (q *mainQueue) run() {
	q.mu.Lock()
	fns := q.fns
	q.fns = nil
	q.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}
