package daemon

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// What a session's agent waits on the user for, read and answered from
// outside its terminal (another session, the command line), with the keys
// its preset says its user would press, as the chat does.

var sessionAskHandlers = map[string]handler{
	proto.MethodSessionAsk: with(func(c *conn, p proto.SessionRef) (any, error) { return c.srv.sessionAsk(p.ID) }),
	proto.MethodSessionAnswer: with(func(c *conn, p proto.SessionAnswerParams) (any, error) {
		return c.srv.answerSession(p)
	}),
	proto.MethodSessionInterrupt: with(func(c *conn, p proto.SessionRef) (any, error) {
		return ok, c.srv.interruptSession(p.ID)
	}),
}

// sessionAsk is what session id's agent waits on: the tool call under way
// while it waits, as its transcript says.
func (s *Server) sessionAsk(id string) (proto.SessionAsk, error) {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return proto.SessionAsk{}, err
	}
	info := sess.Info()
	if info.State != proto.StateWaiting || info.Exited {
		return proto.SessionAsk{}, nil
	}
	out := proto.SessionAsk{Waiting: true}
	t, err := s.transcript(proto.TranscriptParams{ID: id})
	if err != nil || !t.Supported {
		out.Reason = "its agent keeps no conversation enowX reads: its screen (read) says what it asks"
		return out, nil
	}
	var last *proto.TranscriptEntry
	for i := len(t.Entries) - 1; i >= 0; i-- {
		if e := t.Entries[i]; e.Kind == proto.TranscriptTool {
			last = &t.Entries[i]
			break
		}
	}
	if last == nil || last.Done {
		out.Reason = "it asks something its screen shows (read), not a tool"
		return out, nil
	}
	out.Tool, out.Detail, out.Questions = last.Text, withoutSecrets(clip(last.Detail, peekClip)), last.Questions
	keys := approveKeys(info.Agent)
	switch {
	case keys.Allow == "":
		out.Reason = "enowX doesn't know the keys of " + info.Agent + "'s answers: send them, as its screen says"
	case len(out.Questions) > 0:
		if q := out.Questions[0]; !q.Multi {
			for i := range q.Options {
				out.Answers = append(out.Answers, strconv.Itoa(i+1))
			}
		}
		if len(out.Answers) == 0 {
			out.Reason = "a question with several picks: answer it in its terminal"
		}
	default:
		out.Answers = []string{proto.AnswerAllow}
		if keys.Always != "" {
			out.Answers = append(out.Answers, proto.AnswerAlways)
		}
		out.Answers = append(out.Answers, proto.AnswerDeny)
	}
	return out, nil
}

func approveKeys(agentID string) agent.ApproveKeys {
	if p, ok := agent.Find(agentID); ok {
		return p.Approve
	}
	return agent.ApproveKeys{}
}

// answerSession presses the keys of p.Answer in its session's agent, when
// it waits for one of them.
func (s *Server) answerSession(p proto.SessionAnswerParams) (proto.SessionAsk, error) {
	ask, err := s.sessionAsk(p.ID)
	if err != nil {
		return ask, err
	}
	if !ask.Waiting {
		return ask, fmt.Errorf("answer: the session's agent isn't waiting for an answer")
	}
	answer := strings.ToLower(strings.TrimSpace(p.Answer))
	if !slices.Contains(ask.Answers, answer) {
		why := ask.Reason
		if why == "" {
			why = "it takes " + strings.Join(ask.Answers, ", ")
		}
		return ask, fmt.Errorf("answer: %q doesn't answer what it asks: %s", p.Answer, why)
	}
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return ask, err
	}
	keys := approveKeys(sess.Info().Agent)
	press := answer // a question's option, by its number
	switch answer {
	case proto.AnswerAllow:
		press = keys.Allow
	case proto.AnswerAlways:
		press = keys.Always
	case proto.AnswerDeny:
		press = keys.Deny
	}
	s.sessions.Input(sess, []byte(press))
	s.logger().Info("a session's agent answered from outside", "session", p.ID, "answer", answer, "tool", ask.Tool)
	return proto.SessionAsk{}, nil
}

// interruptSession stops what session id's agent does, with its own key.
func (s *Server) interruptSession(id string) error {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return err
	}
	key := "\x03"
	if p, ok := agent.Find(sess.Info().Agent); ok {
		key = p.InterruptKey()
	}
	s.sessions.Input(sess, []byte(key))
	return nil
}
