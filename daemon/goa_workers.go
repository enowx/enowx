package daemon

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// GOA's workers' permission prompts, on the worker's host: before a tool
// runs, what is urgent (it can't be undone, money, publishing, secrets) is
// left for the user to answer, with a sound and a banner; the rest the
// judge allows or refuses when it is sure (unsure is urgent too); what is
// left asks, and GOA, told, answers it. No limit on workers: accounts'
// fallback and budgets handle the rate limits.
//
// The user hears of it once the agent really asks: a tool call isn't a prompt,
// as its permission mode or its settings may let it run. A worker in bypass
// never asks, so nothing of it waits on the user; what is urgent there is
// logged and runs, as bypass promises (docs/goa.md).

// goaWorkerTool is the decision on worker info's tool call ev.
func (s *Server) goaWorkerTool(info proto.SessionInfo, ev proto.HookEvent) proto.HookDecision {
	s.goaClearUrgent(info.ID)
	urgent := goa.Urgent(ev.Tool, ev.Command, ev.Paths)
	if ev.Command == "" || !s.useOn(proto.DecideApprove) {
		switch {
		case urgent != "":
			s.goaLeaveToUser(info, ev, urgent)
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
		s.goaLeaveToUser(info, ev, urgent)
		return proto.HookDecision{}
	case err != nil, d.Judge == "":
		return proto.HookDecision{} // no judge: GOA answers
	case !sure:
		s.acted(d, proto.ActedNothing, 0)
		s.goaLeaveToUser(info, ev, fmt.Sprintf("%s (%.2f): %s", goa.UrgentUnsure, d.Confidence, d.Why))
		return proto.HookDecision{}
	case d.Verdict == proto.VerdictAllow:
		s.acted(d, actedOnAllow(info), 0)
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

// workerRunsUnasked says worker info's agent won't prompt for tool call ev:
// by the mode it reports with the call, or, reporting none, as it was started.
func workerRunsUnasked(info proto.SessionInfo, ev proto.HookEvent) bool {
	if ev.PermissionMode != "" {
		return ev.PermissionMode == agentModeBypass
	}
	return info.SkipPermissions
}

// goaLeaveToUser marks what worker info asks next as the user's, so GOA can't
// answer it. The user is told when it does ask (goaWorkerAsks); an agent with
// no hook for its prompts is taken to ask now, as nothing would say so later.
func (s *Server) goaLeaveToUser(info proto.SessionInfo, ev proto.HookEvent, why string) {
	if workerRunsUnasked(info, ev) {
		s.logger().Info("goa: a worker in bypass runs what would be the user's", "session", info.ID, "tool", ev.Tool, "why", why)
		return
	}
	s.goa.mu.Lock()
	s.goa.urgent[info.ID] = why
	s.goa.pending[info.ID] = ev
	s.goa.mu.Unlock()
	if agent.HookSupport(info.Agent)[proto.HookPermission] != proto.HookInAgent {
		s.goaWorkerAsks(info)
	}
}

// goaWorkerAsks tells the user the prompt worker info just showed is theirs,
// with a sound and a banner, if the tool call it is for was left to them.
func (s *Server) goaWorkerAsks(info proto.SessionInfo) {
	s.goa.mu.Lock()
	ev, pending := s.goa.pending[info.ID]
	why := s.goa.urgent[info.ID]
	delete(s.goa.pending, info.ID)
	s.goa.mu.Unlock()
	if pending {
		s.goaUrgent(info, ev, why)
	}
}

// goaUrgent tells the user with a sound and a banner that worker info asks
// them about tool call ev.
func (s *Server) goaUrgent(info proto.SessionInfo, ev proto.HookEvent, why string) {
	what := strings.TrimSpace(ev.Tool + " " + clip(oneLine(s.maskLent(ev.Command)), 160))
	if what == "" && len(ev.Paths) > 0 {
		what = strings.Join(ev.Paths, ", ")
	}
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
	delete(s.goa.pending, id)
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
