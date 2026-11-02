package daemon

import (
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/terminal"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A stream checks its mirror for changes every activePeriod while the
// screen changes, and every idlePeriod once it has been still for
// idleAfter, and every restPeriod once still for restAfter: each check
// copies the whole screen, which adds up over many idle sessions. Typing
// and resizing wake it at once, and typing checks every echoPeriod for
// echoWindow, to catch the echo as soon as the program writes it; only
// output that starts on its own after a quiet spell (an agent finishing a
// long step) may wait for the idle or rest period. restPeriod stays under
// session's streamFresh, so the session counts on these checks.
const (
	echoPeriod   = 2 * time.Millisecond
	echoWindow   = 30 * time.Millisecond
	activePeriod = 16 * time.Millisecond
	idlePeriod   = 50 * time.Millisecond
	idleAfter    = 500 * time.Millisecond
	restPeriod   = 250 * time.Millisecond
	restAfter    = 5 * time.Second
)

// stream sends one session's frames to one connection. Its pump goroutine
// is the only one that sends its frames, and it announces every change of
// the screen's size before the frames drawn at it, whoever resized it
// (this client or another one attached to the same session).
type stream struct {
	id     uint32
	conn   *conn
	sess   *session.Session
	mirror *terminal.Mirror
	wake   chan struct{}
	done   chan struct{}
	once   sync.Once
}

func newStream(c *conn, id uint32, sess *session.Session, m *terminal.Mirror) *stream {
	return &stream{
		id: id, conn: c, sess: sess, mirror: m,
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}
}

// input types into the session and wakes the pump for the echo.
func (st *stream) input(p []byte) {
	st.conn.srv.sessions.Input(st.sess, p)
	st.poke()
}

// resize resizes the session's terminal; the pump announces the new size.
func (st *stream) resize(p proto.ResizeParams) {
	st.sess.Terminal().ResizePixels(p.Cols, p.Rows, p.CellWidth, p.CellHeight)
	st.poke()
}

func (st *stream) poke() {
	select {
	case st.wake <- struct{}{}:
	default:
	}
}

func (st *stream) pump() {
	defer st.mirror.Close()
	timer := time.NewTimer(0)
	defer timer.Stop()

	term := st.sess.Terminal()
	sentCols, sentRows := -1, -1
	lastChange, lastWake := time.Now(), time.Time{}
	for {
		select {
		case <-st.done:
			return
		case <-st.sess.Done():
			return // replaced: the app attaches to the new run (SessionInfo.Run)
		case <-timer.C:
		case <-st.wake:
			lastChange, lastWake = time.Now(), time.Now()
		}
		if cols, rows := term.Size(); cols != sentCols || rows != sentRows {
			sentCols, sentRows = cols, rows
			st.conn.send(proto.Frame{Type: proto.TypeResize, Channel: st.id, Payload: proto.EncodeSize(cols, rows)})
		}
		frame, due := st.mirror.Frame()
		if len(frame) > 0 {
			st.conn.send(proto.Frame{Type: proto.TypeOutput, Channel: st.id, Payload: frame})
		}
		changed := len(frame) > 0 || due
		if changed {
			lastChange = time.Now()
		}
		st.sess.NoteFrame(changed)
		period := activePeriod
		switch {
		case time.Since(lastWake) < echoWindow:
			period = echoPeriod
		case time.Since(lastChange) > restAfter:
			period = restPeriod
		case time.Since(lastChange) > idleAfter:
			period = idlePeriod
		}
		timer.Reset(period)
	}
}

func (st *stream) stop() {
	st.once.Do(func() { close(st.done) })
}
