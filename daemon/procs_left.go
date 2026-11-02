package daemon

import (
	"fmt"
	"slices"

	"github.com/enowx/enowx/proto"
)

// procsLeft are what still runs with no session looking after it: its session gone or
// ended, or left from under it, as use says.
func (s *Server) procsLeft(use func(pid int) proto.Usage) []proto.LeftProcess {
	s.procs.mu.Lock()
	defer s.procs.mu.Unlock()
	var out []proto.LeftProcess
	for _, p := range s.procs.known {
		ended := true
		if sess, err := s.sessions.Get(p.Session); err == nil {
			ended = sess.Info().Exited
		}
		if ended || p.Left {
			out = append(out, proto.LeftProcess{PID: p.PID, Start: p.Start, Name: p.Name, Session: p.Session, Usage: use(p.PID)})
		}
	}
	slices.SortFunc(out, func(x, y proto.LeftProcess) int { return x.PID - y.PID })
	return out
}

// stopLeftProcess stops a process the Resource usage page shows as left, and no other.
func (s *Server) stopLeftProcess(p proto.StopProcessParams) error {
	procs := s.procs.procsTake(func(x startedProc) bool { return x.PID == p.PID && x.Start == p.Start })
	if len(procs) == 0 {
		return fmt.Errorf("procs: process %d isn't one a session here left running", p.PID)
	}
	go s.endProcs(procs)
	return nil
}
