package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/enowx/enowx/proto"
)

// The sessions the user always allows to hibernate and close sessions, without
// a card, when nothing would be lost. It is this daemon that holds the rule, as
// it holds the asking session: the app only offers the choice, and checks
// another host's sessions, which only it reaches. Kept in a file, as sessions
// outlive the daemon; a session's entry says nothing once it is gone, its ID
// never coming back.

// controlAllowFile is where the choices are kept.
func (s *Server) controlAllowFile() string { return filepath.Join(s.dataDir, "control-allow.json") }

// controlAllows reads the sessions allowed, by ID, with since when. The caller
// holds controlAllowMu.
func (s *Server) controlAllows() map[string]time.Time {
	out := map[string]time.Time{}
	if data, err := os.ReadFile(s.controlAllowFile()); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

// listControlAllows are the sessions allowed that are still there.
func (s *Server) listControlAllows() []proto.ControlAllow {
	s.controlAllowMu.Lock()
	allows := s.controlAllows()
	s.controlAllowMu.Unlock()
	out := []proto.ControlAllow{}
	for id, since := range allows {
		sess, err := s.sessions.Get(id)
		if err != nil {
			continue
		}
		info := sess.Info()
		out = append(out, proto.ControlAllow{Session: id, Name: nonEmpty(info.Name, s.agentLabel(info.Agent)), Since: since})
	}
	slices.SortFunc(out, func(a, b proto.ControlAllow) int { return a.Since.Compare(b.Since) })
	return out
}

// setControlAllow keeps or revokes p's choice. A teammate's session on this
// host is never allowed: it doesn't speak for the owner.
func (s *Server) setControlAllow(p proto.ControlAllowParams) error {
	if !p.Off {
		sess, err := s.sessions.Get(p.Session)
		if err != nil {
			return err
		}
		if sess.Info().Member != nil {
			return errors.New("control: a teammate's session always asks")
		}
	}
	s.controlAllowMu.Lock()
	defer s.controlAllowMu.Unlock()
	allows := s.controlAllows()
	for id := range allows {
		if _, err := s.sessions.Get(id); err != nil {
			delete(allows, id) // gone: while here, so the file doesn't only grow
		}
	}
	if p.Off {
		delete(allows, p.Session)
	} else {
		allows[p.Session] = time.Now()
	}
	data, err := json.MarshalIndent(allows, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.controlAllowFile(), data, 0o600)
}

// alwaysAllowed says the user always allows session from to do method, which
// can only be hibernating or closing. What it would lose still asks: the caller
// sees to that.
func (s *Server) alwaysAllowed(from, method string) bool {
	if from == "" || (method != proto.MethodSessionKill && method != proto.MethodSessionHibernate) {
		return false
	}
	sess, err := s.sessions.Get(from)
	if err != nil {
		return false
	}
	if info := sess.Info(); info.Exited || info.Member != nil {
		return false
	}
	s.controlAllowMu.Lock()
	defer s.controlAllowMu.Unlock()
	_, ok := s.controlAllows()[from]
	return ok
}

// stoppingLoses says stopping session id of this host, for session from, would
// lose something, or isn't from's to decide: an agent at work, something served,
// an app shown, a program left running; what can't come back on its conversation
// (a shell, say, whose plan says no more of what runs in it); its own session, or
// a teammate's. One that has ended loses nothing.
func (s *Server) stoppingLoses(from, id string, ports map[string][]proto.Port) bool {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return false // nothing to lose: what is asked says it isn't there
	}
	info := sess.Info()
	if id == from || info.Member != nil {
		return true
	}
	if info.Exited {
		return false
	}
	plan := s.hibernatePlan(sess, ports)
	return plan.Blocked != "" || plan.Busy()
}

// closeLoses is stoppingLoses for host call p, a session.kill.
func (s *Server) closeLoses(p proto.HostCallParams) bool {
	var ref proto.SessionRef
	if err := json.Unmarshal(p.Params, &ref); err != nil {
		return true
	}
	ports, _ := s.allPorts()
	return s.stoppingLoses(p.From, ref.ID, ports)
}
