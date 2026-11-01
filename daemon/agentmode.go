package daemon

import (
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// agentModeBypass is the permission mode, as agents' hooks report it, in
// which an agent runs every tool without asking.
const agentModeBypass = "bypassPermissions"

// agentModeFresh is how long a reported mode counts as the agent's current
// one. A hook reports it just before each tool call, so an agent asking
// from a tool has reported within moments; older than this, it may have
// changed since (shift+tab) with nothing to say so.
const agentModeFresh = 2 * time.Minute

// agentModes keeps the permission mode each session's agent last reported
// through its hooks. It is memory only: after a restart a mode is unknown
// until the next report.
type agentModes struct {
	mu sync.Mutex
	by map[string]agentMode
}

type agentMode struct {
	mode string
	at   time.Time
}

func (m *agentModes) note(id, mode string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.by == nil {
		m.by = map[string]agentMode{}
	}
	for other, e := range m.by {
		if at.Sub(e.at) > time.Hour {
			delete(m.by, other)
		}
	}
	m.by[id] = agentMode{mode: mode, at: at}
}

func (m *agentModes) forget(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.by, id)
}

func (m *agentModes) get(id string) (agentMode, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.by[id]
	return e, ok
}

// sessionBypasses is the mode of session from when its agent runs without
// asking now, by what its hooks last reported: "" when it doesn't or that
// isn't known, so the user is asked. How it started counts for nothing, as
// it can change the mode since; a teammate's session on this host never
// speaks for the owner.
func (s *Server) sessionBypasses(from string) string {
	if from == "" {
		return ""
	}
	sess, err := s.sessions.Get(from)
	if err != nil {
		return ""
	}
	info := sess.Info()
	if info.Exited || info.Member != nil || agent.HookSupport(info.Agent)[proto.HookToolBefore] != proto.HookInAgent {
		return ""
	}
	e, ok := s.agentModes.get(from)
	if !ok || e.mode != agentModeBypass || time.Since(e.at) > agentModeFresh {
		return ""
	}
	return e.mode
}
