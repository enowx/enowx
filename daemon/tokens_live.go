package daemon

import (
	"time"

	"github.com/enowx/enowx/core/agent"
)

// tokenDBAgents keep their records in a database their running program writes to
// (OpenCode's opencode.db and its kin): it is read while one of their sessions runs and
// once more just after, never between, so its writers aren't kept waiting by a reader
// with nothing new to find (issue #47).
var tokenDBAgents = map[string]bool{"opencode": true, "opencode2": true, "kilo": true, "mimo-code": true}

// tokenAgentsLive says which agents' records a scan at now reads: every agent's but
// those of tokenDBAgents with no session running, or stopped more than a scan ago. It
// runs with scanMu held, which owns dbSeen.
func (s *Server) tokenAgentsLive(now time.Time) func(string) bool {
	t := &s.tok
	if t.dbSeen == nil {
		t.dbSeen = map[string]time.Time{}
	}
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		if id := agent.Base(info.Agent); !info.Exited && tokenDBAgents[id] {
			t.dbSeen[id] = now
		}
	}
	return func(id string) bool {
		if !tokenDBAgents[id] {
			return true
		}
		seen, ok := t.dbSeen[id]
		return ok && now.Sub(seen) <= 2*scanEvery
	}
}
