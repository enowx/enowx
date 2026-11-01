package daemon

import (
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// Sessions read for what stops them, whatever started them: an agent's hooks
// don't tell of every prompt, and a shell tab has none. A session found asking
// goes to waiting, which puts it on GOA's board and in its conversation.

const (
	promptEvery  = 3 * time.Second
	promptStill  = 3 * time.Second  // a listed prompt's screen, still this long
	stalledStill = 20 * time.Second // anything else that looks like one
)

// promptFollow is a session's screen as last read. Only watchPrompts touches it.
type promptFollow struct {
	text     string
	since    time.Time
	asking   bool // this loop put it in waiting
	deadline string
}

// watchPrompts reads sessions' screens until done closes.
func (s *Server) watchPrompts(done <-chan struct{}) {
	follows := map[string]*promptFollow{}
	tick := time.NewTicker(promptEvery)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
		}
		seen := map[string]bool{}
		for _, sess := range s.sessions.List() {
			info := sess.Info()
			if info.Exited || info.Queued || info.Parent != "" || info.GOA {
				continue
			}
			// Its hooks say it finished its turn: what shows is its answer.
			if info.State == proto.StateDone {
				continue
			}
			if _, asked := s.controlAsks.Load(info.ID); asked {
				continue
			}
			seen[info.ID] = true
			f := follows[info.ID]
			if f == nil {
				f = &promptFollow{since: time.Now()}
				follows[info.ID] = f
			}
			text := sess.Terminal().Text()
			if text != f.text {
				f.text, f.since = text, time.Now()
			}
			p, asking := promptOn(text, time.Since(f.since))
			switch {
			case asking && info.State != proto.StateWaiting:
				f.asking, f.deadline = true, p.Deadline
				s.sessions.SetState(sess, proto.StateWaiting)
			case !asking && f.asking && text != "" && time.Since(f.since) >= promptStill:
				if info.State == proto.StateWaiting {
					s.sessions.SetState(sess, proto.StateWorking)
				}
				if f.deadline != "" && s.goa.board.Has(info.ID) {
					s.goaTell("[enowX GOA] Session " + info.ID + "'s prompt, which gave " + f.deadline + ", no longer shows: it was answered or ran out. Read it (read " + info.ID + ").")
				}
				f.asking, f.deadline = false, ""
			}
		}
		for id := range follows {
			if !seen[id] {
				delete(follows, id)
			}
		}
	}
}

// promptOn is what a screen still this long asks.
func promptOn(text string, still time.Duration) (agent.ScreenPrompt, bool) {
	if still < promptStill {
		return agent.ScreenPrompt{}, false
	}
	if p, ok := agent.PromptShown(text); ok {
		return p, true
	}
	if still >= stalledStill {
		return agent.StalledPrompt(text)
	}
	return agent.ScreenPrompt{}, false
}
