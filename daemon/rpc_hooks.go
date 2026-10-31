package daemon

import (
	"strings"

	"github.com/enowx/enowx/proto"
)

// hookHandlers keep the host's hooks and take the events agents' own
// hooks hand over (daemon/hooks.go).
var hookHandlers = map[string]handler{
	proto.MethodHooksList: with(func(c *conn, p proto.ProjectDir) (any, error) { return c.srv.hooksInfo(p.Dir), nil }),
	proto.MethodHooksSave: with(func(c *conn, p []proto.Hook) (any, error) {
		return c.srv.saveHostHooks(p)
	}),
	proto.MethodHooksSaveProject: with(func(c *conn, p proto.ProjectHooks) (any, error) {
		return ok, c.srv.saveProjectHooks(p)
	}),
	proto.MethodHooksSaveSession: with(func(c *conn, p proto.SessionHooks) (any, error) {
		return ok, c.srv.saveSessionHooks(p)
	}),
	proto.MethodHooksApprove: with(func(c *conn, p proto.HooksApproval) (any, error) {
		return ok, c.srv.approveHooks(p)
	}),
	proto.MethodHooksPropose: with(func(c *conn, p proto.HookProposal) (any, error) { return c.srv.propose(p) }),
	proto.MethodHooksDecide: with(func(c *conn, p proto.HookDecideParams) (any, error) {
		return ok, c.srv.decide(p)
	}),
	proto.MethodHooksTest: with(func(c *conn, p proto.HookTest) (any, error) { return c.srv.testHook(p) }),
	proto.MethodHooksLog:  bare(func(c *conn) (any, error) { return c.srv.hookLog(), nil }),
	proto.MethodSessionHook: with(func(c *conn, p proto.SessionHookParams) (any, error) {
		return c.srv.sessionHook(p)
	}),
}

// sessionHook takes event p from a session's agent: the state it means,
// the turn's start or end and the answer to a permission, then the
// user's hooks and what they decided; a Git commit, push or merge the
// agent is about to run goes by the hooks before Git too.
func (s *Server) sessionHook(p proto.SessionHookParams) (proto.HookDecision, error) {
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return proto.HookDecision{}, err
	}
	if p.State != "" {
		if err := s.sessions.SetState(sess, p.State); err != nil {
			return proto.HookDecision{}, err
		}
	}
	info := sess.Info()
	if !userSession(info) {
		return proto.HookDecision{}, nil // the app's own agents
	}
	ev := p.Event
	ev.Via = proto.HookInAgent
	s.permissionAnswer(info, ev)
	if ev.Event == proto.HookPermission {
		tool := ev.Tool
		if tool == "" {
			tool = "*"
		}
		s.permissionAsked(info, tool)
	}
	d := s.fireHooks(info, ev)
	if ev.Event == proto.HookToolBefore && !d.Block {
		cmd := ev.Command
		if d.Command != "" {
			cmd = d.Command
		}
		if op := gitOp(cmd); op != "" {
			g := s.fireHooks(info, proto.HookEvent{Event: proto.HookGitBefore, Op: op, Tool: ev.Tool, Command: cmd, Via: proto.HookInAgent})
			if g.Block {
				return proto.HookDecision{Block: true, Reason: g.Reason}, nil
			}
		}
	}
	dir := info.Project
	if dir == "" {
		dir = info.Dir
	}
	switch ev.Event {
	case proto.HookToolAfter:
		s.toolStep(info, ev)
	case proto.HookPromptStart:
		s.promptStep(info.ID)
		if !d.Block {
			go s.turnStarted(info)
			if extra := s.ragContext(info, dir, ev.Prompt); extra != "" {
				d.Context = strings.TrimSpace(d.Context + "\n\n" + extra)
			}
		}
	case proto.HookTurnDone:
		go s.turnEnded(info)
		s.ragSoon(dir)
	case proto.HookSessionEnd:
		go s.ragOffer(info)
	}
	return d, nil
}
