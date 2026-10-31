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
			go c.srv.goaWorkerChanged(p.Session)
		}
		return ok, nil
	}),
	proto.MethodGOAFlags:  bare(func(c *conn) (any, error) { return c.srv.goaFlags(), nil }),
	proto.MethodGOARevert: with(func(c *conn, p proto.GOARevertParams) (any, error) { return c.srv.goaRevert(p.ID) }),
	proto.MethodGOARemember: with(func(c *conn, p proto.GOARememberParams) (any, error) {
		return c.srv.goaRemember(p)
	}),
	proto.MethodGOARecall: with(func(c *conn, p proto.GOARecallParams) (any, error) { return c.srv.goaRecall(p) }),
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
	for _, m := range []string{proto.MethodGOADelegate, proto.MethodGOAOverride, proto.MethodGOARecall, proto.MethodGOARevert} {
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
