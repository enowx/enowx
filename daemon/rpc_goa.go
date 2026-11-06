package daemon

import (
	"github.com/enowx/enowx/proto"
)

// goaHandlers are GOD of Agents' methods (goa*.go).
var goaHandlers = map[string]handler{
	proto.MethodGOAAgents: bare(func(c *conn) (any, error) { return goaAgents(), nil }),
	proto.MethodGOABoard:  bare(func(c *conn) (any, error) { return c.srv.goaBoard(), nil }),
	proto.MethodGOADelegate: with(func(c *conn, p proto.GOADelegateParams) (any, error) {
		return c.srv.goaDelegate(p)
	}),
	proto.MethodGOATask: with(func(c *conn, p proto.GOATaskParams) (any, error) { return c.srv.goaSetTask(p) }),
	proto.MethodGOAOverride: with(func(c *conn, p proto.GOAOverrideParams) (any, error) {
		return c.srv.goaOverride(p)
	}),
	proto.MethodGOAWorker: with(func(c *conn, p proto.GOAWorkerParams) (any, error) {
		if c.srv.goa.board.Has(p.Session.ID) {
			go c.srv.goaWorkerSeen(p.Session)
		} else {
			go c.srv.goaToldChanged(p.Session)
		}
		return ok, nil
	}),
	proto.MethodGOAReport: with(func(c *conn, p proto.GOAReportParams) (any, error) { return ok, c.srv.goaReport(p) }),
	proto.MethodGOAWake: with(func(c *conn, p proto.GOAWakeParams) (any, error) {
		return c.srv.goaWake(p.ID, c.gone), nil
	}),
	proto.MethodGOAFlags:  bare(func(c *conn) (any, error) { return c.srv.goaFlags(), nil }),
	proto.MethodGOARevert: with(func(c *conn, p proto.GOARevertParams) (any, error) { return c.srv.goaRevert(p.ID) }),
	proto.MethodGOARemember: with(func(c *conn, p proto.GOARememberParams) (any, error) {
		return c.srv.goaRemember(p)
	}),
	proto.MethodGOARecall: with(func(c *conn, p proto.GOARecallParams) (any, error) { return c.srv.goaRecall(p) }),
	proto.MethodGOARules:  bare(func(c *conn) (any, error) { return c.srv.goaRules(), nil }),
	proto.MethodGOARulesSave: with(func(c *conn, p proto.GOARules) (any, error) {
		r, err := c.srv.saveGOARules(p)
		if err == nil {
			c.srv.refreshRules()
		}
		return r, err
	}),
	proto.MethodGOASwitch:  with(func(c *conn, p proto.GOASwitchParams) (any, error) { return c.srv.goaSwitch(p) }),
	proto.MethodGOARestart: with(func(c *conn, p proto.GOASwitchParams) (any, error) { return c.srv.goaRestart(p) }),
	proto.MethodGOAStall:   bare(func(c *conn) (any, error) { return c.srv.goaStall(), nil }),
	proto.MethodGOAStallSave: with(func(c *conn, p proto.GOAStall) (any, error) {
		return c.srv.saveGOAStall(p)
	}),
}

func init() {
	// Set here, as GOA's handlers reach other methods' through host.call,
	// which the handlers table can't name in its own initializer.
	for m, h := range goaHandlers {
		if _, dup := handlers[m]; dup {
			panic("daemon: method registered twice: " + m)
		}
		handlers[m] = h
	}
	// Delegating waits on another host through the app, and on the judge.
	for _, m := range []string{proto.MethodGOARestart, proto.MethodGOADelegate, proto.MethodGOAOverride, proto.MethodGOARecall, proto.MethodGOARevert, proto.MethodGOASwitch} {
		slowMethods[m] = true
	}
}

// goaID is GOA's session here, "" for none running.
func (s *Server) goaID() string {
	if g, ok := s.goaSession(); ok && !g.Exited {
		return g.ID
	}
	return ""
}
