package daemon

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// goaWakeTime is the longest a session gets to come back before GOA says it
// didn't; goaWakePoll how often it looks, through host.call on any host.
const (
	goaWakeTime = 60 * time.Second
	goaWakePoll = 400 * time.Millisecond
)

// goaPick is the session already in p's project a task with no session named
// goes to, by goa.PickSession; ok false calls for a new one.
func (s *Server) goaPick(p proto.GOADelegateParams) (proto.SessionInfo, bool, error) {
	var list []proto.SessionInfo
	if err := s.goaCall(p.From, p.Host, proto.MethodSessionList, nil, &list); err != nil {
		return proto.SessionInfo{}, false, err
	}
	tasks := s.goa.board.List()
	busy := func(id string) bool {
		return slices.ContainsFunc(tasks, func(t proto.GOATask) bool {
			return t.Session == id && (t.Status == proto.GOATaskWorking || t.Status == proto.GOATaskWaiting || t.Status == proto.GOATaskStalled)
		})
	}
	info, ok := goa.PickSession(list, goa.Want{Project: p.Project, Agent: p.Agent, Config: p.Config}, busy)
	return info, ok, nil
}

// goaSkipsUnheard says whether a worker of agent id starts skipping prompts,
// though not told to: the user keeps them skipped, and its prompts would wait
// unseen, as it has no hooks to tell GOA of them.
func (s *Server) goaSkipsUnheard(id string) bool {
	g, ok := s.goaSession()
	return ok && g.SkipUnheard && id != "" && !agent.ReportsItself(id)
}

// goaReady has session info ready for a prompt, continuing it when its agent
// isn't running; it says whether it had to. An error means no prompt would
// reach it.
func (s *Server) goaReady(p proto.GOADelegateParams, info proto.SessionInfo) (bool, error) {
	if !info.Exited && !info.Queued {
		return false, nil
	}
	if info.Exited {
		if !goa.ComesBack(info) {
			return false, fmt.Errorf("goa: session %s ended and can't come back on a conversation (a shell, or a program enowX can't continue), so nothing was given to it: start a new one", info.ID)
		}
		// Running by now: what someone sent it brought it back since info was read.
		if err := s.goaCall(p.From, p.Host, proto.MethodSessionContinue, proto.SessionRef{ID: info.ID}, nil); err != nil && !strings.Contains(err.Error(), errStillRunning) {
			return false, fmt.Errorf("goa: session %s can't be brought back: %w", info.ID, err)
		}
	}
	return true, s.goaWaitReady(p, info.ID)
}

// goaWaitReady waits for session id on p's host to be ready: through control
// on another host, at once here, where it keeps polls out of the control log.
func (s *Server) goaWaitReady(p proto.GOADelegateParams, id string) error {
	if s.thisHost(p.Host) {
		return s.waitSessionReady(id)
	}
	return waitReady(id, func() (proto.SessionInfo, string, error) {
		var list []proto.SessionInfo
		if err := s.goaCall(p.From, p.Host, proto.MethodSessionList, nil, &list); err != nil {
			return proto.SessionInfo{}, "", err
		}
		i := slices.IndexFunc(list, func(x proto.SessionInfo) bool { return x.ID == id })
		if i < 0 {
			return proto.SessionInfo{}, "", fmt.Errorf("goa: session %s is gone", id)
		}
		var screen proto.SessionScreen
		if !list[i].Exited && !list[i].Queued {
			if err := s.goaCall(p.From, p.Host, proto.MethodSessionScreen, proto.SessionRef{ID: id}, &screen); err != nil {
				return proto.SessionInfo{}, "", err
			}
		}
		return list[i], screen.Text, nil
	})
}

// waitSessionReady is waitReady for a session of this host.
func (s *Server) waitSessionReady(id string) error {
	return waitReady(id, func() (proto.SessionInfo, string, error) {
		sess, err := s.sessions.Get(id)
		if err != nil {
			return proto.SessionInfo{}, "", err
		}
		return sess.Info(), s.maskLent(sess.Terminal().Text()), nil
	})
}

// waitReady waits for session id, as peek reads it, to show its agent back and
// quiet, as a first prompt waits (promptQuiet, promptTimeout). A resumed
// session shows its old screen at once, so it counts from the screen changing;
// a screen that never changes or settles still gets the prompt when that wait
// ends, rather than losing it. It fails when the session ends or stays queued.
func waitReady(id string, peek func() (proto.SessionInfo, string, error)) error {
	start := time.Now()
	first, last, quietSince, changed := "", "", start, false
	for n := 0; ; n++ {
		info, text, err := peek()
		if err != nil {
			return err
		}
		if info.Exited {
			return fmt.Errorf("goa: session %s ended as it came back (exit %d): read it with: read %s", id, info.ExitCode, id)
		}
		if !info.Queued {
			if n == 0 {
				first = text
			}
			if text != last {
				last, quietSince = text, time.Now()
			}
			changed = changed || text != first
			if !agent.TrustAsked(text) && strings.TrimSpace(last) != "" {
				if changed && time.Since(quietSince) >= promptQuiet || time.Since(start) >= promptTimeout {
					return nil
				}
			}
		}
		if time.Since(start) >= goaWakeTime {
			what := "didn't come back"
			if info.Queued {
				what = "still waits for fewer agents to work"
			}
			return fmt.Errorf("goa: session %s %s after %s", id, what, goaWakeTime)
		}
		time.Sleep(goaWakePoll)
	}
}

// goaNoteAsleep keeps whether session info is hibernated, and says if that changed.
func (s *Server) goaNoteAsleep(info proto.SessionInfo) bool {
	asleep := info.Exited && info.Hibernated
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	if s.goa.asleep[info.ID] == asleep {
		return false
	}
	if s.goa.asleep == nil {
		s.goa.asleep = map[string]bool{}
	}
	if asleep {
		s.goa.asleep[info.ID] = true
	} else {
		delete(s.goa.asleep, info.ID)
	}
	return true
}

// goaAsleep says task t's session is hibernated: as it is here, as last heard on
// another host, which a board's read can't go and ask.
func (s *Server) goaAsleep(t proto.GOATask) bool {
	if t.Session == "" {
		return false
	}
	if s.thisHost(t.Host) {
		sess, err := s.sessions.Get(t.Session)
		return err == nil && sess.Info().Hibernated
	}
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	return s.goa.asleep[t.Session]
}

// sessionWord is what reads as a session's ID in a report: IDs are hex, and a commit's
// short hash, at 7, doesn't reach it.
var sessionWord = regexp.MustCompile(`\b[0-9a-f]{8,}\b`)

// goaAsleepNamed is what a report of being blocked leaves out: the sessions it names, on
// host, that are asleep, and whether they would come back. self is the one reporting.
func (s *Server) goaAsleepNamed(host, outcome, self, said string) string {
	if outcome != "blocked" {
		return ""
	}
	words := sessionWord.FindAllString(said, -1)
	if len(words) == 0 {
		return ""
	}
	var list []proto.SessionInfo
	if err := s.goaCall("", host, proto.MethodSessionList, nil, &list); err != nil {
		return ""
	}
	var out strings.Builder
	for _, info := range list {
		named := slices.ContainsFunc(words, func(w string) bool { return strings.HasPrefix(info.ID, w) })
		if !named || info.ID == self || !info.Exited {
			continue
		}
		switch {
		case !goa.ComesBack(info):
			fmt.Fprintf(&out, " (Session %s, which it names, has ended and can't come back: start a new one.)", info.ID)
		case info.Hibernated:
			fmt.Fprintf(&out, " (Session %s, which it names, is asleep, hibernated: what is sent to it brings it back.)", info.ID)
		default:
			fmt.Fprintf(&out, " (Session %s, which it names, has ended: what is sent to it brings it back.)", info.ID)
		}
	}
	return out.String()
}
