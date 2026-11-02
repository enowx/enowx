package daemon

import (
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// A permission prompt on an agent's screen, whatever started the agent: a
// Claude Code by hand in a shell tab, or a prompt that shows in bypass. No hook
// tells of all of those, so this goes by what shows (watchPrompts).
//
// The judge answers what it is sure of. What it isn't sure of, or what can't be
// undone, spends money, publishes or touches secrets goes to the owner, in time
// and saying how long there is. With no judge answering, GOD of Agents answers
// for its own workers, and otherwise the owner does, as without any of this.

// promptScreenLines is how much of the screen goes with a question about it:
// the command and the prompt's warning, and not much else.
const promptScreenLines = 40

// promptSeen decides what to do of permission prompt p on session info's screen.
// It runs off the watch's loop, a judge taking its time.
func (s *Server) promptSeen(info proto.SessionInfo, p agent.ScreenPrompt, screen string) {
	if !userSession(info) || !agent.PromptAsksPermission(p.Text) {
		return
	}
	text := s.maskLent(agent.ScreenEnd(screen, promptScreenLines))
	urgent := goa.Urgent("", text, nil)
	if s.useOn(proto.DecideApprove) {
		d, err := s.decideFor(info, proto.DecideAsk{Use: proto.DecideApprove,
			Command: "The agent's permission prompt, as its screen shows it, the command and its warning included:\n" + text})
		if err == nil && d.Judge != "" {
			sure := s.sure(d)
			switch {
			case sure && d.Verdict == proto.VerdictRefuse:
				if s.pressAnswer(info, p, proto.AnswerDeny) {
					s.acted(d, proto.ActedRefused, 0)
					return
				}
			case sure && d.Verdict == proto.VerdictAllow && urgent == "":
				if s.pressAnswer(info, p, proto.AnswerAllow) {
					s.acted(d, proto.ActedAllowed, 0)
					return
				}
			}
			s.acted(d, proto.ActedNothing, 0)
			s.promptToOwner(info, p, urgent)
			return
		}
	}
	// No judge set up, or none answering.
	if g, ok := s.goaSession(); ok && !g.Exited && urgent == "" && (info.GOAWorker != "" || s.goa.board.Has(info.ID)) {
		s.promptToGOA(info, g, p, text)
		return
	}
	s.promptToOwner(info, p, urgent)
}

// pressAnswer presses the key of answer in session info's agent, if the prompt
// still shows, and says whether it did.
func (s *Server) pressAnswer(info proto.SessionInfo, p agent.ScreenPrompt, answer string) bool {
	sess, err := s.sessions.Get(info.ID)
	if err != nil {
		return false
	}
	screen := sess.Terminal().Text()
	now, ok := agent.PromptShown(screen)
	if !ok || now.Text != p.Text {
		return false
	}
	keys := agent.PromptKeys(info.Agent, screen)
	press := keys.Allow
	if answer == proto.AnswerDeny {
		press = keys.Deny
	}
	if press == "" {
		return false
	}
	s.promptAnswered.Store(info.ID, true)
	s.sessions.Input(sess, []byte(press))
	s.logger().Info("a permission prompt answered by the judge", "session", info.ID, "answer", answer)
	return true
}

// promptToOwner tells the user a prompt is theirs, with the time it gives. A
// worker's that is urgent is marked so, for GOA not to answer it.
func (s *Server) promptToOwner(info proto.SessionInfo, p agent.ScreenPrompt, urgent string) {
	if urgent != "" && (info.GOAWorker != "" || s.goa.board.Has(info.ID)) {
		s.goa.mu.Lock()
		s.goa.urgent[info.ID] = urgent
		s.goa.mu.Unlock()
	}
	text := nonEmpty(info.Name, s.agentLabel(info.Agent)) + " asks: " + clip(oneLine(p.Text), 200)
	if urgent != "" {
		text += " (" + urgent + ")"
	}
	if p.Deadline != "" {
		text += " It decides by itself in " + p.Deadline + "."
	}
	s.broadcast(proto.EventNotifySound, proto.NotifySoundParams{Kind: proto.NotifyInput, Session: info.ID, Text: text})
}

// promptToGOA has GOA answer a worker's prompt, told now, busy or not: the
// prompt won't wait for its turn to end, and what is typed waits in its input.
func (s *Server) promptToGOA(info, g proto.SessionInfo, p agent.ScreenPrompt, text string) {
	when := "before it times out"
	if p.Deadline != "" {
		when = "within " + p.Deadline
	}
	line := "[enowX GOA] Session " + info.ID + " shows a permission prompt and no judge answers it. Read it, and answer " + when +
		" (answer " + info.ID + " allow|deny). What can't be undone, spends money, publishes or touches secrets is the user's: don't answer those.\n" + clip(text, 2000)
	if err := s.input(proto.SessionInputParams{ID: g.ID, Text: line, Paste: true, Enter: true}); err != nil {
		s.logger().Warn("goa: tell of a prompt", "session", info.ID, "err", err)
		s.promptToOwner(info, p, "")
	}
}

// promptGone records a prompt that gave a time and left the screen with no
// answer of the judge's, as an expired one does. The agent is told by its own
// refusal; GOA by what watchPrompts tells it.
func (s *Server) promptGone(info proto.SessionInfo, text, deadline string) {
	d := proto.Decision{ID: newRenderID(), Use: proto.DecideApprove, Session: info.ID, Project: sessionProject(info),
		Agent: agent.Base(info.Agent), Time: time.Now(), Input: clip(strings.TrimSpace(s.maskLent(text)), 300),
		Acted: proto.ActedPromptGone, Why: "it gave " + deadline + " and left the screen"}
	s.judge.Record(d)
	s.broadcast(proto.EventDecision, d)
}
