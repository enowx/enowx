package daemon

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
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
//  4. Idle, with none of those (Codex, an agent with no hooks, a hook gone): typed in,
//     but only into a composer sure to be empty, with no keystroke for a few seconds.
//     Otherwise the line waits, shown in the app, for the next turn or that moment.
//
// Nothing here changes what the user is typing, has sent or is editing.

const (
	// goaTypedQuiet is how long since a keystroke before a line may be typed in.
	goaTypedQuiet = 5 * time.Second
	// goaContextMost is how much goes at once in an agent's context, under what agents
	// take (Claude Code 10,000 characters, Codex about 2,500 tokens); the rest follows.
	goaContextMost = 8000
)

func init() { slowMethods[proto.MethodGOAWake] = true } // it waits, on a connection of its own

// goaWaker is a hook waiting to wake GOA's idle agent.
type goaWaker struct{ text chan string }

// goaWake waits for lines for session id, for its agent's background hook, and takes
// over from any hook before it: until lines come, a later hook comes, the connection
// is gone or the daemon stops.
func (s *Server) goaWake(id string, gone <-chan struct{}) proto.GOAWakeReply {
	g, ok := s.goaSession()
	if !ok || g.ID != id {
		return proto.GOAWakeReply{}
	}
	w := &goaWaker{text: make(chan string, 1)}
	s.goa.mu.Lock()
	old := s.goa.waker
	s.goa.waker = w
	s.goa.mu.Unlock()
	if old != nil {
		old.text <- ""
	}
	s.goaFlushIdle() // what waited for just this
	select {
	case text := <-w.text:
		return proto.GOAWakeReply{Text: text}
	case <-gone:
	case <-s.done:
	}
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	if s.goa.waker == w {
		s.goa.waker = nil
	}
	select {
	case text := <-w.text:
		// It was given lines as it went: they go back, not lost.
		if text != "" {
			s.goa.tell = append([]goaLine{{Text: text}}, s.goa.tell...)
		}
	default:
	}
	return proto.GOAWakeReply{}
}

// goaFlush gives what waits for GOA (session id) to its idle agent: through the hook
// waiting to wake it, or typed only where that can't take anything of the user's.
func (s *Server) goaFlush(id string) {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return
	}
	lines := s.goaLinesToSay()
	if len(lines) == 0 {
		s.saveGOATell() // moot lines may have gone
		return
	}
	s.goa.mu.Lock()
	w := s.goa.waker
	if w != nil {
		s.goa.waker = nil
	}
	s.goa.mu.Unlock()
	switch {
	case w != nil:
		w.text <- joinLines(lines)
		lines = nil
	case goaCanType(sess, time.Now()):
		if err := s.input(proto.SessionInputParams{ID: id, Text: joinLines(lines), Paste: true, Enter: true}); err != nil {
			s.logger().Warn("goa: could not tell GOA", "session", id, "err", err)
		} else {
			lines = nil
		}
	}
	if len(lines) > 0 {
		s.goa.mu.Lock()
		s.goa.tell = append(lines, s.goa.tell...)
		s.goa.mu.Unlock()
	}
	s.saveGOATell()
}

func joinLines(lines []goaLine) string {
	texts := make([]string, len(lines))
	for i, l := range lines {
		texts[i] = l.Text
	}
	return strings.Join(texts, "\n")
}

// goaCanType says typing into GOA's session can't touch anything of the user's: no
// key pressed for a few seconds, nothing at work or asked, a composer sure to be empty.
func goaCanType(sess *session.Session, now time.Time) bool {
	info := sess.Info()
	return !info.Exited && info.State != proto.StateWorking && info.State != proto.StateWaiting &&
		sess.TypedFor(now) >= goaTypedQuiet && agent.PromptEmpty(sess.Terminal().Text())
}

// goaTake takes what fits an agent's context of what waits for GOA, the rest staying.
func (s *Server) goaTake() string {
	lines := s.goaLinesToSay()
	size, n := 0, 0
	for n < len(lines) && (n == 0 || size+len(lines[n].Text) < goaContextMost) {
		size += len(lines[n].Text) + 1
		n++
	}
	if n < len(lines) {
		s.goa.mu.Lock()
		s.goa.tell = append(append([]goaLine{}, lines[n:]...), s.goa.tell...)
		s.goa.mu.Unlock()
	}
	s.saveGOATell()
	if n == 0 {
		return ""
	}
	text := joinLines(lines[:n])
	if r := []rune(text); len(r) > goaContextMost {
		text = string(r[:goaContextMost]) + "…"
	}
	return text
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
		if text := s.goaTake(); text != "" {
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
