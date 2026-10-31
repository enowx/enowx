package daemon

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// GOA's workers' permission prompts, on the worker's host: before a tool
// runs, what is urgent (it can't be undone, money, publishing, secrets) is
// left for the user to answer, with a sound and a banner; the rest the
// judge allows or refuses when it is sure (unsure is urgent too); what is
// left asks, and GOA, told, answers it. No limit on workers: accounts'
// fallback and budgets handle the rate limits.

// goaWorkerTool is the decision on worker info's tool call ev.
func (s *Server) goaWorkerTool(info proto.SessionInfo, ev proto.HookEvent) proto.HookDecision {
	s.goaClearUrgent(info.ID)
	urgent := goa.Urgent(ev.Tool, ev.Command, ev.Paths)
	if ev.Command == "" || !s.useOn(proto.DecideApprove) {
		switch {
		case urgent != "":
			s.goaUrgent(info, ev, urgent)
		case ev.Command == "" && inside(info.Dir, ev.Paths):
			return proto.HookDecision{Approve: true} // its own work, in its folder
		}
		return proto.HookDecision{} // GOA answers what is left
	}
	d, err := s.decideFor(info, proto.DecideAsk{Use: proto.DecideApprove, Command: ev.Command})
	sure := err == nil && d.Judge != "" && s.sure(d)
	switch {
	case sure && d.Verdict == proto.VerdictRefuse:
		// A refusal is safe to act on, urgent or not.
		s.acted(d, proto.ActedRefused, 0)
		return proto.HookDecision{Block: true, Reason: "enowX's judge refused this command: " + d.Why + " Tell GOD of Agents what you need instead."}
	case urgent != "":
		if err == nil && d.Judge != "" {
			s.acted(d, proto.ActedNothing, 0)
		}
		s.goaUrgent(info, ev, urgent)
		return proto.HookDecision{}
	case err != nil, d.Judge == "":
		return proto.HookDecision{} // no judge: GOA answers
	case !sure:
		s.acted(d, proto.ActedNothing, 0)
		s.goaUrgent(info, ev, fmt.Sprintf("%s (%.2f): %s", goa.UrgentUnsure, d.Confidence, d.Why))
		return proto.HookDecision{}
	case d.Verdict == proto.VerdictAllow:
		s.acted(d, proto.ActedAllowed, 0)
		return proto.HookDecision{Approve: true}
	}
	s.acted(d, proto.ActedNothing, 0)
	return proto.HookDecision{}
}

// inside says whether every path is in dir (relative ones are).
func inside(dir string, paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			if strings.HasPrefix(filepath.Clean(p), "..") {
				return false
			}
			continue
		}
		if rel, err := filepath.Rel(dir, p); err != nil || strings.HasPrefix(rel, "..") {
			return false
		}
	}
	return true
}

// goaUrgent marks what worker info asks next as the user's, and tells
// them with a sound and a banner.
func (s *Server) goaUrgent(info proto.SessionInfo, ev proto.HookEvent, why string) {
	what := strings.TrimSpace(ev.Tool + " " + clip(oneLine(s.maskLent(ev.Command)), 160))
	if what == "" && len(ev.Paths) > 0 {
		what = strings.Join(ev.Paths, ", ")
	}
	s.goa.mu.Lock()
	s.goa.urgent[info.ID] = why
	s.goa.mu.Unlock()
	name := nonEmpty(info.Name, s.agentLabel(info.Agent))
	s.broadcast(proto.EventNotifySound, proto.NotifySoundParams{Kind: proto.NotifyInput, Session: info.ID,
		Text: "Urgent, for you: " + name + " (GOD of Agents' worker) asks to run " + what + ": " + why + "."})
	s.logger().Info("goa: a worker's call left to the user", "session", info.ID, "why", why)
}

// goaUrgentOf is why session id's ask is the user's, "" when it isn't.
func (s *Server) goaUrgentOf(id string) string {
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	return s.goa.urgent[id]
}

func (s *Server) goaClearUrgent(id string) {
	s.goa.mu.Lock()
	delete(s.goa.urgent, id)
	s.goa.mu.Unlock()
}

// goaMayAnswer refuses a session's answer (through host.call) to an ask
// that is the user's.
func (s *Server) goaMayAnswer(p proto.HostCallParams) error {
	if p.Method != proto.MethodSessionAnswer || p.From == "" {
		return nil
	}
	var ref proto.SessionAnswerParams
	if json.Unmarshal(p.Params, &ref) != nil {
		return nil
	}
	if why := s.goaUrgentOf(ref.ID); why != "" {
		return fmt.Errorf("goa: this one is the user's to answer (%s): tell them, don't answer it", why)
	}
	return nil
}
