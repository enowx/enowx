package ui

import (
	"time"

	"github.com/enowx/enowx/proto"
)

// judgeOffFor is how long the host's judge isn't asked about messages
// after it couldn't weigh one (its use chat off, an older daemon).
const judgeOffFor = time.Minute

// notifyChatJudged tells the user of message m as the host's judge weighs
// it (use chat, docs/decide.md): urgent notifies whatever the chat's
// settings, quiet only counts as unread; anything else, or no answer, as
// the settings say. mention says it names or answers the user, which the
// judge never keeps quiet.
func (a *App) notifyChatJudged(m proto.ChatMessage, mention bool) {
	hc, cl := a.chatsOf(), a.client
	if cl == nil || time.Since(m.At) > chatToastAge || time.Since(hc.judgeOff) < judgeOffFor {
		a.notifyChat(m, mention)
		return
	}
	gen := a.gen
	ask := proto.DecideAsk{Use: proto.DecideChat, From: m.Name, Place: a.spaceName(m.Team), Text: m.Text, Mention: mention}
	go func() {
		d, err := cl.AskDecision(ask)
		a.onMain(func() {
			if gen != a.gen {
				return
			}
			switch {
			case err != nil:
				hc.judgeOff = time.Now()
				a.notifyChat(m, mention)
			case d.Acted == proto.ActedQuiet:
				// Counted unread, nothing more.
			case d.Acted == proto.ActedUrgent:
				a.notifyChatAs(m, mention, d.Why)
			default:
				a.notifyChat(m, mention)
			}
		})
	}()
}
