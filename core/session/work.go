package session

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// maxTick bounds one look's share of the work counted, so a pause of the
// watch (a sleeping laptop) doesn't count as work.
const maxTick = 2 * time.Second

// working reports whether the agent is on a task: as its hooks say, else
// as its screen shows. The caller holds s.mu.
func (s *Session) workingLocked(now time.Time) bool {
	if s.exited {
		return false
	}
	switch s.stateLocked(now) {
	case proto.StateWorking:
		return true
	case proto.StateWaiting, proto.StateDone:
		return false
	}
	return s.active
}

// countWork adds the time since the last look to the work while the agent
// works, and reports whether that reached its budget just now. The caller
// holds s.mu.
func (s *Session) countWork(now time.Time) bool {
	if !s.tick.IsZero() && s.workingLocked(now) {
		s.worked += min(now.Sub(s.tick), maxTick)
	}
	s.tick = now
	if s.meta.Budget > 0 && s.worked >= s.meta.Budget && !s.overBudget {
		s.overBudget = true
		return true
	}
	return false
}

// SetBudget sets how long session s's agent may work, 0 for no limit. A
// budget it is already over interrupts it at the watch's next look.
func (m *Manager) SetBudget(s *Session, budget time.Duration) {
	s.mu.Lock()
	s.meta.Budget = max(budget, 0)
	s.overBudget = false
	s.mu.Unlock()
	m.notify(s)
}

// noteMax bounds a session's note: a line or two of status.
const noteMax = 300

// SetCreds records the vault's credentials lent to session s.
func (m *Manager) SetCreds(s *Session, names []string) {
	s.mu.Lock()
	s.meta.Creds = slices.Clone(names)
	s.mu.Unlock()
	m.notify(s)
}

// SetFeatures sets session s's own feature switches.
func (m *Manager) SetFeatures(s *Session, on map[string]bool) {
	s.mu.Lock()
	s.meta.Features = maps.Clone(on)
	s.mu.Unlock()
	m.notify(s)
}

// UpdateRules changes what session s's agent has of its rules by f, which
// runs locked, so what it reads and changes goes together: it only computes.
func (m *Manager) UpdateRules(s *Session, f func(*proto.SessionRules)) {
	s.mu.Lock()
	var r proto.SessionRules
	if s.meta.Rules != nil {
		r = *s.meta.Rules
	}
	before := r
	f(&r)
	changed := r != before
	if changed {
		s.meta.Rules = &r
	}
	s.mu.Unlock()
	if changed {
		m.notify(s)
	}
}

// SetNoAutoHibernate keeps session s from hibernating on its own, or
// lets it again.
func (m *Manager) SetNoAutoHibernate(s *Session, never bool) {
	s.mu.Lock()
	s.meta.NoAutoHibernate = never
	s.mu.Unlock()
	m.notify(s)
}

// SetConversation records the conversation session s's agent is on.
func (m *Manager) SetConversation(s *Session, id string) {
	s.mu.Lock()
	changed := s.meta.Conversation != id
	s.meta.Conversation = id
	s.mu.Unlock()
	if changed {
		m.notify(s)
	}
}

// NewConversation says session s's agent left its conversation for a new one, not known yet:
// what its folder held before now isn't its own.
func (m *Manager) NewConversation(s *Session) {
	s.mu.Lock()
	s.meta.Conversation, s.meta.ConvoSince = "", time.Now()
	s.mu.Unlock()
	m.notify(s)
}

// SetNote sets what session s is at, "" to clear it.
func (m *Manager) SetNote(s *Session, note string) {
	note = strings.TrimSpace(note)
	if r := []rune(note); len(r) > noteMax {
		note = string(r[:noteMax]) + "…"
	}
	s.mu.Lock()
	s.meta.Note = note
	s.mu.Unlock()
	m.notify(s)
}

// turnTextMost bounds what a session keeps of its turn's last message: what GOA's line
// holds, as it goes with every update of the session.
const turnTextMost = 2000

// NoteTurnText keeps the message s's turn ended on, or "" as a turn starts. It says
// nothing by itself: a change of state follows.
func (m *Manager) NoteTurnText(s *Session, text string) {
	if r := []rune(text); len(r) > turnTextMost {
		text = string(r[:turnTextMost]) + "…"
	}
	s.mu.Lock()
	s.meta.TurnText = text
	s.mu.Unlock()
}

// SetReport keeps its agent's report of a turn.
func (m *Manager) SetReport(s *Session, outcome, text string) {
	s.mu.Lock()
	s.meta.Report, s.meta.ReportOutcome, s.meta.ReportAt = text, outcome, time.Now().UnixMilli()
	s.mu.Unlock()
	m.notify(s)
}

// quickExitsFor is how long quick exits count against starting again.
const quickExitsFor = 30 * time.Second

// QuickExits is how many times in a row s ended right after starting.
func (s *Session) QuickExits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.lastExit) > quickExitsFor {
		return 0 // later on, the user may well try again
	}
	return s.quickExits
}

// ForgetQuickExits lets session s start again whatever ended before: the user asked for it.
func (m *Manager) ForgetQuickExits(s *Session) {
	s.mu.Lock()
	s.quickExits = 0
	s.mu.Unlock()
}

// SetModel records what session s's agent last said it runs on.
func (m *Manager) SetModel(s *Session, model, effort, started string, moved bool) {
	s.mu.Lock()
	s.meta.Model, s.meta.Effort, s.meta.ModelStarted, s.meta.ModelMoved = model, effort, started, moved
	s.mu.Unlock()
	m.notify(s)
}

// SetBackground records the agents session s's agent has in the background; changed: which they are.
func (m *Manager) SetBackground(s *Session, agents []proto.BackgroundAgent) (changed bool) {
	s.mu.Lock()
	changed = !slices.EqualFunc(s.background, agents, func(a, b proto.BackgroundAgent) bool { return a.PID == b.PID && a.Start == b.Start && a.Left == b.Left })
	any := len(agents) > 0 || changed
	s.background = agents
	s.mu.Unlock()
	if any {
		m.notify(s) // their CPU and memory are live, each look
	}
	return changed
}

// movedFromLocked is SessionInfo.ModelStarted. The caller holds s.mu.
func (s *Session) movedFromLocked() string {
	if s.meta.ModelMoved {
		return s.meta.ModelStarted
	}
	return ""
}

// SetMode records the permission mode session s's agent is in.
func (m *Manager) SetMode(s *Session, mode string) {
	s.mu.Lock()
	s.meta.Mode = mode
	s.mu.Unlock()
	m.notify(s)
}

// nameMax bounds a session's name: a line in the session list.
const nameMax = 80

// SetName names session s, "" for none.
func (m *Manager) SetName(s *Session, name string) {
	name = strings.Join(strings.Fields(name), " ")
	if r := []rune(name); len(r) > nameMax {
		name = string(r[:nameMax])
	}
	s.mu.Lock()
	changed := s.meta.Name != name
	s.meta.Name = name
	s.mu.Unlock()
	if changed {
		m.notify(s)
	}
}

// SetPrivate marks s private, kept from its host's teams, or not.
func (m *Manager) SetPrivate(s *Session, private bool) {
	s.mu.Lock()
	s.meta.Private = private
	s.mu.Unlock()
	m.notify(s)
}

// Replace starts spec in place of the session spec.ID names, keeping its
// place among the sessions: a queued session's agent starting.
func (m *Manager) Replace(spec Spec) (*Session, error) {
	old, err := m.Get(spec.ID)
	if err != nil {
		return nil, err
	}
	if spec.Created.IsZero() {
		spec.Created = old.created
	}
	old.mu.Lock()
	quick, last := old.quickExits, old.lastExit
	old.mu.Unlock()
	next, err := m.create(spec, old)
	if err == nil {
		next.mu.Lock()
		if next.quickExits == 0 {
			next.quickExits = quick
		}
		if next.lastExit.IsZero() {
			next.lastExit = last
		}
		next.mu.Unlock()
	}
	return next, err
}
