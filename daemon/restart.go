package daemon

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// What a daemon's restart stops, said before it, and brought back after. It ends what
// sessions run, as their terminals are its own: an update waits while that would cost
// anything (hostconn), and when it can't wait, what can come back does, exactly.

// carryOnPrompt tells an agent whose turn a restart cut short to go on.
const carryOnPrompt = "[enowX] The daemon restarted while you were working, which cut your turn short. Carry on from where you stopped. Check first what already ran before repeating anything."

// restartWhatMost bounds a command as listed.
const restartWhatMost = 200

// restartPlan says what a restart would stop now, and what the last one ended.
func (s *Server) restartPlan() proto.RestartPlan {
	plan := proto.RestartPlan{Lost: s.restartLostNow()}
	s.sampleNow()
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		if info.Exited || info.Queued {
			continue
		}
		if _, isAgent := agent.Find(info.Agent); isAgent {
			if atWork(info) {
				plan.Agents = append(plan.Agents, proto.RestartItem{Session: info.ID, Name: info.Name, What: info.Agent})
			}
			continue
		}
		for _, args := range s.commandsUnder(s.sampledProgram(info.ID)) {
			plan.Commands = append(plan.Commands, proto.RestartItem{Session: info.ID, Name: info.Name, What: commandWords(args)})
		}
	}
	s.Diag.Event("restart.plan", "agents", len(plan.Agents), "commands", len(plan.Commands), "lost", len(plan.Lost))
	return plan
}

// atWork says an agent is mid-turn, or asking: not one whose state went stale, as an
// interrupted one's does.
func atWork(info proto.SessionInfo) bool {
	return info.State == proto.StateWorking && !info.StateStale || info.State == proto.StateWaiting
}

// sessionCommands are the programs a terminal's shell is running now.
func (s *Server) sessionCommands(id string) [][]string { return s.commandsUnder(s.sessionProgram(id)) }

// commandsUnder are the programs own, a session's own processes, started.
func (s *Server) commandsUnder(own map[int]bool) [][]string {
	st := &s.statsState
	var out [][]string
	seen := map[int]bool{}
	for pid := range own {
		for _, child := range st.sampler.Children(pid) {
			if own[child] || seen[child] {
				continue
			}
			seen[child] = true
			if args := procstat.Args(child); len(args) > 0 {
				out = append(out, args)
			}
		}
	}
	slices.SortFunc(out, func(a, b []string) int { return strings.Compare(strings.Join(a, " "), strings.Join(b, " ")) })
	return out
}

// commandWords is a command to list: its secrets masked, its program by name.
func commandWords(args []string) string {
	words := append([]string{filepath.Base(args[0])}, args[1:]...)
	return clip(diag.Scrub(strings.Join(words, " ")), restartWhatMost)
}

// noteForRestart writes down, as the daemon stops, what can't be read after: the agents
// mid-turn, and what runs in terminals, which either comes back or is said to be lost.
func (s *Server) noteForRestart() {
	for _, sess := range s.sessions.List() {
		info, meta := sess.Info(), sess.Meta()
		if info.Exited || info.Queued || !meta.Relaunch {
			continue
		}
		note := restartNote{}
		if _, isAgent := agent.Find(info.Agent); isAgent {
			note.midTurn = atWork(info)
		} else {
			for _, args := range s.sessionCommands(info.ID) {
				if back, ok := agent.TabAgentCommand(args); ok && note.tabAgent == nil {
					note.tabAgent = back
				} else {
					note.lost = append(note.lost, commandWords(args))
				}
			}
		}
		if note.midTurn || note.tabAgent != nil || len(note.lost) > 0 {
			s.persist.noteRestart(info.ID, note)
			s.Diag.Event("restart.noted", "session", info.ID, "midTurn", note.midTurn, "tabAgent", note.tabAgent != nil, "lost", len(note.lost))
		}
	}
}

// restored finishes what a restart noted of session sess: an agent mid-turn is told to
// carry on, once, as it is told only here and its record has forgotten by then.
func (s *Server) restored(sess *session.Session, r record) {
	for _, what := range r.Lost {
		s.restartLostAdd(proto.RestartItem{Session: r.ID, Name: r.Name, What: what})
	}
	if !r.MidTurn {
		return
	}
	s.background.start(func() {
		if err := s.waitSessionReady(sess.ID()); err != nil {
			s.Diag.Event("restart.carryOn", "session", sess.ID(), "err", err)
			return
		}
		err := s.input(proto.SessionInputParams{ID: sess.ID(), Text: carryOnPrompt, Paste: true, Enter: true})
		s.Diag.Event("restart.carryOn", "session", sess.ID(), "err", err)
	})
}

func (s *Server) restartLostAdd(item proto.RestartItem) {
	s.restartLostMu.Lock()
	defer s.restartLostMu.Unlock()
	s.restartLost = append(s.restartLost, item)
}

func (s *Server) restartLostNow() []proto.RestartItem {
	s.restartLostMu.Lock()
	defer s.restartLostMu.Unlock()
	return slices.Clone(s.restartLost)
}
