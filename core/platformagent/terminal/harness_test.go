package terminal

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/core/platformagent/record"
)

// pipe is an input a test writes to.
func pipe(t *testing.T) (*io.PipeReader, *io.PipeWriter) {
	t.Helper()
	pr, pw := io.Pipe()
	t.Cleanup(func() { pw.Close() })
	return pr, pw
}

// fakeRunner is a Runner whose turns do what a test says.
type fakeRunner struct {
	mu                  sync.Mutex
	model, effort, mode string
	budget, cost        float64
	turns               []string
	files               [][]string
	cleared, closed     int
	compacted           int
	onTurn              func(ctx context.Context, text string) error
	onCompact           func(ctx context.Context) error
	failModel           error
}

func (f *fakeRunner) Turn(ctx context.Context, text string, files []string) error {
	f.mu.Lock()
	f.turns = append(f.turns, text)
	f.files = append(f.files, files)
	on := f.onTurn
	f.mu.Unlock()
	if on != nil {
		return on(ctx, text)
	}
	return nil
}

func (f *fakeRunner) Settings() (string, string, string, float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.model, f.effort, f.mode, f.budget
}

func (f *fakeRunner) SetModel(m string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failModel != nil {
		return f.failModel
	}
	f.model = m
	return nil
}

func (f *fakeRunner) SetEffort(e string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.effort = e
	return nil
}

func (f *fakeRunner) SetMode(m string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = m
	return nil
}

func (f *fakeRunner) SetBudget(b float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.budget = b
}

func (f *fakeRunner) Compact(ctx context.Context) error {
	f.mu.Lock()
	f.compacted++
	on := f.onCompact
	f.mu.Unlock()
	if on != nil {
		return on(ctx)
	}
	return nil
}

func (f *fakeRunner) Clear() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleared++
	return nil
}

func (f *fakeRunner) Cost() float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cost
}

func (f *fakeRunner) Context() (int64, int64) { return 50_000, 200_000 }
func (f *fakeRunner) Conversation() string    { return "conv" }

func (f *fakeRunner) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
	return nil
}

func (f *fakeRunner) turnsSeen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.turns...)
}

func (f *fakeRunner) setTurn(on func(ctx context.Context, text string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onTurn = on
}

// lockedBuffer is what the terminal wrote, read while it writes.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// session is a Terminal running in a test.
type session struct {
	t     *testing.T
	term  *Terminal
	r     *fakeRunner
	in    *io.PipeWriter
	out   *lockedBuffer
	ended chan error
	stop  context.CancelFunc
}

func start(t *testing.T, change func(*Config, *fakeRunner)) *session {
	t.Helper()
	pr, pw := io.Pipe()
	out := &lockedBuffer{}
	cfg := Config{In: pr, Out: out, EscWait: 5 * time.Millisecond, NumberWait: 20 * time.Millisecond}
	r := &fakeRunner{model: "claude-test", effort: "high", mode: record.ModeDefault, budget: 10}
	if change != nil {
		change(&cfg, r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{t: t, term: New(cfg), r: r, in: pw, out: out, ended: make(chan error, 1), stop: cancel}
	go func() { s.ended <- s.term.Run(ctx, r) }()
	t.Cleanup(func() {
		cancel()
		pw.Close()
		select {
		case <-s.ended:
		case <-time.After(5 * time.Second):
		}
	})
	return s
}

// press writes bytes as one read of the terminal's.
func (s *session) press(keys string) {
	s.t.Helper()
	if _, err := s.in.Write([]byte(keys)); err != nil {
		s.t.Fatal(err)
	}
}

// settle gives Run's goroutine time to read what was pressed before the next press, so
// the two come as separate reads.
func (s *session) settle() { time.Sleep(15 * time.Millisecond) }

// waitOut waits for the output to hold text.
func (s *session) waitOut(text string) {
	s.t.Helper()
	waitFor(s.t, "output holding "+strconv.Quote(text), func() bool { return strings.Contains(s.out.String(), text) }, func() string { return s.out.String() })
}

// waitEnded waits for Run to return.
func (s *session) waitEnded() {
	s.t.Helper()
	select {
	case err := <-s.ended:
		if err != nil {
			s.t.Fatal(err)
		}
		s.ended <- nil
	case <-time.After(5 * time.Second):
		s.t.Fatalf("Run didn't end; output:\n%s", s.out.String())
	}
}

func waitFor(t *testing.T, what string, ok func() bool, shows func() string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	got := ""
	if shows != nil {
		got = shows()
	}
	t.Fatalf("timed out waiting for %s; got:\n%s", what, got)
}

// vtScreen is the little of a terminal this package uses, for what the screen ends up
// showing: scrolling never drops a row, so it holds the scrollback too.
type vtScreen struct {
	cols     int
	rows     [][]rune
	row, col int
	pending  bool
}

func newVT(cols int) *vtScreen { return &vtScreen{cols: cols, rows: [][]rune{nil}} }

func (v *vtScreen) feed(s string) {
	in := []rune(s)
	for i := 0; i < len(in); i++ {
		r := in[i]
		switch {
		case r == '\r':
			v.col, v.pending = 0, false
		case r == '\n':
			v.down()
			v.pending = false
		case r == 0x1b && i+1 < len(in) && in[i+1] == '[':
			j := i + 2
			for j < len(in) && (in[j] < 0x40 || in[j] > 0x7e) {
				j++
			}
			n, _ := strconv.Atoi(strings.TrimPrefix(string(in[i+2:j]), "?"))
			v.csi(in[j], n)
			i = j
		default:
			v.put(r)
		}
	}
}

func (v *vtScreen) down() {
	v.row++
	for v.row >= len(v.rows) {
		v.rows = append(v.rows, nil)
	}
}

func (v *vtScreen) put(r rune) {
	if v.pending {
		v.col, v.pending = 0, false
		v.down()
	}
	line := v.rows[v.row]
	for len(line) <= v.col {
		line = append(line, ' ')
	}
	line[v.col] = r
	v.rows[v.row] = line
	v.col++
	if v.col == v.cols {
		v.col, v.pending = v.cols-1, true
	}
}

func (v *vtScreen) csi(final rune, n int) {
	switch final {
	case 'A':
		v.row = max(v.row-max(n, 1), 0)
		v.pending = false
	case 'G':
		v.col, v.pending = max(n-1, 0), false
	case 'J':
		if v.col < len(v.rows[v.row]) {
			v.rows[v.row] = v.rows[v.row][:v.col]
		}
		v.rows = v.rows[:v.row+1]
	}
}

// text is the screen as enowX's headless terminal gives it.
func (v *vtScreen) text() string {
	var lines []string
	for _, r := range v.rows {
		lines = append(lines, strings.TrimRight(string(r), " "))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
