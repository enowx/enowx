// Package diag keeps what explains a problem later: a log of events for the app and
// one for the daemon, with their rotation, the removal of old logs, and the bundle a
// bug report carries. A log holds names, IDs, sizes and errors, never what a session
// or a canvas holds, and anything that looks like a secret is masked before it is
// written (scrub.go). Writing never blocks the caller: a line that finds the queue
// full is counted and dropped.
package diag

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// AppLog and DaemonLog are the logs' names in the data folder.
	AppLog    = "app.log"
	DaemonLog = "enowxd.log"
	// MaxFile is how large a log grows before it rotates, KeepRotated how many
	// rotated copies stay (NAME.1 is the most recent).
	MaxFile     = 5 << 20
	KeepRotated = 3
	// queueSize is how many lines wait for the writer before they are dropped.
	queueSize = 1024
	// lastKept is how many events' names Doing says.
	lastKept = 6
	// exitMark ends the log of a run that ended well.
	exitMark = "exit.clean"
)

// Log is one program's log. A nil Log does nothing, for code that runs without one.
type Log struct {
	path    string
	maxFile int64
	dir     string
	lines   chan string
	flush   chan chan struct{}
	stop    chan struct{}
	stopped chan struct{}
	once    sync.Once
	dropped atomic.Int64

	mu    sync.Mutex
	every map[string]time.Time
	last  []string
}

// Open starts logging to name in the data folder dir. A log an older version left
// over the limit is cut down first.
func Open(dir, name string) *Log {
	l := &Log{path: filepath.Join(dir, name), dir: dir, maxFile: MaxFile, lines: make(chan string, queueSize),
		flush: make(chan chan struct{}), stop: make(chan struct{}), stopped: make(chan struct{}), every: map[string]time.Time{}}
	_ = os.MkdirAll(dir, 0o700)
	go l.write()
	return l
}

// Path is the log's file.
func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Event logs what happened: its name, then key and value pairs, of which only
// strings, errors, numbers, booleans and durations are written.
func (l *Log) Event(name string, kv ...any) {
	if l == nil {
		return
	}
	l.remember(name)
	l.enqueue(time.Now().Format(time.RFC3339) + " " + name + formatPairs(kv) + "\n")
}

// Every logs an event at most once in a while for key.
func (l *Log) Every(key string, every time.Duration, name string, kv ...any) {
	if l == nil {
		return
	}
	now := time.Now()
	l.mu.Lock()
	allowed := now.Sub(l.every[key]) >= every
	if allowed {
		l.every[key] = now
	}
	if len(l.every) > 512 { // keys come from a few names: this is a backstop
		clear(l.every)
	}
	l.mu.Unlock()
	if allowed {
		l.Event(name, kv...)
	}
}

// Write takes finished lines, a logger's, as a writer: scrubbed, never blocking.
func (l *Log) Write(p []byte) (int, error) {
	if l != nil {
		line := Scrub(string(p))
		if !strings.HasSuffix(line, "\n") {
			line += "\n"
		}
		l.enqueue(line)
	}
	return len(p), nil
}

func (l *Log) enqueue(line string) {
	select {
	case l.lines <- line:
	default:
		l.dropped.Add(1)
	}
}

func (l *Log) remember(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.last = append(l.last, name)
	if len(l.last) > lastKept {
		l.last = l.last[len(l.last)-lastKept:]
	}
}

// Doing is the last events' names, the latest last: what was happening, for a mark.
func (l *Log) Doing() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.last, ",")
}

// Flush returns once what was logged before it is in the file.
func (l *Log) Flush() {
	if l == nil {
		return
	}
	ch := make(chan struct{})
	select {
	case l.flush <- ch:
		<-ch
	case <-l.stopped:
	}
}

// Close writes what is left and ends the log.
func (l *Log) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() { close(l.stop) })
	<-l.stopped
}

// write owns the file.
func (l *Log) write() {
	defer close(l.stopped)
	if size, err := fileSize(l.path); err == nil && size > l.maxFile {
		_ = rotate(l.path, l.dir) // a log from before rotation, however large
	}
	f, size := l.open()
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	put := func(line string) {
		if f == nil {
			if f, size = l.open(); f == nil {
				return
			}
		}
		if n := l.dropped.Swap(0); n > 0 {
			line = time.Now().Format(time.RFC3339) + " log.dropped lines=" + strconv.FormatInt(n, 10) + "\n" + line
		}
		n, _ := f.WriteString(line)
		size += int64(n)
		if size > l.maxFile && rotate(l.path, l.dir) == nil {
			// In place: others may hold the file open to append (the daemon's output).
			size = 0
		}
	}
	drain := func() {
		for {
			select {
			case line := <-l.lines:
				put(line)
			default:
				return
			}
		}
	}
	for {
		select {
		case line := <-l.lines:
			put(line)
		case ch := <-l.flush:
			drain()
			close(ch)
		case <-l.stop:
			drain()
			return
		}
	}
}

func (l *Log) open() (*os.File, int64) {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, 0
	}
	size := int64(0)
	if info, err := f.Stat(); err == nil {
		size = info.Size()
	}
	return f, size
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// rotate moves the end of the log at path to NAME.1, the older copies one along, and
// empties the log. A bundle reading them defers it.
func rotate(path, dir string) error {
	if Held(dir) {
		return errHeld
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", path, KeepRotated))
	for i := KeepRotated - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", path, i), fmt.Sprintf("%s.%d", path, i+1))
	}
	if err := copyTail(path, path+".1", MaxFile); err != nil {
		return err
	}
	return os.Truncate(path, 0)
}

func copyTail(from, to string, most int64) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	start := max(info.Size()-most, 0)
	buf := make([]byte, info.Size()-start)
	if _, err := in.ReadAt(buf, start); err != nil {
		return err
	}
	if start > 0 { // from a whole line
		if i := strings.IndexByte(string(buf[:min(len(buf), 1<<16)]), '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	return os.WriteFile(to, buf, 0o600)
}

// formatPairs is kv as " key=value" text.
func formatPairs(kv []any) string {
	var b strings.Builder
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(formatValue(kv[i+1]))
	}
	return b.String()
}

// maxValue bounds one value, an error's most of all.
const maxValue = 400

func formatValue(v any) string {
	var s string
	switch v := v.(type) {
	case nil:
		return `""`
	case string:
		s = v
	case error:
		s = v.Error()
	case time.Duration:
		return v.Round(time.Millisecond).String()
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	default:
		return "?" // what isn't known to be plain isn't written: it may hold content
	}
	s = Scrub(s)
	if len(s) > maxValue {
		s = s[:maxValue] + "…"
	}
	if s == "" || strings.ContainsAny(s, " \t\n\"=") {
		return strconv.Quote(s)
	}
	return s
}

// Start logs that a program started, whether its last run ended well, and has a
// crash's stack written to the log (a crash writes it directly, as nothing else runs
// by then). kv says more of it.
func (l *Log) Start(name, version, build string, kv ...any) {
	if l == nil {
		return
	}
	last := "first"
	if end := lastLine(l.path); end != "" {
		last = "unclean"
		if strings.Contains(end, exitMark) {
			last = "clean"
		}
	}
	l.Event(name+".start", append([]any{"version", version, "build", build, "pid", os.Getpid(), "lastRun", last}, kv...)...)
	l.Flush()
	if f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		_ = debug.SetCrashOutput(f, debug.CrashOptions{}) // keeps its own copy of f
		f.Close()
	}
}

// End logs that a program is ending as it should, and writes what is left.
func (l *Log) End(name string) {
	if l == nil {
		return
	}
	l.Event(exitMark, "program", name)
	l.Close()
}

func lastLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return ""
	}
	size := min(info.Size(), 4096)
	buf := make([]byte, size)
	if _, err := f.ReadAt(buf, info.Size()-size); err != nil {
		return ""
	}
	text := strings.TrimRight(string(buf), "\n")
	return text[strings.LastIndexByte(text, '\n')+1:]
}
