// Package terminal is Claude Platform's face in a session's terminal: it reads what the
// user or the chat's composer types, draws the conversation, and implements
// platformagent.Front.
//
// One goroutine, Run's, owns everything here: the parser, the line, the screen, the
// queue of messages and what is being asked. Input comes to it from a reader goroutine;
// the agent's calls (Event, Permission, ...) come as closures through post, so they
// never block on it, whatever the agent holds meanwhile.
package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/enowx/enowx/core/platformagent"
)

// Defaults of Config's timings.
const (
	// escWait is how long a lone Esc waits to be told from a key's sequence.
	escWait = 80 * time.Millisecond
	// numberWait is how long a number's first digit waits for a second one, only when a
	// question's options go that high.
	numberWait = 200 * time.Millisecond
	// exitPress is how soon a second Ctrl+C exits.
	exitPress = 2 * time.Second
	// exitWait is how long a turn gets to end on leaving.
	exitWait = 3 * time.Second
	// resizeEvery is how often the size is looked at, the same on every platform.
	resizeEvery = 300 * time.Millisecond
)

// ErrClosed answers what was being asked when the terminal closed.
var ErrClosed = errors.New("terminal: closed")

// ErrDeclined is Questions' error when the user pressed Esc instead of answering.
var ErrDeclined = errors.New("terminal: the user declined to answer")

// Config sets up a Terminal.
type Config struct {
	In  io.Reader
	Out io.Writer
	// Interactive means In and Out are the same terminal, in raw mode (Raw): the live
	// region, colours, and In's read error ends the program. Otherwise lines of plain
	// text, and In's end waits for the work under way, then leaves.
	Interactive bool
	// Cols is the terminal's width now.
	Cols func() int
	// AllowBypass offers bypassPermissions in Shift+Tab's cycle and /mode.
	AllowBypass bool
	// FirstPrompt is sent as soon as Run starts.
	FirstPrompt string
	// EscWait, NumberWait and Now replace the defaults in tests.
	EscWait, NumberWait time.Duration
	Now                 func() time.Time
}

var _ platformagent.Front = (*Terminal)(nil)

// Terminal is the agent's Front in a session's terminal.
type Terminal struct {
	cfg Config

	// Reached from any goroutine.
	mu     sync.Mutex
	posted []func()
	wake   chan struct{}
	closed chan struct{}

	// Run's goroutine alone.
	r        platformagent.Runner
	sc       *screen
	parser   Parser
	ed       editor
	stash    []rune
	ctx      context.Context
	asks     []asker
	job      *job
	queue    []job
	stopped  bool // the running job was told to stop
	saidStop bool // and the screen says it stopped
	eof      bool
	exiting  bool
	finished bool
	ctrlCAt  time.Time
	numberC  <-chan time.Time
	exitC    <-chan time.Time
	view     settingsView
	turnMsg  string // the message whose text streamed
	// The start and the latest of the thinking under way.
	thinkHead, thinkTail string
}

// New is a terminal to give an Agent as its Front, before Run.
func New(cfg Config) *Terminal {
	if cfg.EscWait == 0 {
		cfg.EscWait = escWait
	}
	if cfg.NumberWait == 0 {
		cfg.NumberWait = numberWait
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Cols == nil {
		cfg.Cols = func() int { return 80 }
	}
	return &Terminal{
		cfg:    cfg,
		wake:   make(chan struct{}, 1),
		closed: make(chan struct{}),
		sc:     newScreen(cfg.Out, cfg.Interactive, cfg.Cols),
	}
}

// post runs f on Run's goroutine, in order, without waiting for it.
func (t *Terminal) post(f func()) {
	t.mu.Lock()
	t.posted = append(t.posted, f)
	t.mu.Unlock()
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// drain runs what was posted.
func (t *Terminal) drain() {
	t.mu.Lock()
	fs := t.posted
	t.posted = nil
	t.mu.Unlock()
	for _, f := range fs {
		f()
	}
}

// Run draws and reads until the user leaves or ctx ends, then closes r. It re-panics on
// its own goroutine what a turn's goroutine panicked with, for the caller's defers to
// restore the terminal.
func (t *Terminal) Run(ctx context.Context, r platformagent.Runner) error {
	t.r, t.ctx = r, ctx
	defer r.Close()
	defer close(t.closed)
	defer t.sc.Close()

	input := make(chan []byte)
	inputEnd := make(chan error, 1)
	go t.read(input, inputEnd)

	var resize <-chan time.Time
	if t.cfg.Interactive {
		tick := time.NewTicker(resizeEvery)
		defer tick.Stop()
		resize = tick.C
	}
	t.refresh()
	t.sc.Line(t.sc.paint(styleDim, "Claude Platform: /help for commands"))
	if t.cfg.FirstPrompt != "" {
		t.submit(t.cfg.FirstPrompt)
	}
	t.update()

	var escC <-chan time.Time
	var stop <-chan struct{} = ctx.Done()
	for !t.finished {
		select {
		case b := <-input:
			keys := t.parser.Feed(b)
			escC = nil
			if t.parser.Pending() {
				escC = time.After(t.cfg.EscWait)
			}
			t.keys(keys)
		case <-escC:
			escC = nil
			t.keys(t.parser.Flush())
		case err := <-inputEnd:
			inputEnd = nil
			t.inputEnded(err)
		case <-t.wake:
			t.drain()
		case <-t.numberC:
			t.numberC = nil
			if a, ok := t.currentAsk().(*questionAsk); ok {
				a.timeUp(t)
			}
		case <-resize:
			// drawLive redraws at a new width.
		case <-t.exitC:
			t.finished = true
		case <-stop:
			stop = nil
			t.quit()
		}
		t.update()
	}
	return nil
}

// read hands over what In gives, until it ends.
func (t *Terminal) read(input chan<- []byte, end chan<- error) {
	buf := make([]byte, 4096)
	for {
		n, err := t.cfg.In.Read(buf)
		if n > 0 {
			select {
			case input <- append([]byte(nil), buf[:n]...):
			case <-t.closed:
				return
			}
		}
		if err != nil {
			end <- err
			return
		}
	}
}

// inputEnded is In ending: a terminal gone is the end, but piped text only says no more
// follows what it asked.
func (t *Terminal) inputEnded(err error) {
	t.keys(t.parser.Flush())
	if t.cfg.Interactive {
		t.quit()
		return
	}
	t.eof = true
	if !t.ed.empty() {
		t.enter() // a last line with no line break
	}
	for _, a := range t.asks {
		a.noAnswer(t)
	}
	t.asks = nil
	if t.job == nil && len(t.queue) == 0 {
		t.quit()
	}
}

// quit leaves as soon as the running job has stopped, or exitWait passed.
func (t *Terminal) quit() {
	if t.exiting {
		return
	}
	t.exiting = true
	if t.job == nil {
		t.finished = true
		return
	}
	t.stopJob()
	t.exitC = time.After(exitWait)
}

// update brings the live region up to date and writes what the last step changed.
func (t *Terminal) update() {
	t.sc.footer = t.footerText()
	t.sc.marker = t.marker()
	t.sc.input, t.sc.cursor = t.ed.text, t.ed.cur
	t.sc.Flush()
}

// marker is the prompt's mark: ❯ only when a message typed starts a turn, for a quiet
// session's screen to read as ready.
func (t *Terminal) marker() string {
	if !t.cfg.Interactive {
		return ""
	}
	switch {
	case t.currentAsk() != nil:
		return "? "
	case t.job != nil:
		return "+ "
	}
	return "❯ "
}

// keys handles what one read gave.
func (t *Terminal) keys(ks []Key) {
	// A key and its Enter in one read count as that key alone.
	alone := len(ks) == 1 || len(ks) == 2 && ks[1].Kind == KeyEnter
	for i, k := range ks {
		if a := t.currentAsk(); a != nil {
			more := i < len(ks)-1 && ks[i+1].Kind != KeyEnter
			a.key(t, k, !alone, more)
			continue
		}
		t.lineKey(k)
	}
}

// lineKey is a key for the line and what runs, nothing being asked.
func (t *Terminal) lineKey(k Key) {
	if k.Kind != KeyCtrlC {
		t.ctrlCAt = time.Time{}
	}
	switch k.Kind {
	case KeyEnter:
		t.enter()
	case KeyEsc:
		if t.job != nil {
			t.stopJob()
			return
		}
		t.ed.clear()
	case KeyCtrlC:
		t.ctrlC()
	case KeyCtrlD:
		if t.ed.empty() {
			t.quit()
			return
		}
		t.ed.apply(k)
	case KeyShiftTab:
		t.cycleMode()
	case KeyRedraw:
		t.sc.erased = false
		t.sc.shown = ""
	default:
		t.ed.apply(k)
	}
}

// enter sends the line.
func (t *Terminal) enter() {
	text := t.ed.String()
	t.ed.remember()
	t.submit(text)
}

// ctrlC interrupts what runs and forgets what was queued, or clears the line; pressed
// again soon, it leaves.
func (t *Terminal) ctrlC() {
	now := t.cfg.Now()
	again := !t.ctrlCAt.IsZero() && now.Sub(t.ctrlCAt) < exitPress
	t.ctrlCAt = now
	switch {
	case again:
		t.quit()
	case t.job != nil:
		if n := len(t.queue); n > 0 {
			t.queue = nil
			t.sc.Line(t.sc.paint(styleDim, fmt.Sprintf("Dropped %d queued", n)))
		}
		t.stopJob()
	case !t.ed.empty():
		t.ed.clear()
		t.ctrlCAt = time.Time{}
	default:
		t.sc.Line(t.sc.paint(styleDim, "Press Ctrl+C again to exit"))
	}
}

// currentAsk is what the user is being asked now.
func (t *Terminal) currentAsk() asker {
	if len(t.asks) == 0 {
		return nil
	}
	return t.asks[0]
}
