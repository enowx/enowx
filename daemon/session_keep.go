package daemon

import (
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A session keeps the model, effort and permission mode it was given. Its agent's global
// settings can't be relied on for that: Claude Code's /model and /effort save what they set
// as the default for new sessions, in a file all its sessions read. So each is passed
// whenever a session starts or resumes, and a new session takes what the user chose for
// that agent in enowX, the agent's own settings only where nothing was chosen.

// keptArgs are the arguments that start a session's agent again as it last ran. A config
// decides its own model and effort.
func keptArgs(m session.Meta) []string {
	if m.Config != nil {
		return nil
	}
	return agent.StartArgs(m.Agent, m.Model, m.Effort, m.Mode, m.SkipPermissions)
}

// withAgentStart gives p what the user chose for new sessions of its agent, where p itself
// doesn't say.
func (s *Server) withAgentStart(p proto.SessionCreateParams) proto.SessionCreateParams {
	start := s.sched.get().AgentStart[agent.Base(p.Agent)]
	if p.Model == "" {
		p.Model = start.Model
	}
	if p.Effort == "" {
		p.Effort = start.Effort
	}
	if p.Mode == "" {
		p.Mode = start.Mode
	}
	return p
}
