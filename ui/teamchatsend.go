package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// Sending what the composer holds: a new message, with its mentions,
// images and the message it answers, or the text of the user's message
// being edited.

// sendChat sends text to team's chat with the mentions it still names,
// or saves it over the message being edited, and empties the draft (the
// one put aside, after an edit); false when there's nothing to send.
func (a *App) sendChat(team, text string) bool {
	hc, cl := a.chatsOf(), a.client
	images := a.imagesSendable()
	if hc.editing != nil {
		images = nil // an edit changes the text only
	}
	if strings.TrimSpace(text) == "" && len(images) == 0 || cl == nil {
		return false
	}
	editing := hc.editing
	var mentions []proto.ChatMention
	for _, mn := range hc.mentions {
		// A mention the message held as a chip, not written in its text,
		// stays through an edit: only "@Title" taken out drops one.
		chip := editing != nil && !strings.Contains(editing.Text, "@"+mn.Title)
		if (chip || strings.Contains(text, "@"+mn.Title)) && !slices.ContainsFunc(mentions, func(x proto.ChatMention) bool { return x.Kind == mn.Kind && x.ID == mn.ID }) {
			mentions = append(mentions, mn)
		}
	}
	replyTo := ""
	if hc.replyTo != nil && editing == nil {
		replyTo = hc.replyTo.ID
	}
	hc.draft, hc.mentions, hc.editing, hc.replyTo = "", nil, nil, nil
	a.stoppedTyping(team)
	hc.err = ""
	if editing == nil {
		a.outboxAdd(team, text, mentions, replyTo)
		return true
	}
	hc.draft, hc.saved = hc.saved, ""
	go func() {
		_, err := cl.EditChat(proto.ChatEditParams{Team: team, ID: editing.ID, Text: text, Mentions: mentions})
		a.onMain(func() {
			hc.err = ""
			if err != nil {
				hc.err = strings.TrimPrefix(client.Explain(err), "teams: ")
			}
		})
	}()
	return true
}

// outboxAdd puts a message in the list now, with the draft's images, and starts it
// on its way.
func (a *App) outboxAdd(team, text string, mentions []proto.ChatMention, replyTo string) {
	hc := a.chatsOf()
	hc.nextKey++
	o := &outgoing{key: fmt.Sprintf("out:%d", hc.nextKey), team: team, text: text, mentions: mentions, replyTo: replyTo,
		at: time.Now(), images: hc.pending, known: map[string]bool{}}
	hc.pending = nil
	for _, m := range hc.byTeam[team] {
		o.known[m.ID] = true
	}
	for i, p := range o.images {
		hc.images[localImageID(o, i)] = &chatImage{bmp: p.preview, asked: true}
	}
	hc.out = append(hc.out, o)
	hc.list.ScrollToEnd()
	a.outboxStep(team)
}

// startEditing puts the user's message m in the composer to change it,
// the draft put aside meanwhile.
func (a *App) startEditing(m proto.ChatMessage) {
	hc := a.chatsOf()
	if hc.editing == nil {
		hc.saved = hc.draft
	}
	hc.editing = &m
	hc.draft, hc.mentions = m.Text, slices.Clone(m.Mentions)
	hc.setCaret, hc.focus, hc.dismissed = len([]rune(m.Text)), true, m.Text
}

// stopEditing leaves the message as it was and gives the draft back.
func (a *App) stopEditing() {
	hc := a.chatsOf()
	if hc.editing == nil {
		return
	}
	hc.editing, hc.mentions = nil, nil
	hc.draft, hc.saved = hc.saved, ""
	hc.setCaret, hc.focus = len([]rune(hc.draft)), true
}

// lastOwnMessage is the user's last message in team's chat.
func (a *App) lastOwnMessage(team string) (proto.ChatMessage, bool) {
	me, msgs := a.cloudOf().st.AccountID, a.chatsOf().byTeam[team]
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Account == me && me != "" {
			return msgs[i], true
		}
	}
	return proto.ChatMessage{}, false
}
