package daemon

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/core/autopush"
	"github.com/enowx/enowx/proto"
)

// An agent reading other sessions when the user asks it to (docs/cli.md,
// sessions everywhere and peek): this host's, and through the app every
// host it reaches. A session's conversation comes as its agent recorded
// it, the last messages only, a line that looks like a secret taken out;
// its environment, credentials and files never come. Its project is read
// with project context and project read, read-only.

var sessionPeekHandlers = map[string]handler{
	proto.MethodSessionsEverywhere: bare(func(c *conn) (any, error) { return c.srv.sessionsEverywhere(), nil }),
	proto.MethodSessionPeek:        with(func(c *conn, p proto.SessionPeekParams) (any, error) { return c.srv.peekSession(p) }),
}

const (
	peekLast    = 40   // messages read when none is said
	peekMax     = 200  // the most read at once
	peekClip    = 4000 // a message's most characters
	peekToolMax = 200  // a tool call's
)

// sessionsEverywhere are this host's sessions, then those of every other
// host the app reaches.
func (s *Server) sessionsEverywhere() []proto.HostSessions {
	here := proto.HostSessions{Host: s.daemonID, HostName: s.userdata.Name, Sessions: []proto.SessionInfo{}}
	for _, sess := range s.sessions.List() {
		here.Sessions = append(here.Sessions, sess.Info())
	}
	out := []proto.HostSessions{here}
	raw, err := s.askApp("sessions.everywhere", struct{}{}, 30*time.Second)
	if err != nil {
		return out // no app open: this host's alone
	}
	var others []proto.HostSessions
	if json.Unmarshal(raw, &others) == nil {
		for _, h := range others {
			if h.Host != s.daemonID {
				out = append(out, h)
			}
		}
	}
	return out
}

// peekSession reads a session's conversation, here or on another host.
func (s *Server) peekSession(p proto.SessionPeekParams) (proto.SessionPeek, error) {
	if p.Host != "" && p.Host != s.daemonID && !strings.EqualFold(p.Host, s.userdata.Name) {
		s.goaHeardOf(p.From, p.Session)
		raw, err := s.askApp("session.peek", p, time.Minute)
		if err != nil {
			return proto.SessionPeek{}, err
		}
		var out proto.SessionPeek
		return out, json.Unmarshal(raw, &out)
	}
	var info proto.SessionInfo
	found := 0
	for _, sess := range s.sessions.List() {
		i := sess.Info()
		if i.ID == p.Session {
			info, found = i, 1
			break
		}
		if strings.EqualFold(i.Name, p.Session) {
			info = i
			found++
		}
	}
	switch found {
	case 0:
		return proto.SessionPeek{}, fmt.Errorf("sessions: no session %q on %s", p.Session, s.userdata.Name)
	case 1:
	default:
		return proto.SessionPeek{}, fmt.Errorf("sessions: %d sessions are named %q on %s: use the ID", found, p.Session, s.userdata.Name)
	}
	s.goaHeardOf(p.From, info.ID)
	out := proto.SessionPeek{Host: s.daemonID, HostName: s.userdata.Name, Session: info, Messages: []proto.PeekMessage{}}
	t, err := s.transcript(proto.TranscriptParams{ID: info.ID})
	if err != nil {
		return out, err
	}
	if !t.Supported {
		out.Reason = t.Reason
		if out.Reason == "" {
			out.Reason = "its agent keeps no conversation enowX reads"
		}
		return out, nil
	}
	var all []proto.PeekMessage
	for _, e := range t.Entries {
		m := proto.PeekMessage{Role: e.Kind, Time: e.Time, Text: e.Text}
		switch e.Kind {
		case proto.TranscriptUser, proto.TranscriptAssistant:
		case proto.TranscriptTool:
			m.Text = clip(strings.TrimSpace(e.Text+" "+e.Detail), peekToolMax)
		default:
			continue
		}
		if strings.TrimSpace(m.Text) == "" {
			continue
		}
		m.Text = withoutSecrets(clip(m.Text, peekClip))
		all = append(all, m)
	}
	last := p.Last
	if last <= 0 {
		last = peekLast
	}
	last = min(last, peekMax)
	if len(all) > last {
		out.Earlier, all = len(all)-last, all[len(all)-last:]
	}
	out.Messages = append(out.Messages, all...)
	s.logger().Info("a session's conversation read by an agent", "session", info.ID, "name", info.Name)
	return out, nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// withoutSecrets is text with each line that looks like it holds a secret
// taken out.
func withoutSecrets(text string) string {
	found := autopush.ScanSecrets("message", []byte(text))
	if len(found) == 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	for _, f := range found {
		if f.Line > 0 && f.Line <= len(lines) {
			lines[f.Line-1] = "[taken out: it looks like " + f.Kind + "]"
		}
	}
	return strings.Join(lines, "\n")
}

func init() {
	// A member's agent reads a session of a shared project, one it sees,
	// and the team's log records it.
	visitorMethods[proto.MethodSessionPeek] = api.RoleViewer
	visitorOverrides[proto.MethodSessionPeek] = with(func(c *conn, p proto.SessionPeekParams) (any, error) {
		v := c.visitor.Load()
		// Found among the sessions they see alone: one they don't is as
		// missing as one that isn't, by ID or by name.
		scope := c.visitorScope()
		var match []string
		for _, sess := range c.srv.sessions.List() {
			if info := sess.Info(); (info.ID == p.Session || strings.EqualFold(info.Name, p.Session)) && visibleTo(info, scope) {
				match = append(match, info.ID)
			}
		}
		switch len(match) {
		case 0:
			return nil, fmt.Errorf("sessions: no session %q shared with your team on this host", p.Session)
		case 1:
		default:
			return nil, fmt.Errorf("sessions: %d sessions are named %q on this host: use the ID", len(match), p.Session)
		}
		p.Host, p.Session = "", match[0]
		out, err := c.srv.peekSession(p)
		if err != nil {
			return nil, err
		}
		if !visibleTo(out.Session, scope) {
			return nil, fmt.Errorf("sessions: no session %q shared with your team on this host", p.Session)
		}
		out.Session = forVisitor(out.Session)
		go c.srv.recordAudit(v.Team, api.AuditParams{Action: api.AuditRead, Detail: visitorLabel(*v) + " · " + c.srv.sessionLabel(out.Session.ID)})
		return out, nil
	})
}
