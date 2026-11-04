package daemon

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/enowx/enowx/cloud/api"
	cloudclient "github.com/enowx/enowx/cloud/client"
	"github.com/enowx/enowx/proto"
)

// What a visitor holds lasts only while it is theirs to hold: a project
// unshared takes their views of its sessions and their control of them
// with it; a team the host no longer shares with, or isn't in, takes
// their connection; and the cloud is asked now and then whether each is
// still a member of the team and in which role, a member gone being let
// go and one made a viewer losing what a viewer can't do.

// visitorRecheck is how often the cloud is asked who visitors are.
var visitorRecheck = 2 * time.Minute

// watchVisitors rechecks visitors every visitorRecheck until stop closes.
func (s *Server) watchVisitors(stop <-chan struct{}) {
	t := time.NewTicker(visitorRecheck)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-s.done:
			return
		case <-t.C:
			s.recheckVisitors("")
		}
	}
}

// visitorSnapshot is who is connected, copied.
func (s *Server) visitorSnapshot() map[*conn]proto.Visitor {
	s.visitors.mu.Lock()
	defer s.visitors.mu.Unlock()
	out := make(map[*conn]proto.Visitor, len(s.visitors.conns))
	for c, v := range s.visitors.conns {
		out[c] = v
	}
	return out
}

// recheckVisitors asks the cloud, with the host's own token, who of team's
// connected visitors (every team's for "") is still its member and in
// which role, and lets go of those who aren't. A cloud out of reach
// changes nothing; one saying the host isn't in the team any more lets
// go of the team's visitors.
func (s *Server) recheckVisitors(team string) {
	snap := s.visitorSnapshot()
	teams := map[string]bool{}
	for _, v := range snap {
		if team == "" || v.Team == team {
			teams[v.Team] = true
		}
	}
	if len(teams) == 0 {
		return
	}
	st, err := s.cloud.store.Get()
	if err != nil || !st.SignedIn() {
		for c, v := range snap {
			if teams[v.Team] {
				c.closedBecause("daemon: visitors signed out")
				c.close() // signed out, the host lets nobody in
			}
		}
		return
	}
	cl, err := cloudclient.New(st.URL, st.Token)
	if err != nil {
		return
	}
	for t := range teams {
		ctx, cancel := context.WithTimeout(context.Background(), cloudTimeout)
		got, err := cl.Team(ctx, t)
		cancel()
		roles := map[string]string{}
		switch {
		case err == nil:
			for _, m := range got.Members {
				roles[m.Account.ID] = m.Role
			}
		case refused(err):
			// The host is no longer the team's: nobody of it stays.
		default:
			s.logger().Warn("cloud: could not recheck the team's visitors", "team", t, "err", err)
			continue
		}
		for c, v := range snap {
			if v.Team != t {
				continue
			}
			role, ok := roles[v.Account]
			switch {
			case !ok:
				s.logger().Info("a visitor is no longer the team's member", "account", v.Account, "team", t)
				c.close()
			case role != v.Role:
				s.logger().Info("a visitor's role changed", "account", v.Account, "team", t, "role", role)
				s.setVisitorRole(c, role)
			}
		}
	}
	s.enforceScopes()
}

// refused says the cloud answered err: the host may not see the team.
func refused(err error) bool {
	var e *cloudclient.Error
	return errors.As(err, &e) && e.Status >= 400 && e.Status < 500 && e.Status != http.StatusTooManyRequests
}

// setVisitorRole gives connection c's visitor role from now on.
func (s *Server) setVisitorRole(c *conn, role string) {
	old := c.visitor.Load()
	if old == nil {
		return
	}
	v := *old
	v.Role = role
	c.visitor.Store(&v)
	s.visitors.mu.Lock()
	if _, ok := s.visitors.conns[c]; ok {
		s.visitors.conns[c] = v
	}
	s.visitors.mu.Unlock()
}

// enforceScopes takes from visitors what is no longer shared with them:
// their connection when their team's share is empty or the host left the
// team, their views of sessions out of what is shared, and control of a
// session they no longer see or, made viewers, may no longer type into.
func (s *Server) enforceScopes() {
	snap := s.visitorSnapshot()
	shared := s.readShare().Teams
	for c, v := range snap {
		scope := shared[v.Team]
		if _, ok := s.spaceByTeam(v.Team); !ok || len(scope) == 0 {
			c.close()
			delete(snap, c)
			continue
		}
		c.mu.Lock()
		var gone []uint32
		for id, st := range c.streams {
			if !visibleTo(st.sess.Info(), scope) {
				gone = append(gone, id)
			}
		}
		c.mu.Unlock()
		for _, id := range gone {
			c.detach(id)
		}
	}
	for session, account := range s.takeover.all() {
		keep := false
		for c := range snap {
			if v := c.visitor.Load(); v != nil && v.Account == account && rank[v.Role] >= rank[api.RoleMember] && c.sessionInScope(session) {
				keep = true
			}
		}
		if keep {
			continue
		}
		s.takeover.set(session, "")
		end := proto.TakeoverAnswer{Session: session, Account: account}
		for _, vc := range s.visitorConns(account) {
			vc.event(proto.EventTakeoverAnswered, end)
		}
	}
	s.visitorsChanged()
}
