package session

import (
	"bytes"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// DeliveryGuard binds a delivery to the input and hook history of one program.
// Its fields are private so only successful queueing can advance it.
type DeliveryGuard struct {
	input uint64
	epoch uint64
}

// Retain observed unsafe screens even if a subsequent redraw hides them.
func (s *Session) noteDeliveryScreenLocked() {
	if s.deliveryPending && !agent.PromptFree(s.term.Text()) {
		s.deliveryEpoch++
	}
}

// DeliveryGuard captures cancellation history before inspecting readiness.
func (s *Session) DeliveryGuard() DeliveryGuard {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return DeliveryGuard{input: s.inputSeq, epoch: s.deliveryEpoch}
}

// DeliveryInput validates and queues input against the same session locks as
// manual input, hook reports, activity and program replacement. Notifications
// run only after those locks are released. Empty requires an untouched composer;
// the commit permits the delivery's own pasted draft.
func (m *Manager) DeliveryInput(s *Session, p []byte, guard DeliveryGuard, empty bool) (DeliveryGuard, bool) {
	return m.input(s, p, nil, &guard, empty, nil)
}

// DeliveryPaste also reports successful PTY completion, independently of queue
// acceptance. Failure or close need not signal written; the caller bounds it.
func (m *Manager) DeliveryPaste(s *Session, p []byte, guard DeliveryGuard) (DeliveryGuard, <-chan struct{}, bool) {
	written := make(chan struct{})
	next, ok := m.input(s, p, nil, &guard, true, written)
	return next, written, ok
}

// deliveryReadyLocked checks current readiness, not the snapshot before paste.
// The caller holds the manager, input and session locks through queueing.
func (s *Session) deliveryReadyLocked(guard DeliveryGuard) bool {
	if s.inputSeq != guard.input || s.deliveryEpoch != guard.epoch || s.exited || s.meta.Queued {
		return false
	}
	state := s.stateLocked(time.Now())
	if state == proto.StateWaiting || s.workingLocked(time.Now()) {
		return false
	}
	return true
}

// Input types p into the session and records it (see NoteInput).
func (m *Manager) Input(s *Session, p []byte) {
	m.input(s, p, nil, nil, false, nil)
}

// InputSequence identifies the most recently queued input, independently of echo.
func (s *Session) InputSequence() uint64 {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	return s.inputSeq
}

// InputIfUnchanged queues p only if no other input followed sequence after.
// It does not infer the contents of the program's composer from its screen.
func (m *Manager) InputIfUnchanged(s *Session, p []byte, after uint64) bool {
	_, ok := m.input(s, p, &after, nil, false, nil)
	return ok
}

func (m *Manager) input(s *Session, p []byte, after *uint64, guard *DeliveryGuard, empty bool, written chan struct{}) (DeliveryGuard, bool) {
	// Lock order: manager (guarded delivery only), input, session. Send only
	// enqueues bytes; prompt callbacks and notifications run after unlocking.
	if guard != nil {
		m.mu.Lock()
		if m.sessions[s.id] != s {
			m.mu.Unlock()
			return DeliveryGuard{}, false
		}
	}
	s.inputMu.Lock()
	s.mu.Lock()
	if after != nil && s.inputSeq != *after || guard != nil && !s.deliveryReadyLocked(*guard) {
		if guard != nil {
			s.deliveryPending = false
		}
		s.mu.Unlock()
		s.inputMu.Unlock()
		if guard != nil {
			m.mu.Unlock()
		}
		return DeliveryGuard{}, false
	}
	var next DeliveryGuard
	var onWritten func()
	if written != nil {
		onWritten = func() {
			// Delayed PTY writes echo after enqueue-time suppression expires.
			// Anchor only this still-current input's echo window to its write.
			s.inputMu.Lock()
			s.mu.Lock()
			if s.inputSeq == next.input && s.deliveryEpoch == next.epoch {
				s.lastInput = time.Now()
			}
			s.mu.Unlock()
			s.inputMu.Unlock()
			close(written)
		}
	}
	if guard != nil {
		if !s.term.SendCheckedWritten(p, func(screen string) bool {
			return agent.PromptFree(screen) && (!empty || agent.PromptEmpty(screen))
		}, onWritten) {
			s.deliveryPending = false
			s.mu.Unlock()
			s.inputMu.Unlock()
			m.mu.Unlock()
			return DeliveryGuard{}, false
		}
	} else {
		s.term.Send(p)
	}
	if guard != nil {
		s.deliveryPending = empty
	}
	if len(p) > 0 {
		s.inputSeq++
	}
	prompted := s.noteInputLocked(p)
	changed := s.stateChanged(time.Now())
	sent := s.lines.feed(p)
	next = DeliveryGuard{input: s.inputSeq, epoch: s.deliveryEpoch}
	s.mu.Unlock()
	s.inputMu.Unlock()
	if guard != nil {
		m.mu.Unlock()
	}
	if m.OnPrompt != nil {
		for _, line := range sent {
			m.OnPrompt(s.id, line)
		}
	}
	if prompted || changed {
		m.notify(s)
	}
	return next, true
}

// NoteInput records what the user typed: its echo must not count as
// activity, and its first line becomes the session's prompt when none was
// given. It reports whether the prompt was just set.
//
// Typing answers an agent that waits for the user, and Enter sends its
// next prompt after it finished, so the state the hooks reported no longer
// holds; agents with hooks report the next one themselves.
func (s *Session) NoteInput(p []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.noteInputLocked(p)
}

func (s *Session) noteInputLocked(p []byte) bool {
	s.lastInput = time.Now()
	// Newlines inside a bracketed paste edit the draft; only a separate
	// Enter submits it. Delivery always queues its entire paste together.
	paste := bytes.HasPrefix(p, []byte("\x1b[200~")) && bytes.HasSuffix(p, []byte("\x1b[201~"))
	if s.state == proto.StateWaiting || s.state == proto.StateDone && !paste && bytes.ContainsAny(p, "\r\n") {
		s.state = ""
		s.deliveryEpoch++
	}
	if s.prompt != "" {
		return false
	}
	for _, r := range string(p) {
		switch {
		case s.inEscape:
			// CSI and SS3 sequences end with a letter or ~; others after one rune.
			if r >= '@' && r <= '~' && r != '[' && r != 'O' {
				s.inEscape = false
			}
		case r == 0x1b:
			s.inEscape = true
		case r == '\r' || r == '\n':
			if line := strings.TrimSpace(string(s.typed)); line != "" {
				s.prompt, s.typed = truncate(line, 200), nil
				return true
			}
			s.typed = nil
		case r == 0x7f || r == 0x08:
			if len(s.typed) > 0 {
				s.typed = s.typed[:len(s.typed)-1]
			}
		case r == 0x15: // Ctrl-U
			s.typed = nil
		case r >= 0x20:
			if len(s.typed) < 400 {
				s.typed = append(s.typed, r)
			}
		}
	}
	return false
}
