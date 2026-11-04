// Package memtune sets how a long-running program of enowX gives memory back: Go keeps
// what a burst took for minutes (its scavenger works at 1% of a core, towards a goal only
// the next collection lowers, and an idle program collects every two minutes), and a
// burst is what these programs are used to having. See docs/performance.md.
package memtune

import (
	"os"
	"runtime/debug"
	"runtime/metrics"
	"time"
)

const (
	// limit is GOMEMLIMIT unless the environment sets one: well above an ordinary heap,
	// so ordinary work never feels it, it makes a burst collect sooner rather than take
	// gigabytes of address space. Over it the runtime spends at most half the CPU
	// collecting, so a heap that truly needs more only runs slower.
	limit = 1 << 30
	// trimEvery is how often the heap is looked at; each look is a read of counters.
	trimEvery = 30 * time.Second
	// trimOver is how much free memory held from the system is worth a collection.
	trimOver  = 32 << 20
	freeBytes = "/memory/classes/heap/free:bytes"
)

// Start applies the limit, and returns free memory to the system while done is open.
// It lets the user's GOMEMLIMIT stand.
func Start(done <-chan struct{}) {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(limit)
	}
	go func() {
		tick := time.NewTicker(trimEvery)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				if FreeHeld() > trimOver {
					debug.FreeOSMemory()
				}
			}
		}
	}()
}

// FreeHeld is the heap's free memory still held from the system.
func FreeHeld() uint64 {
	s := []metrics.Sample{{Name: freeBytes}}
	metrics.Read(s)
	return s[0].Value.Uint64()
}
