package daemon

import (
	"errors"
	"fmt"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/proto"
)

// The judge's later uses on this host (docs/decide.md): a model and effort
// for a prompt (route), the search results that bear on a prompt
// (context), a memory worth offering (memory), how a chat message
// notifies (chat), and whether a failed check is flaky (flaky). Each, off
// or unanswered, leaves things as they were without the judge.

// useOn says whether use is on on this host.
func (s *Server) useOn(use string) bool { return decide.Use(s.judge.Settings().Get(), use).On }

// routeFor is how hard p's prompt is, as a model and effort of its agent.
func (s *Server) routeFor(p proto.DecideRouteParams) (proto.RouteChoice, error) {
	var info proto.SessionInfo
	if p.Session != "" {
		sess, err := s.sessions.Get(p.Session)
		if err != nil {
			return proto.RouteChoice{}, err
		}
		info = sess.Info()
	}
	if info.Agent == "" {
		info.Agent = p.Agent
	}
	id := agent.Base(info.Agent)
	if id == "" {
		return proto.RouteChoice{}, errors.New("decide: route for which agent? (--agent claude, codex, …)")
	}
	out := proto.RouteChoice{Agent: id}
	d, err := s.decideFor(info, proto.DecideAsk{Use: proto.DecideRoute, Session: p.Session, Project: p.Project, Prompt: s.maskLent(p.Prompt)})
	if errors.Is(err, decide.ErrOff) {
		return out, errors.New("decide: the use route (a model per prompt) is off: Settings → Decisions → Uses")
	}
	if err != nil {
		return out, err
	}
	out.Decision, out.Difficulty, out.Why, out.Confidence = d.ID, d.Verdict, d.Why, d.Confidence
	switch {
	case d.Judge == "":
		out.Error = d.Error
		return out, nil
	case !s.sure(d):
		s.acted(d, proto.ActedNothing, 0)
		out.Why = fmt.Sprintf("not sure enough (%.2f), the agent's own model kept: %s", d.Confidence, d.Why)
		return out, nil
	}
	var api *decide.RouteAPI
	if a := agentAPIOf(id); a != nil {
		api = &decide.RouteAPI{Model: a.Model, SmallModel: a.SmallModel}
	}
	r := decide.RouteFor(id, d.Verdict, api)
	out.Model, out.Effort, out.Switch = r.Model, r.Effort, r.Switch(id)
	s.acted(d, proto.ActedRouted, 0)
	return out, nil
}

// pickContext is those of hits (parts, as the prompt would get them) that
// bear on session info's prompt, as the judge says; all of them when the
// use is off or the judge has no sure answer.
func (s *Server) pickContext(info proto.SessionInfo, prompt string, hits []proto.RAGHit, parts []string) []proto.RAGHit {
	if len(hits) == 0 || !s.useOn(proto.DecideContext) {
		return hits
	}
	d, err := s.decideFor(info, proto.DecideAsk{Use: proto.DecideContext, Prompt: s.maskLent(prompt), Parts: parts})
	switch {
	case err != nil:
		return hits
	case !s.sure(d):
		if d.Judge != "" {
			s.acted(d, proto.ActedNothing, 0)
		}
		return hits
	case d.Verdict == proto.VerdictNone:
		s.acted(d, proto.ActedDropped, 0)
		return nil
	}
	kept := decide.Keep(hits, d)
	s.acted(d, proto.ActedChosen, 0)
	return kept
}

// memoryOf is what to offer as a memory of session info's agent's last
// answer: the decision it holds, distilled; ok false when the judge says
// it holds none. Off or unanswered, the answer as it is.
func (s *Server) memoryOf(info proto.SessionInfo, last string) (string, bool) {
	if !s.useOn(proto.DecideMemory) {
		return last, true
	}
	d, err := s.decideFor(info, proto.DecideAsk{Use: proto.DecideMemory, Text: s.maskLent(last)})
	switch {
	case err != nil:
		return last, true
	case !s.sure(d):
		if d.Judge != "" {
			s.acted(d, proto.ActedNothing, 0)
		}
		return last, true
	case d.Verdict == proto.VerdictNothing:
		s.acted(d, proto.ActedDropped, 0)
		return "", false
	}
	s.acted(d, proto.ActedOffered, 0)
	return d.Text, true
}

// chatUrgency is how a team chat message a notifies the user: Acted says
// urgent (a sound and banner whatever the chat's settings) or quiet (only
// counted unread); "" as the settings say. A message naming the user is
// never kept quiet.
func (s *Server) chatUrgency(info proto.SessionInfo, a proto.DecideAsk) (proto.Decision, error) {
	d, err := s.decideFor(info, a)
	if err != nil || d.Judge == "" {
		return d, err
	}
	d.Sure = s.sure(d)
	switch {
	case !d.Sure:
		return s.acted(d, proto.ActedNothing, 0), nil
	case d.Verdict == proto.VerdictUrgent:
		d = s.acted(d, proto.ActedUrgent, 0)
	case d.Verdict == proto.VerdictQuiet && !a.Mention:
		d = s.acted(d, proto.ActedQuiet, 0)
	default:
		d = s.acted(d, proto.ActedNothing, 0)
	}
	d.Sure = true
	return d, nil
}

// judgeCheck asks whether session's check run, just failed, is flaky or
// real, and shows the answer on the check (Runner.OnFailed).
func (s *Server) judgeCheck(session string, run proto.CheckRun) {
	if !s.useOn(proto.DecideFlaky) {
		return
	}
	sess, err := s.sessions.Get(session)
	if err != nil {
		return
	}
	info := sess.Info()
	if !s.runner.Update(session, run.Name, run.Started, func(r *proto.CheckRun) { r.Judging = true }) {
		return
	}
	go func() {
		d, err := s.decideFor(info, proto.DecideAsk{Use: proto.DecideFlaky, Command: run.Command, Output: s.maskLent(run.Output), Exit: run.ExitCode})
		sure := err == nil && s.sure(d)
		s.runner.Update(session, run.Name, run.Started, func(r *proto.CheckRun) {
			r.Judging = false
			if sure {
				r.Judged, r.JudgedWhy = d.Verdict, d.Why
			}
		})
		switch {
		case err != nil || d.Judge == "":
		case !sure:
			s.acted(d, proto.ActedNothing, 0)
		case d.Verdict == proto.VerdictFlaky:
			s.acted(d, proto.ActedFlaky, 0)
		default:
			s.acted(d, proto.ActedReal, 0)
		}
	}()
}
