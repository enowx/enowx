package daemon

import (
	"fmt"
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
			return t.Session == id && (t.Status == proto.GOATaskWorking || t.Status == proto.GOATaskWaiting)
		})
	}
	info, ok := goa.PickSession(list, goa.Want{Project: p.Project, Agent: p.Agent, Skip: p.SkipPermissions, Config: p.Config}, busy)
	return info, ok, nil
}

// goaReady has session info ready for a prompt, continuing it when its agent
// isn't running; it says whether it had to. An error means no prompt would
// reach it.
func (s *Server) goaReady(p proto.GOADelegateParams, info proto.SessionInfo) (bool, error) {
	if !info.Exited && !info.Queued {
		return false, nil
	}
	if info.Exited {
		if err := s.goaCall(p.From, p.Host, proto.MethodSessionContinue, proto.SessionRef{ID: info.ID}, nil); err != nil {
			return false, fmt.Errorf("goa: session %s can't be brought back: %w", info.ID, err)
		}
	}
	return true, s.goaWaitReady(p, info.ID)
}

// goaWaitReady waits for session id to show its agent back and quiet, as a
// first prompt waits (promptQuiet, promptTimeout). A resumed session shows
// its old screen at once, so it counts from the screen changing; a screen
// that never changes or settles still gets the prompt when that wait ends,
// rather than losing it. It fails when the session ends or stays queued.
func (s *Server) goaWaitReady(p proto.GOADelegateParams, id string) error {
	start := time.Now()
	first, last, quietSince, changed := "", "", start, false
	for n := 0; ; n++ {
		var list []proto.SessionInfo
		if err := s.goaCall(p.From, p.Host, proto.MethodSessionList, nil, &list); err != nil {
			return err
		}
		i := slices.IndexFunc(list, func(x proto.SessionInfo) bool { return x.ID == id })
		switch {
		case i < 0:
			return fmt.Errorf("goa: session %s is gone", id)
		case list[i].Exited:
			return fmt.Errorf("goa: session %s ended as it came back (exit %d): read it with: read %s", id, list[i].ExitCode, id)
		}
		queued := list[i].Queued
		if !queued {
			var screen proto.SessionScreen
			if err := s.goaCall(p.From, p.Host, proto.MethodSessionScreen, proto.SessionRef{ID: id}, &screen); err != nil {
				return err
			}
			if n == 0 {
				first = screen.Text
			}
			if screen.Text != last {
				last, quietSince = screen.Text, time.Now()
			}
			changed = changed || screen.Text != first
			asked := agent.TrustAsked(screen.Text)
			if !asked && strings.TrimSpace(last) != "" {
				if changed && time.Since(quietSince) >= promptQuiet || time.Since(start) >= promptTimeout {
					return nil
				}
			}
		}
		if time.Since(start) >= goaWakeTime {
			what := "didn't come back"
			if queued {
				what = "still waits for fewer agents to work"
			}
			return fmt.Errorf("goa: session %s %s after %s", id, what, goaWakeTime)
		}
		time.Sleep(goaWakePoll)
	}
}
