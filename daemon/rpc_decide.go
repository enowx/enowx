package daemon

import (
	"context"
	"time"

	"github.com/enowx/enowx/proto"
)

// decideHandlers are the judge's methods (daemon/decide.go).
var decideHandlers = map[string]handler{
	proto.MethodDecideSettings: bare(func(c *conn) (any, error) { return c.srv.judge.Settings().Get(), nil }),
	proto.MethodDecideSetSettings: with(func(c *conn, p proto.DecideSettings) (any, error) {
		if err := c.srv.judge.Settings().Set(p); err != nil {
			return nil, err
		}
		return c.srv.judge.Settings().Get(), nil
	}),
	proto.MethodDecideJudges: bare(func(c *conn) (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return c.srv.judge.Judges(ctx), nil
	}),
	proto.MethodDecideTest: with(func(c *conn, p proto.JudgeRef) (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return c.srv.judge.Test(ctx, p.ID), nil
	}),
	proto.MethodDecideAsk: with(func(c *conn, p proto.DecideAsk) (any, error) {
		var info proto.SessionInfo
		if p.Session != "" {
			sess, err := c.srv.sessions.Get(p.Session)
			if err != nil {
				return nil, err
			}
			info = sess.Info()
		}
		switch p.Use {
		case proto.DecideTrim:
			return c.srv.trimOutput(info, p)
		case proto.DecideChat:
			return c.srv.chatUrgency(info, p)
		}
		if info.Agent == "" {
			info.Agent = p.Agent
		}
		d, err := c.srv.decideFor(info, p)
		d.Sure = err == nil && c.srv.sure(d)
		return d, err
	}),
	proto.MethodDecideRoute: with(func(c *conn, p proto.DecideRouteParams) (any, error) { return c.srv.routeFor(p) }),
	proto.MethodDecideLog:   with(func(c *conn, p proto.DecideLogParams) (any, error) { return c.srv.judge.Log(p), nil }),
	proto.MethodDecideMark:  with(func(c *conn, p proto.DecideMark) (any, error) { return ok, c.srv.judge.Mark(p) }),
}
