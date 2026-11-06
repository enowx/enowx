package daemon

import (
	"errors"
	"fmt"

	"github.com/enowx/enowx/proto"
)

// Restarting GOD of Agents on its agent and config, going on with its conversation: the
// way of a change of config (goa_switch.go), the same session and all it holds. A GOA
// whose agent ended, leaving its shell, isn't "ended" to continue or wake, and one that
// ended right after starting is refused them for a while: restarting stops it whatever it
// is, and is the user's to ask for (docs/goa.md).

func (s *Server) goaRestart(p proto.GOASwitchParams) (proto.GOASwitchResult, error) {
	g, ok := s.goaSession()
	if !ok {
		return proto.GOASwitchResult{}, errors.New("goa: no GOD of Agents on this host: start it first (goa start)")
	}
	if p.From == g.ID && !p.Check {
		return proto.GOASwitchResult{}, errors.New("goa: GOD of Agents can't restart itself, as that ends the turn it asks in: tell the user to restart it from GOA's header, or with `goa restart` in a terminal")
	}
	sess, err := s.sessions.Get(g.ID)
	if err != nil {
		return proto.GOASwitchResult{}, err
	}
	meta := sess.Meta()
	// Its agent and config, checked as a change to them would be; then its conversation.
	plan, err := s.goaPlanRun(sess, proto.GOASwitchParams{Agent: meta.Agent, Config: nonEmpty(meta.Account, proto.GOAConfigOfficial)}, true)
	if err != nil {
		return proto.GOASwitchResult{}, err
	}
	unresumable := ""
	plan.conversation, plan.convo, err = s.goaConversationFor(sess, plan.account)
	switch {
	case err != nil && !errors.Is(err, errGOAConversation):
		return proto.GOASwitchResult{}, err
	case err != nil:
		unresumable = err.Error()
	case plan.conversation == proto.GOAConversationNone:
		unresumable = "goa: it has no conversation of its own yet to go on with"
	}
	plan.same = false
	if unresumable != "" || p.NewConversation {
		// A new conversation, on the same agent: as GOA starts.
		plan.toAgent, plan.conversation, plan.convo = true, proto.GOAConversationNew, ""
	}
	res := goaSwitchResultOf(g, plan)
	res.Unresumable = unresumable
	if p.Check {
		return res, nil
	}
	if res.AtWork && !p.Force || unresumable != "" && !p.NewConversation {
		return res, nil // not a word of it done: the yes it needs first
	}
	if err := s.goaKeyReadable(g.ID, plan.account); err != nil {
		return proto.GOASwitchResult{}, fmt.Errorf("goa: GOD of Agents keeps running as it was: %w", err)
	}
	if !s.goaHoldLines() {
		return proto.GOASwitchResult{}, errors.New("goa: GOD of Agents is being changed already")
	}
	defer s.goaReleaseLines()
	s.sessions.ForgetQuickExits(sess) // the user asks it to start again
	started, err := s.goaRunSwitch(sess, plan)
	if err != nil {
		if plan.conversation == proto.GOAConversationSame {
			err = fmt.Errorf("%w. To start it on a new conversation instead: goa restart --new-conversation", err)
		}
		return proto.GOASwitchResult{}, err
	}
	res.Session, res.Restarted, res.Mode = started.Info(), true, started.Meta().Mode
	return res, nil
}
