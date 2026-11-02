package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Messages on their way: on send a message joins the list at once, its pictures
// from the local bytes, and the composer clears. Its images finish uploading and it
// is sent from here, one at a time per team so that they stay in order. The real
// message takes its place in the list, in its row. One that fails stays, with Retry.

// outgoing is a message not yet known to be sent. It belongs to the main thread.
type outgoing struct {
	key      string // its ID in the list until the real one comes
	team     string
	text     string
	mentions []proto.ChatMention
	replyTo  string
	at       time.Time
	images   []*pendingImage
	// known are the messages there when it was sent: what comes after, of the same
	// words and images, is this one, whether its answer or its event comes first.
	known   map[string]bool
	sending bool
	tried   bool // sent at least once, so it may have got there
	err     string
}

// outboxStep sends what is next to go in team's chat, if it can go yet.
func (a *App) outboxStep(team string) {
	hc := a.chatsOf()
	for _, o := range hc.out {
		if o.team != team || o.err != "" {
			continue
		}
		if o.sending {
			return
		}
		ready := true
		for _, p := range o.images {
			if p.err != "" {
				o.err = p.err
				ready = false
				break
			}
			ready = ready && p.att != nil
		}
		if o.err != "" {
			continue
		}
		if ready {
			a.outboxSend(o)
		}
		return // its images come first, and what follows after it
	}
}

// outboxSend sends o, whose images are all up.
func (a *App) outboxSend(o *outgoing) {
	cl := a.client
	if cl == nil {
		o.err = "Not connected"
		return
	}
	atts := make([]proto.ChatAttachment, 0, len(o.images))
	for _, p := range o.images {
		atts = append(atts, *p.att)
	}
	o.sending, o.tried = true, true
	p := proto.ChatSendParams{Team: o.team, Text: o.text, Mentions: o.mentions, ReplyTo: o.replyTo, Attachments: atts}
	go func() {
		m, err := cl.SendChat(p)
		a.onMain(func() {
			o.sending = false
			if err != nil {
				o.err = strings.TrimPrefix(client.Explain(err), "teams: ")
			} else {
				a.outboxDone(o, m)
			}
			a.outboxStep(o.team)
		})
	}()
}

// outboxDone has real message m take o's place, in its row. Whichever comes first
// of the answer and the event, the list shows one of them.
func (a *App) outboxDone(o *outgoing, m proto.ChatMessage) {
	hc := a.chatsOf()
	if !slices.Contains(hc.out, o) {
		return
	}
	hc.out = slices.DeleteFunc(slices.Clone(hc.out), func(x *outgoing) bool { return x == o })
	if hc.rowOf == nil {
		hc.rowOf = map[string]string{}
	}
	hc.rowOf[m.ID] = o.key
	if !slices.ContainsFunc(hc.byTeam[m.Team], func(x proto.ChatMessage) bool { return x.ID == m.ID }) {
		hc.byTeam[m.Team] = append(hc.byTeam[m.Team], m)
	}
	for i := range o.images {
		delete(hc.images, localImageID(o, i))
	}
}

// sameContent says m has o's words and images.
func (o *outgoing) sameContent(m proto.ChatMessage) bool {
	if strings.TrimSpace(m.Text) != strings.TrimSpace(o.text) || len(m.Attachments) != len(o.images) {
		return false
	}
	for i, p := range o.images {
		if p.att == nil || p.att.ID != m.Attachments[i].ID {
			return false
		}
	}
	return true
}

// outboxSettle matches what has reached the list with what was sent.
func (a *App) outboxSettle(team, me string) {
	hc := a.chatsOf()
	for _, o := range slices.Clone(hc.out) {
		if o.team != team || !o.tried {
			continue
		}
		for _, m := range hc.byTeam[team] {
			if !o.known[m.ID] && m.Account == me && o.sameContent(m) {
				o.sending = false
				a.outboxDone(o, m)
				break
			}
		}
	}
}

func localImageID(o *outgoing, i int) string { return fmt.Sprintf("%s:%d", o.key, i) }

// chatShown is team's messages with those on their way after them.
func (a *App) chatShown(team string) []proto.ChatMessage {
	hc, me := a.chatsOf(), a.cloudOf().st.AccountID
	if len(hc.out) == 0 {
		return hc.byTeam[team]
	}
	a.outboxSettle(team, me)
	msgs := hc.byTeam[team]
	for _, o := range hc.out {
		if o.team != team {
			continue
		}
		m := proto.ChatMessage{ID: o.key, Team: team, At: o.at, Account: me, Name: "You", Text: o.text, Mentions: o.mentions, ReplyTo: o.replyTo}
		for i, p := range o.images {
			w, h := 0, 0
			if p.preview != nil {
				w, h = p.preview.Size()
			}
			m.Attachments = append(m.Attachments, proto.ChatAttachment{ID: localImageID(o, i), Name: p.name, Width: w, Height: h})
		}
		msgs = append(msgs[:len(msgs):len(msgs)], m)
	}
	return msgs
}

// outgoingOf is the message on its way shown as id, nil for a real one.
func (a *App) outgoingOf(id string) *outgoing {
	for _, o := range a.chatsOf().out {
		if o.key == id {
			return o
		}
	}
	return nil
}

// uploadOf is the image shown as attachment id while it is on its way.
func (a *App) uploadOf(id string) *pendingImage {
	for _, o := range a.chatsOf().out {
		for i, p := range o.images {
			if localImageID(o, i) == id {
				return p
			}
		}
	}
	return nil
}

// rowKey is the row of message id: a message sent from here keeps the one it was
// shown in on its way, for nothing in it to start over.
func (a *App) rowKey(id string) string {
	if k, ok := a.chatsOf().rowOf[id]; ok {
		return k
	}
	return id
}

// outboxRetry tries o again: its failed images, then the message.
func (a *App) outboxRetry(o *outgoing) {
	o.err = ""
	for _, p := range o.images {
		if p.err != "" {
			p.err = ""
			a.uploadChatImage(o.team, p)
		}
	}
	a.outboxStep(o.team)
}

// outboxReturn takes o back to the composer to change it: nothing is lost.
func (a *App) outboxReturn(o *outgoing) {
	hc := a.chatsOf()
	hc.out = slices.DeleteFunc(slices.Clone(hc.out), func(x *outgoing) bool { return x == o })
	if hc.draft != "" && o.text != "" {
		hc.draft += "\n"
	}
	hc.draft += o.text
	hc.mentions = append(hc.mentions, o.mentions...)
	hc.pending = append(hc.pending, o.images...)
	for i := range o.images {
		delete(hc.images, localImageID(o, i))
	}
	hc.setCaret, hc.focus = len([]rune(hc.draft)), true
	a.outboxStep(o.team)
}

// outgoingState is what a message on its way says under its bubble. Only a failure
// takes room: one on its way is as tall as it will be once sent.
func (a *App) outgoingState(c *ui.Context, o *outgoing) {
	if o.err == "" {
		return
	}
	ui.Row(c.Key("failed")).Gap(theme.S2).AlignItems(ui.Center).Shrink(1).MinWidth(0).MaxWidth(520).Children(func() {
		ui.Icon(c, icon.Alert).FontSize(12).TextColor(theme.Error)
		ui.Text(c, "Not sent: "+o.err).FontSize(theme.TextLabel).TextColor(theme.Error).MaxLines(2).Shrink(1).MinWidth(0)
		if widget.Button(c.Key("retry"), widget.Secondary, nil, "Retry", "").Clicked() {
			a.outboxRetry(o)
		}
		if widget.Button(c.Key("edit"), widget.Secondary, nil, "Edit", "").Tooltip("Back to the composer").Clicked() {
			a.outboxReturn(o)
		}
	})
}
