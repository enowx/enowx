// Package watchdog leaves a report when a program stops answering, so that the next
// freeze has a cause: the stacks of every goroutine and what was on screen, written
// while it is stuck, as nothing can be asked of it after. It also keeps a short line of
// what memory holds, so that growth over hours shows. It is cheap enough for release
// builds: a look twice a second, stacks only when something is wrong.
package watchdog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// lagAfter is how long an answer takes for a stack to be worth having: typing
	// feels it. stallAfter is when the program counts as stuck.
	lagAfter   = 700 * time.Millisecond
	stallAfter = 3 * time.Second
	// lagEvery bounds the reports of lag, which come in runs.
	lagEvery = time.Minute
	// keepReports is how many reports stay, the oldest going.
	keepReports = 30
	// memoryEvery is how often memory is written down, memoryMost how long that
	// file grows before the older half goes.
	memoryEvery = 5 * time.Minute
	memoryMost  = 1 << 20
	beatEvery   = time.Second
	// jumpEvery is how often the heap is looked at for a jump, jumpBytes how much
	// it must grow to be one, and by half as well.
	jumpEvery = 15 * time.Second
	jumpBytes = 256 << 20
	lookEvery = 500 * time.Millisecond
)

// Watch follows a thread that must keep answering.
type Watch struct {
	dir, name string
	// run runs a function on the thread watched and waits for it.
	run func(func())
	// screen says what the thread shows; it runs on that thread.
	screen func() string

	asked  atomic.Int64 // when the beat in flight was asked, 0 for none
	shown  atomic.Value // string: screen, as of the last beat
	frame  atomic.Int64 // when the frame under way began, 0 for none
	slow   slowFrames
	stopCh chan struct{}
	once   sync.Once
	// doing says what was happening lately, for a mark of memory jumping.
	doing atomic.Pointer[func() string]
	// heap is what the Go heap held at the last look, for that mark.
	heap uint64
}

// SetDoing says how to tell what was happening lately, for a mark.
func (w *Watch) SetDoing(doing func() string) { w.doing.Store(&doing) }

// slowFrames counts frames that took long, since last written.
type slowFrames struct {
	mu      sync.Mutex
	count   int
	slowest time.Duration
}

// slowFrame is how long a frame takes before it is counted.
const slowFrame = 100 * time.Millisecond

// Start watches thread name, leaving what it finds in dir. run and screen may be nil
// for a program with no thread to answer: memory alone is written down.
func Start(dir, name string, run func(func()), screen func() string) *Watch {
	w := &Watch{dir: dir, name: name, run: run, screen: screen, stopCh: make(chan struct{})}
	w.shown.Store("")
	if run != nil {
		go w.beat()
		go w.look()
	}
	go w.memory()
	return w
}

// Stop ends the watch.
func (w *Watch) Stop() { w.once.Do(func() { close(w.stopCh) }) }

// FrameStart and FrameEnd bracket building a frame, on the thread watched.
func (w *Watch) FrameStart() { w.frame.Store(time.Now().UnixNano()) }

// FrameEnd ends what FrameStart began.
func (w *Watch) FrameEnd() time.Duration {
	began := w.frame.Swap(0)
	if began == 0 {
		return 0
	}
	d := time.Duration(time.Now().UnixNano() - began)
	if d >= slowFrame {
		w.slow.mu.Lock()
		w.slow.count++
		w.slow.slowest = max(w.slow.slowest, d)
		w.slow.mu.Unlock()
	}
	return d
}

// beat has the thread answer, over and over.
func (w *Watch) beat() {
	for {
		w.asked.Store(time.Now().UnixNano())
		w.run(func() {
			if w.screen != nil {
				w.shown.Store(w.screen())
			}
		})
		w.asked.Store(0)
		select {
		case <-w.stopCh:
			return
		case <-time.After(beatEvery):
		}
	}
}

// look writes a report when the beat in flight has taken too long, once for a stall
// and at most once a minute for lag. A machine that slept shows as the look itself
// being late, and doesn't count.
func (w *Watch) look() {
	var lastLag, stuckSince int64
	var lastLook = time.Now()
	tick := time.NewTicker(lookEvery)
	defer tick.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case now := <-tick.C:
			late := now.Sub(lastLook) > 4*lookEvery
			lastLook = now
			asked := w.asked.Load()
			if asked == 0 || late {
				if stuckSince != 0 && asked == 0 {
					w.line(fmt.Sprintf("%s answered again after %s", w.name, time.Duration(now.UnixNano()-stuckSince).Round(100*time.Millisecond)))
				}
				if asked == 0 || late {
					stuckSince = 0
				}
				continue
			}
			waited := time.Duration(now.UnixNano() - asked)
			switch {
			case waited >= stallAfter && stuckSince == 0:
				stuckSince = asked
				w.report("stall", waited)
			case waited >= lagAfter && stuckSince == 0 && now.UnixNano()-lastLag >= int64(lagEvery):
				lastLag = now.UnixNano()
				w.report("lag", waited)
			}
		}
	}
}

// report writes the stacks of every goroutine and what was on screen.
func (w *Watch) report(kind string, waited time.Duration) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s hasn't answered for %s, at %s\n", kind, w.name, waited.Round(10*time.Millisecond), time.Now().Format(time.RFC3339))
	if began := w.frame.Load(); began != 0 {
		fmt.Fprintf(&b, "in a frame for %s\n", time.Duration(time.Now().UnixNano()-began).Round(10*time.Millisecond))
	} else {
		b.WriteString("not in a frame\n")
	}
	shown, _ := w.shown.Load().(string)
	fmt.Fprintf(&b, "on screen, as of its last answer: %s\n", shown)
	fmt.Fprintf(&b, "%s\n\n%s", memoryLine(), allStacks())
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return
	}
	file := filepath.Join(w.dir, fmt.Sprintf("%s-%s-%s.txt", w.name, kind, time.Now().Format("20060102-150405")))
	if os.WriteFile(file, []byte(b.String()), 0o600) == nil {
		trimReports(w.dir, keepReports)
	}
}

// allStacks is every goroutine's stack, as much as fits in 8 MB.
func allStacks() string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) || len(buf) >= 8<<20 {
			return string(buf[:n])
		}
		buf = make([]byte, 2*len(buf))
	}
}

// trimReports keeps the newest keep reports in dir; the memory log stays.
func trimReports(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".txt") && !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	// Names end in their time; the time decides whichever thread wrote it.
	sort.Slice(names, func(i, j int) bool { return reportTime(names[i]) < reportTime(names[j]) })
	for _, name := range names[:max(len(names)-keep, 0)] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func reportTime(name string) string {
	name = strings.TrimSuffix(name, ".txt")
	if len(name) < len("20060102-150405") {
		return name
	}
	return name[len(name)-len("20060102-150405"):]
}

// memory writes down what memory holds, and the frames that were slow.
func (w *Watch) memory() {
	tick := time.NewTicker(memoryEvery)
	defer tick.Stop()
	glance := time.NewTicker(jumpEvery)
	defer glance.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-glance.C:
			w.lookForJump()
			continue
		case <-tick.C:
		}
		w.slow.mu.Lock()
		count, slowest := w.slow.count, w.slow.slowest
		w.slow.count, w.slow.slowest = 0, 0
		w.slow.mu.Unlock()
		line := memoryLine()
		if count > 0 {
			line += fmt.Sprintf(" slowFrames=%d slowest=%s", count, slowest.Round(time.Millisecond))
		}
		w.line(line)
	}
}

// lookForJump marks memory jumping since the last look, with what was happening.
func (w *Watch) lookForJump() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if heapJumped(w.heap, m.HeapAlloc) {
		w.line(w.jumpMark(w.heap, m.HeapAlloc))
	}
	w.heap = m.HeapAlloc
}

// heapJumped says the heap grew much and by much, from one look to the next.
func heapJumped(before, now uint64) bool {
	return before > 0 && now >= before+jumpBytes && now >= before+before/2
}

func (w *Watch) jumpMark(before, now uint64) string {
	shown, _ := w.shown.Load().(string)
	doing := ""
	if f := w.doing.Load(); f != nil {
		doing = (*f)()
	}
	return fmt.Sprintf("mark memory jump heap=%dMB from=%dMB doing=%q on screen: %s", now>>20, before>>20, doing, shown)
}

// memoryLine says what the Go heap holds and how many goroutines there are.
func memoryLine() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf("heap=%dMB sys=%dMB objects=%d goroutines=%d gcPauseTotal=%s", m.HeapAlloc>>20, m.Sys>>20, m.HeapObjects, runtime.NumGoroutine(), time.Duration(m.PauseTotalNs).Round(time.Millisecond))
}

// line adds a line to this thread's log.
func (w *Watch) line(text string) {
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return
	}
	path := filepath.Join(w.dir, w.name+".log")
	data, _ := os.ReadFile(path)
	if len(data) > memoryMost {
		lines := strings.Split(string(data), "\n")
		data = []byte(strings.Join(slices.Clone(lines[len(lines)/2:]), "\n"))
	}
	data = append(data, []byte(time.Now().Format(time.RFC3339)+" "+text+"\n")...)
	_ = os.WriteFile(path, data, 0o600)
}
