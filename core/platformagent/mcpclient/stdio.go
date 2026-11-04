package mcpclient

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/enowx/enowx/internal/quietcmd"
)

// How long a server gets to leave by itself on Close: stdin's end, then a request to
// terminate, then a kill.
const (
	exitAfterStdin = 500 * time.Millisecond
	exitAfterTerm  = time.Second
	// strayOutputWait is how long a stdout some escaped child of the server's still
	// holds is read after the server exits.
	strayOutputWait = time.Second
)

// stdioTransport is a server program, a JSON message a line on its stdin and stdout.
// Its stderr is drained and logged nowhere; only its last line is kept, masked when read,
// to say why the server stopped.
//
// Goroutines: writeLoop owns stdin's writes, readLoop owns stdout, drainStderr owns
// stderr, wait reaps the process and its group.
type stdioTransport struct {
	cfg    serverConfig
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	out    chan []byte
	in     chan []byte
	stderr stderrTail
	// drained closes when stderr has been read to its end.
	drained chan struct{}
	// exited closes when the process has been reaped, its group killed, stderr drained.
	exited chan struct{}
	// over closes when every line of stdout has been handed over.
	over      chan struct{}
	exitState error // set before exited closes
	readErr   error // set before over closes
	closeOnce sync.Once
}

func startStdio(cfg serverConfig) (*stdioTransport, error) {
	cmd := quietcmd.Command(cfg.Command, cfg.Args...)
	cmd.Env = cfg.environ
	ownGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("can't run %s: %w", cfg.Command, err)
	}
	// Pipes of our own, not StdoutPipe: Wait closes that one, losing what a server said
	// just before it exited.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("can't run %s: %w", cfg.Command, err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		return nil, fmt.Errorf("can't run %s: %w", cfg.Command, err)
	}
	cmd.Stdout, cmd.Stderr = stdoutW, stderrW
	err = cmd.Start()
	_ = stdoutW.Close()
	_ = stderrW.Close()
	if err != nil {
		_ = stdin.Close()
		_ = stdoutR.Close()
		_ = stderrR.Close()
		return nil, fmt.Errorf("can't run %s: %w", cfg.Command, err)
	}
	t := &stdioTransport{
		cfg: cfg, cmd: cmd, stdin: stdin,
		out: make(chan []byte), in: make(chan []byte),
		drained: make(chan struct{}), exited: make(chan struct{}), over: make(chan struct{}),
	}
	go t.drainStderr(stderrR)
	go t.wait(stdoutR, stderrR)
	go t.writeLoop()
	go t.readLoop(stdoutR)
	return t, nil
}

func (t *stdioTransport) drainStderr(stderr *os.File) {
	defer close(t.drained)
	_, _ = io.Copy(&t.stderr, stderr)
}

func (t *stdioTransport) wait(stdout, stderr *os.File) {
	t.exitState = t.cmd.Wait()
	killGroup(t.cmd) // what it started goes with it
	// An escaped child can hold the pipes open: closing ours ends the reads regardless.
	time.AfterFunc(strayOutputWait, func() {
		_ = stdout.Close()
		_ = stderr.Close()
	})
	<-t.drained
	close(t.exited)
}

func (t *stdioTransport) writeLoop() {
	for {
		select {
		case msg := <-t.out:
			// A stuck write ends with the process: Wait closes stdin.
			if _, err := t.stdin.Write(msg); err != nil {
				killGroup(t.cmd) // it isn't reading: of no use from here
				return
			}
		case <-t.exited:
			return
		}
	}
}

func (t *stdioTransport) readLoop(stdout *os.File) {
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 64<<10), maxMessage)
	for lines.Scan() {
		line := bytes.TrimSpace(lines.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue // a log line on the wrong stream
		}
		t.in <- bytes.Clone(line)
	}
	if err := lines.Err(); errors.Is(err, bufio.ErrTooLong) {
		t.readErr = fmt.Errorf("it sent a message over %d MB", maxMessage>>20)
	}
	killGroup(t.cmd) // stdout's end leaves it speechless, whatever it is doing
	<-t.exited
	close(t.over)
}

func (t *stdioTransport) send(ctx context.Context, msg []byte, _ int64) error {
	line := append(bytes.Clone(msg), '\n')
	select {
	case t.out <- line:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.exited:
		return &goneError{why: t.exitText()}
	}
}

func (t *stdioTransport) inbound() <-chan []byte { return t.in }
func (t *stdioTransport) ended() <-chan struct{} { return t.over }
func (t *stdioTransport) negotiated(string)      {}

func (t *stdioTransport) why() string {
	if t.readErr != nil {
		return t.readErr.Error()
	}
	return t.exitText()
}

func (t *stdioTransport) exitText() string {
	text := "it exited"
	if t.exitState != nil {
		text += ": " + t.exitState.Error()
	}
	if line := t.cfg.hideOutput(t.stderr.lastLine()); line != "" {
		text += ": " + oneLine(line, maxStderrLine)
	}
	return text
}

// maxStderrLine is how much of a server's last line of stderr a message repeats.
const maxStderrLine = 200

// stderrTail keeps the end of what a server printed.
type stderrTail struct {
	mu   sync.Mutex
	tail []byte
}

func (s *stderrTail) Write(p []byte) (int, error) {
	const keep = 4 << 10
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tail = append(s.tail, p...)
	if len(s.tail) > keep {
		s.tail = bytes.Clone(s.tail[len(s.tail)-keep:])
	}
	return len(p), nil
}

// lastLine is the last line with something on it.
func (s *stderrTail) lastLine() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := bytes.Split(s.tail, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if line := bytes.TrimSpace(lines[i]); len(line) > 0 {
			return string(line)
		}
	}
	return ""
}

func (t *stdioTransport) close() {
	t.closeOnce.Do(func() {
		_ = t.stdin.Close()
		if t.exitsWithin(exitAfterStdin) {
			return
		}
		terminateGroup(t.cmd)
		if t.exitsWithin(exitAfterTerm) {
			return
		}
		killGroup(t.cmd)
		<-t.exited
	})
}

func (t *stdioTransport) exitsWithin(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-t.exited:
		return true
	case <-timer.C:
		return false
	}
}
