package daemon

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// Agents a session's agent started: general, as any agent's show in its process tree, whether
// or not it says so. What runs inside its own process, or only at its API, can't be seen this
// way. A process counts from its second look, so that a one-shot call of an agent (a judge, a
// title) isn't one; a server or a command doesn't count, as no preset names it.

const (
	// backgroundStuckAfter is how long an agent uses next to no CPU before it reads as stuck: a
	// model's slow answer takes minutes, not this.
	backgroundStuckAfter = 15 * time.Minute
	// backgroundBusyPercent is what counts as using CPU: a waiting agent's timers stay under it.
	backgroundBusyPercent = 0.5
)

// agentLook is what the last look saw of a background agent.
type agentLook struct {
	since, at, busy time.Time
	cpu             time.Duration
}

// backgroundLocked says, by session, the agents each has in its background. was is the list
// before this look. The caller holds t.mu.
func (t *procTracker) backgroundLocked(list []procstat.Proc, was map[int]startedProc, args func(int) []string, now time.Time) map[string][]proto.BackgroundAgent {
	byPID := make(map[int]procstat.Proc, len(list))
	for _, p := range list {
		byPID[p.PID] = p
	}
	programs := map[int]string{}
	program := func(pid int) string {
		name, ok := programs[pid]
		if !ok {
			name = agent.ProgramOf(args(pid))
			programs[pid] = name
		}
		return name
	}
	seen := map[startedProc]agentLook{}
	out := map[string][]proto.BackgroundAgent{}
	for pid, p := range t.known {
		before, ok := was[pid]
		name := ""
		if ok && before.Start == p.Start {
			name = program(pid)
		}
		if name == "" || !inBackground(pid, p, t.known, byPID, program) {
			continue
		}
		key := startedProc{PID: pid, Start: p.Start}
		look, again := t.agentSeen[key]
		percent := 0.0
		if !again {
			look = agentLook{since: now, busy: now}
		} else if took := now.Sub(look.at); took > 0 {
			percent = 100 * (byPID[pid].CPUTime - look.cpu).Seconds() / took.Seconds()
		}
		if percent >= backgroundBusyPercent {
			look.busy = now
		}
		look.at, look.cpu = now, byPID[pid].CPUTime
		seen[key] = look
		out[p.Session] = append(out[p.Session], proto.BackgroundAgent{PID: pid, Start: p.Start, Name: name,
			Command: commandLine(args(pid)), Since: look.since, Left: p.Left, Memory: byPID[pid].RSS, CPU: byPID[pid].CPUTime.Seconds(),
			CPUPercent: percent, Stuck: now.Sub(look.busy) >= backgroundStuckAfter})
	}
	t.agentSeen = seen
	for _, agents := range out {
		slices.SortFunc(agents, func(x, y proto.BackgroundAgent) int { return x.PID - y.PID })
	}
	return out
}

// inBackground says agent process pid isn't its session's own: it runs under another agent, or
// left its tree. A program its own direct parent also runs is that one, behind a wrapper (an
// npm package's script starting its binary).
func inBackground(pid int, p startedProc, known map[int]startedProc, byPID map[int]procstat.Proc, program func(int) string) bool {
	parent := byPID[pid].PPID
	if program(parent) == program(pid) && known[parent].Session == p.Session {
		return false
	}
	if p.Left {
		return true
	}
	for at := parent; ; at = byPID[at].PPID {
		up, ok := known[at]
		if !ok || up.Session != p.Session {
			return false // reached the session's root: it is the session's agent
		}
		if program(at) != "" {
			return true
		}
	}
}

// adoptDetached puts on the list what left its session's tree before a look could see it there, as
// a command started with nohup whose shell has ended: taken up by init, it still bears its session's
// tag, where the system shows it, and only a session of this daemon's counts. It says if any came.
func adoptDetached(known map[int]startedProc, list []procstat.Proc, env func(int, string) string, isSession func(string) bool) bool {
	inits := map[int]bool{1: true}
	for _, p := range list {
		if p.Name == "systemd" {
			inits[p.PID] = true // a user's own, which takes up orphans too
		}
	}
	added := false
	for _, p := range list {
		if _, ok := known[p.PID]; ok || !inits[p.PPID] {
			continue
		}
		if id := env(p.PID, session.EnvSession); id != "" && isSession(id) {
			known[p.PID] = startedProc{PID: p.PID, Start: p.Start, Name: p.Name, Session: id, Left: true}
			added = true
		}
	}
	return added
}

// isSession says id is a session of this daemon's.
func (s *Server) isSession(id string) bool {
	_, err := s.sessions.Get(id)
	return err == nil
}

// commandLine is a process's arguments, short enough for a tooltip.
func commandLine(args []string) string {
	line := strings.Join(args, " ")
	if len(line) > 200 {
		line = line[:200] + "…"
	}
	return line
}

// noteBackground gives sessions what a look found.
func (s *Server) noteBackground(found map[string][]proto.BackgroundAgent) {
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		agents := found[info.ID]
		if info.Exited {
			agents = nil
		}
		if len(agents) == 0 && len(info.Background) == 0 {
			continue
		}
		if s.sessions.SetBackground(sess, agents) {
			names := make([]string, len(agents))
			for i, a := range agents {
				names[i] = a.Name
			}
			s.logger().Info("procs: a session's background agents", "session", info.ID, "agents", names)
		}
		s.goaBackgroundChanged(info, info.Background, agents)
	}
}

// goaBackgroundChanged tells GOA of a worker's background agent that ended or got stuck, and shows
// the board the change. A worker whose session ended is told of as that.
func (s *Server) goaBackgroundChanged(info proto.SessionInfo, was, now []proto.BackgroundAgent) {
	if !s.goa.board.Has(info.ID) {
		return
	}
	changed := len(was) != len(now)
	for _, b := range was {
		i := slices.IndexFunc(now, func(n proto.BackgroundAgent) bool { return n.PID == b.PID && n.Start == b.Start })
		switch {
		case i < 0:
			changed = true
			if !info.Exited {
				s.goaTellAbout(info.ID, fmt.Sprintf("[enowX GOA] Worker %s: its background agent %s (pid %d) ended, after %s.", info.ID, b.Name, b.PID, time.Since(b.Since).Round(time.Second)))
			}
		case now[i].Stuck && !b.Stuck:
			s.goaTellAbout(info.ID, fmt.Sprintf("[enowX GOA] Worker %s: its background agent %s (pid %d) has used next to no CPU for %s and may be stuck: enowx-app background %s lists it, background stop ends it.", info.ID, b.Name, b.PID, backgroundStuckAfter, info.ID))
		}
	}
	if changed {
		s.goaChanged()
	}
}

// stopBackgroundAgent stops one of p.Session's background agents and all it started, and no other
// process: the rest of the session goes on. Asked first, then forced (endProcs).
func (s *Server) stopBackgroundAgent(p proto.StopBackgroundParams) error {
	sess, err := s.sessions.Get(p.Session)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(sess.Info().Background, func(b proto.BackgroundAgent) bool { return b.PID == p.PID && b.Start == p.Start }) {
		return fmt.Errorf("procs: process %d isn't a background agent of session %s", p.PID, p.Session)
	}
	list, err := procstat.Snapshot()
	if err != nil {
		return fmt.Errorf("procs: list processes: %w", err)
	}
	kids := make(map[int][]int, len(list))
	for _, x := range list {
		kids[x.PPID] = append(kids[x.PPID], x.PID)
	}
	tree := map[int]bool{}
	for queue := []int{p.PID}; len(queue) > 0; queue = queue[1:] {
		tree[queue[0]] = true
		queue = append(queue, kids[queue[0]]...)
	}
	procs := s.procs.procsTake(func(x startedProc) bool { return x.Session == p.Session && tree[x.PID] })
	if !slices.ContainsFunc(procs, func(x startedProc) bool { return x.PID == p.PID && x.Start == p.Start }) {
		return fmt.Errorf("procs: process %d has already ended", p.PID)
	}
	s.logger().Info("procs: stopped a background agent", "session", p.Session, "pid", p.PID, "processes", len(procs))
	info := sess.Info()
	left := slices.DeleteFunc(slices.Clone(info.Background), func(b proto.BackgroundAgent) bool { return b.PID == p.PID })
	s.sessions.SetBackground(sess, left)
	s.goaBackgroundChanged(info, info.Background, left)
	go s.endProcs(procs)
	return nil
}
