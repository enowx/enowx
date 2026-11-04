package daemon

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hooks"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// Workers' lines reach GOD of Agents without its input box: a line typed there took
// the user's unfinished sentence along with it. By what each agent's documentation
// allows, in this order:
//
//  1. In a turn: its hooks' context, at the next prompt or tool call's end (Claude Code
//     and Codex).
//  2. At a turn's end: its Stop hook keeps the turn going to take them (both).
//  3. Idle: Claude Code's background hook, which wakes it on exit code 2. Codex
//     documents no way in to an idle thread.
//  4. Without one of those channels, reports stay in Tasks. A screen snapshot
//     cannot reserve a composer's input against concurrent user typing.
//
// Nothing here changes what the user is typing, has sent or is editing.

const (
	// goaContextMost is how much goes at once in an agent's context, under what agents
	// take (Claude Code 10,000 characters, Codex about 2,500 tokens); the rest follows.
	goaContextMost = 8000
)

func init() { slowMethods[proto.MethodGOAWake] = true } // it waits, on a connection of its own

// goaWaker is a hook waiting to wake GOA's idle agent.
type goaWaker struct {
	text  chan string
	lines []goaLine // original metadata, guarded by delivery; returned on cancellation
}

// goaWake waits for lines for session id, for its agent's background hook, and takes
// over from any hook before it: until lines come, a later hook comes, the connection
// is gone or the daemon stops.
func (s *Server) goaWake(id string, gone <-chan struct{}) proto.GOAWakeReply {
	g, ok := s.goaSession()
	if !ok || g.ID != id {
		return proto.GOAWakeReply{}
	}
	w := &goaWaker{text: make(chan string, 1)}
	s.goa.delivery.Lock()
	s.goa.mu.Lock()
	old := s.goa.waker
	s.goa.waker = w
	s.goa.mu.Unlock()
	if old != nil {
		old.text <- ""
	}
	s.goa.delivery.Unlock()
	s.goaFlushIdle() // what waited for just this
	select {
	case text := <-w.text:
		return proto.GOAWakeReply{Text: text}
	case <-gone:
	case <-s.done:
	}
	s.goaReturnWake(w)
	return proto.GOAWakeReply{}
}

// goaReturnWake puts back a handoff the disconnected hook did not consume.
func (s *Server) goaReturnWake(w *goaWaker) {
	s.goa.delivery.Lock()
	defer s.goa.delivery.Unlock()
	s.goa.mu.Lock()
	if s.goa.waker == w {
		s.goa.waker = nil
	}
	select {
	case text := <-w.text:
		// It was given lines as it went: they go back, not lost.
		if text != "" {
			s.goa.tell = uniqueGOALines(append(w.lines, s.goa.tell...))
		}
	default:
	}
	s.goa.mu.Unlock()
	s.saveGOATell()
}

// goaFlush gives what waits for GOA (session id) to its idle agent: through the hook
// waiting to wake it. Without a safe channel, the queue stays visible in Tasks.
func (s *Server) goaFlush(id string) {
	s.goa.delivery.Lock()
	defer s.goa.delivery.Unlock()
	_, err := s.sessions.Get(id)
	if err != nil {
		return
	}
	s.goa.mu.Lock()
	w := s.goa.waker
	s.goa.mu.Unlock()
	if w == nil {
		return // never paste or press Enter in the user's composer
	}
	lines := s.goaLinesToSay()
	if len(lines) == 0 {
		s.saveGOATell() // moot lines may have gone
		return
	}
	s.goa.mu.Lock()
	s.goa.waker = nil
	w.lines = lines
	s.goa.mu.Unlock()
	w.text <- joinLines(lines)
	s.saveGOATell()
}

func joinLines(lines []goaLine) string {
	texts := make([]string, len(lines))
	for i, l := range lines {
		texts[i] = l.Text
	}
	return strings.Join(texts, "\n")
}

// goaTake takes what fits an agent's context of what waits for GOA, the rest staying.
func (s *Server) goaTake() string {
	s.goa.delivery.Lock()
	defer s.goa.delivery.Unlock()
	text, _ := s.goaTakeOf(s.goaLinesToSay())
	return text
}

// goaTakeOf takes what fits of lines, the rest going back, and says how many it took.
func (s *Server) goaTakeOf(lines []goaLine) (string, int) {
	size, n := 0, 0
	for n < len(lines) && (n == 0 || size+len(lines[n].Text) < goaContextMost) {
		size += len(lines[n].Text) + 1
		n++
	}
	if n < len(lines) {
		s.goa.mu.Lock()
		s.goa.tell = uniqueGOALines(append(append([]goaLine{}, lines[n:]...), s.goa.tell...))
		s.goa.mu.Unlock()
	}
	s.saveGOATell()
	if n == 0 {
		return "", 0
	}
	text := joinLines(lines[:n])
	if r := []rune(text); len(r) > goaContextMost {
		text = string(r[:goaContextMost]) + "…"
	}
	return text, n
}

// goaHookLines gives GOA's agent what waits for it in the hook it is in, and says a
// turn's end goes on to take it.
func (s *Server) goaHookLines(sess *session.Session, info proto.SessionInfo, ev proto.HookEvent, d *proto.HookDecision) bool {
	if !info.GOA || d.Block {
		return false
	}
	switch ev.Event {
	case proto.HookToolAfter, proto.HookPromptStart:
		if text := s.goaTake(); text != "" {
			d.Context = strings.TrimSpace(d.Context + "\n\n" + text)
			s.goaChanged()
		}
	case proto.HookTurnDone:
		if text := s.goaTakeAtStop(); text != "" {
			d.Continue = text
			_ = s.sessions.SetState(sess, proto.StateWorking)
			s.goaChanged()
			return true
		}
	}
	return false
}

// goaWaitingLines are the lines waiting for GOA, for the app to show.
func (s *Server) goaWaitingLines() []string {
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	if len(s.goa.tell) == 0 {
		return nil
	}
	out := make([]string, len(s.goa.tell))
	for i, l := range s.goa.tell {
		out[i] = l.Text
	}
	return out
}

// goaWakeRetry is how soon the background hook looks for the daemon again, once it
// restarted; it waits as long as its agent lets it.
const (
	goaWakeRetry = 5 * time.Second
	goaWakeMost  = 24 * time.Hour
)

// goaWakeHook is `enowx hook --wake`, GOA's agent's background hook: it waits for lines,
// and hands them over as its agent reads them: on stderr, with exit code 2.
func goaWakeHook(id string, stderr interface{ Write([]byte) (int, error) }) int {
	if id == "" {
		return 0
	}
	deadline := time.Now().Add(goaWakeMost)
	for time.Now().Before(deadline) {
		var reply proto.GOAWakeReply
		var err error
		answered := false
		withDaemon(time.Until(deadline), func(c *client.Client) {
			err = c.Call(proto.MethodGOAWake, proto.GOAWakeParams{ID: id}, &reply)
			answered = true
		})
		switch {
		case answered && err == nil && reply.Text != "":
			_, _ = stderr.Write([]byte(reply.Text + "\n"))
			return hooks.ExitBlock
		case answered && err == nil:
			return 0 // a later hook took over, or the session ended
		}
		time.Sleep(goaWakeRetry)
	}
	return 0
}

// hookEventName is which of its events an agent's hook ran for.
func hookEventName(data []byte, fallback string) string {
	var in struct {
		Event string `json:"hook_event_name"`
	}
	if json.Unmarshal(data, &in) == nil && in.Event != "" {
		return in.Event
	}
	return fallback
}

// contextOutput adds text to an agent's context, in the shape Claude Code and Codex
// both document for a tool's end.
func contextOutput(data []byte, text string) string {
	out, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName": hookEventName(data, "PostToolUse"), "additionalContext": text}})
	return string(out)
}

// continueOutput keeps a turn going, to take text. Claude Code takes it as context, not
// as a hook's error; Codex, known by the turn_id it alone sends, as a continuation.
func continueOutput(data []byte, text string) string {
	var in struct {
		TurnID string `json:"turn_id"`
	}
	_ = json.Unmarshal(data, &in)
	if in.TurnID != "" {
		out, _ := json.Marshal(map[string]string{"decision": "block", "reason": text})
		return string(out)
	}
	return contextOutput(data, text)
}
