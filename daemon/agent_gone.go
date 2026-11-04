package daemon

import (
	"os"
	"time"

	"github.com/enowx/enowx/core/procstat"
)

// A session's agent that ends leaves a shell in its terminal, for the user. What enowX types is for
// the agent: in that shell a prompt's words run as commands. So nothing is typed there, and each
// line typed is looked at just before, however long ago the agent was seen. A look and a write are
// not one step: an agent ending in the moment between them is the one case left, and programGone
// keeps it to that moment.

// agentGoneAgain is how soon a look that says gone looks again: a session just started has its
// wrapper before its agent.
const agentGoneAgain = 150 * time.Millisecond

// agentGone says session id's agent has ended and left its fallback shell in the terminal.
func (s *Server) agentGone(id string) bool {
	return s.agentGoneNow(id) && func() bool { time.Sleep(agentGoneAgain); return s.agentGoneNow(id) }()
}

func (s *Server) agentGoneNow(id string) bool {
	list, err := procstat.Snapshot()
	if err != nil {
		return false // can't tell, on a system that can't list processes
	}
	self := os.Getpid()
	kids := make(map[int][]int, len(list))
	for _, p := range list {
		kids[p.PPID] = append(kids[p.PPID], p.PID)
	}
	for _, root := range kids[self] {
		if sessionOf(root) == id {
			return programGone(root, kids, procstat.Args)
		}
	}
	return false
}
