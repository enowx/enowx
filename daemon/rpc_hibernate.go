package daemon

import "github.com/enowx/enowx/proto"

var hibernateHandlers = map[string]handler{
	proto.MethodSessionHibernate: with(func(c *conn, p proto.SessionHibernateParams) (any, error) {
		mode := ""
		if !c.remote {
			mode = c.srv.sessionBypasses(p.From)
			if mode == "" && c.srv.alwaysAllowed(p.From, proto.MethodSessionHibernate) {
				mode = proto.ControlAlways
			}
		}
		return c.srv.hibernateSessions(p, mode)
	}),
	proto.MethodSessionWake: with(func(c *conn, p proto.SessionRef) (any, error) {
		sess, err := c.srv.wakeSession(p.ID)
		if err != nil {
			return nil, err
		}
		return sess.Info(), nil
	}),
	proto.MethodHibernatePrefs: bare(func(c *conn) (any, error) {
		return proto.HibernatePrefs{NeverByDefault: c.srv.hibernatePrefs().NeverByDefault}, nil
	}),
	proto.MethodSessionAutoHibernate: with(func(c *conn, p proto.SessionAutoHibernateParams) (any, error) {
		return ok, c.srv.setAutoHibernate(p)
	}),
}
