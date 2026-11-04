package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// What is sent to a session whose agent isn't running brings it back on its
// conversation first, as last it ran (keptArgs), rather than going to a terminal
// nothing reads. Those sending while it comes back wait in line, so each goes in once,
// in the order they came, once its agent can take it.

// sessionWakes are the sessions coming back. Safe from any goroutine; nothing is
// called holding mu but the look at a session.
type sessionWakes struct {
	mu    sync.Mutex
	lines map[string]*wakeLine
}

// wakeLine is one session coming back and those waiting to send to it.
type wakeLine struct {
	woke   bool          // its agent had stopped: false for one only queued
	ready  chan struct{} // closed once its agent is back, or can't be
	undone chan struct{} // closed if the session is put to sleep again meanwhile
	err    error         // why it can't, set before ready closes
	last   chan struct{} // closed once the last in line has sent
	users  int           // those in line, and the bringing back itself
}

// open starts a line for session id. The caller holds mu.
func (w *sessionWakes) open(id string, woke bool) *wakeLine {
	if w.lines == nil {
		w.lines = map[string]*wakeLine{}
	}
	nobody := make(chan struct{})
	close(nobody)
	l := &wakeLine{woke: woke, ready: make(chan struct{}), undone: make(chan struct{}), last: nobody, users: 1}
	w.lines[id] = l
	return l
}

func (w *sessionWakes) release(id string, l *wakeLine) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if l.users--; l.users == 0 && w.lines[id] == l {
		delete(w.lines, id)
	}
}

// undo ends session id's line, as it was put to sleep again while coming back: the wait
// for its agent says nothing now, and what is sent from here on brings it back anew.
func (w *sessionWakes) undo(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if l := w.lines[id]; l != nil {
		delete(w.lines, id)
		close(l.undone)
	}
}

// finish says session id came back, or err why it didn't.
func (w *sessionWakes) finish(id string, l *wakeLine, err error) {
	l.err = err
	close(l.ready)
	w.release(id, l)
}

// comesBack says a session's agent can come back on its conversation: not a shell,
// nor a program enowX can't continue.
func comesBack(meta session.Meta) bool {
	preset, ok := agent.Find(meta.Agent)
	return ok && len(preset.Continue) > 0 && meta.Relaunch
}

// wakeThen runs send once session id's agent can take it, and says whether it had to
// be brought back. It waits for that, so it isn't for a connection's own goroutine
// (needsWake); with its agent running it sends at once.
func (s *Server) wakeThen(id string, send func() error) (bool, error) {
	w := &s.wakes
	w.mu.Lock()
	l := w.lines[id]
	if l == nil {
		sess, err := s.sessions.Get(id)
		if err != nil {
			w.mu.Unlock()
			return false, err
		}
		info := sess.Info()
		if !info.Exited && !info.Queued {
			w.mu.Unlock()
			return false, send()
		}
		if info.Exited && !comesBack(sess.Meta()) {
			w.mu.Unlock()
			return false, fmt.Errorf("session %s ended and can't come back on a conversation (a shell, or a program enowX can't continue), so nothing was sent: start a new session", id)
		}
		l = w.open(id, info.Exited)
		go s.bringBack(id, l)
	}
	before, mine := l.last, make(chan struct{})
	l.last = mine
	l.users++
	w.mu.Unlock()
	defer w.release(id, l)
	defer close(mine)
	select {
	case <-l.ready:
	case <-l.undone:
	}
	<-before
	select {
	case <-l.undone:
		// Mine is still closed to those behind, who stay behind.
		if _, err := s.wakeThen(id, send); err != nil {
			return false, err
		}
		return true, nil
	default:
	}
	if l.err != nil {
		return false, fmt.Errorf("session %s couldn't be brought back, so nothing was sent: %w", id, l.err)
	}
	return l.woke, send()
}

// bringBack has session id's agent back and ready, for line l.
func (s *Server) bringBack(id string, l *wakeLine) {
	var err error
	if l.woke {
		_, err = s.continueSession(id)
	}
	if err == nil {
		err = s.waitSessionReady(id)
	}
	s.wakes.finish(id, l, err)
}

// wakeSession brings session id back as asked to, not waiting for its agent, while
// what others send meanwhile waits for it; one already coming back is left to.
func (s *Server) wakeSession(id string) (*session.Session, error) {
	w := &s.wakes
	w.mu.Lock()
	if w.lines[id] != nil {
		w.mu.Unlock()
		return s.sessions.Get(id)
	}
	sess, err := s.sessions.Get(id)
	if err != nil || !sess.Info().Exited {
		w.mu.Unlock()
		return s.continueSession(id) // says why not
	}
	l := w.open(id, true)
	w.mu.Unlock()
	next, err := s.continueSession(id)
	if err != nil {
		w.finish(id, l, err)
		return nil, err
	}
	go func() { w.finish(id, l, s.waitSessionReady(id)) }()
	return next, nil
}

// needsWake says what req sends to a session would have to wait for its agent, so it
// runs beside the connection's other requests. Typing to a running one stays in order.
func (s *Server) needsWake(req proto.Request) bool {
	if req.Method != proto.MethodSessionInput && req.Method != proto.MethodSessionAnswer {
		return false
	}
	var ref proto.SessionRef
	return json.Unmarshal(req.Params, &ref) == nil && s.notRunning(ref.ID)
}

// notRunning says session id's agent isn't there to take anything yet.
func (s *Server) notRunning(id string) bool {
	w := &s.wakes
	w.mu.Lock()
	coming := w.lines[id] != nil
	w.mu.Unlock()
	if coming {
		return true
	}
	sess, err := s.sessions.Get(id)
	if err != nil {
		return false
	}
	info := sess.Info()
	return info.Exited || info.Queued
}

// tellSession sends a session what the daemon has for it, in the background when its
// agent has to be brought back first; what stops it is logged as what.
func (s *Server) tellSession(p proto.SessionInputParams, what string) {
	send := func() {
		if _, err := s.wakeThen(p.ID, func() error { return s.input(p) }); err != nil {
			s.logger().Warn(what, "session", p.ID, "err", err)
		}
	}
	if s.notRunning(p.ID) {
		go send()
		return
	}
	send()
}

// errNotAnswer is an answer no agent could take, whatever it asked.
var errNotAnswer = errors.New("answer: allow, always, deny, or a question's option number")

// asleepAnswer is what session id's agent is told of an answer to a question that went
// with the program that asked it: it comes back on its conversation, not its prompt.
func asleepAnswer(answer string) (string, error) {
	answer = strings.ToLower(strings.TrimSpace(answer))
	if _, err := strconv.Atoi(answer); err != nil && answer != proto.AnswerAllow && answer != proto.AnswerAlways && answer != proto.AnswerDeny {
		return "", errNotAnswer
	}
	return "This session stopped and came back while you were asking something, so the prompt is gone. The answer you were given to it: " + answer + ". Go on from there, asking again what still needs a yes.", nil
}
