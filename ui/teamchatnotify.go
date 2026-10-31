package ui

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A team member's message while its chat isn't open: a sound, a card in
// the window's top right corner to read it, answer it or open the chat
// from, and a desktop banner while the window is in the background; by
// the notification settings (all messages, mentions only, or none).

const (
	chatToastLife = 8 * time.Second // how long a card shows, unless hovered or being answered
	chatToastMax  = 3               // cards shown at once, the oldest going first
	chatToastAge  = 2 * time.Minute // older messages, brought by a sync, notify nothing
)

// chatToast is a message's card. It belongs to the main thread.
type chatToast struct {
	msg      proto.ChatMessage
	mention  bool
	urgent   string    // why the judge found it urgent, "" when it didn't
	shown    time.Time // when it showed, or was last hovered
	replying bool
	draft    string
	focus    bool
	sending  bool
	err      string
}

// chatMode is how team's messages notify: "" all, "mentions", "off".
func chatMode(st proto.NotifySettings, team string) string {
	if m, ok := st.ChatTeams[team]; ok {
		return m
	}
	return st.Chat
}

// notifyChat tells the user of message m, by their settings: mention
// says it names them or answers them, which always notifies.
func (a *App) notifyChat(m proto.ChatMessage, mention bool) { a.notifyChatAs(m, mention, "") }

// notifyChatAs is notifyChat with why, when the judge found m urgent:
// it notifies whatever the chat's settings.
func (a *App) notifyChatAs(m proto.ChatMessage, mention bool, urgent string) {
	st := a.notifySettings()
	mode := chatMode(st, m.Team)
	if !mention && urgent == "" && (mode == "off" || mode == "mentions") || time.Since(m.At) > chatToastAge {
		return
	}
	a.queueSound(proto.NotifyChat)
	what := snippet(m.Text)
	if what == "" && len(m.Attachments) > 0 {
		what = "Sent a picture"
	}
	if a.win != nil && !a.win.IsFocused() && !st.NoBanner && !a.state.NotifyOff {
		title := m.Name
		if mention {
			title += " mentioned you"
		}
		if urgent != "" {
			title = "Urgent · " + title
		}
		n := mygo.NewNotification(mygo.NotificationOptions{ID: "chat:" + m.Team, Title: title, Subtitle: a.spaceName(m.Team), Body: what, Silent: true})
		gen, msg := a.gen, m
		n.OnClick(func() {
			a.onMain(func() {
				if gen == a.gen {
					a.openChatAt(msg)
				}
			})
			a.showWindow()
		})
		go func() { _ = n.Show() }()
	}
	a.chatToasts = append(a.chatToasts, &chatToast{msg: m, mention: mention, urgent: urgent, shown: time.Now()})
	if over := len(a.chatToasts) - chatToastMax; over > 0 {
		a.chatToasts = slices.Delete(a.chatToasts, 0, over)
	}
}

// openChatAt opens team chat of m in the side panel, m lit.
func (a *App) openChatAt(m proto.ChatMessage) {
	a.state.Panel.Team = m.Team
	a.openPanel(panelChat)
	hc := a.chatsOf()
	hc.flash, hc.flashAt = m.ID, time.Now()
	a.chatToasts = slices.DeleteFunc(a.chatToasts, func(t *chatToast) bool { return t.msg.Team == m.Team })
}

// chatToastsView draws the cards, dropping those whose time is up.
func (a *App) chatToastsView(c *ui.Context) {
	now := time.Now()
	a.chatToasts = slices.DeleteFunc(a.chatToasts, func(t *chatToast) bool {
		return !t.replying && !t.sending && now.Sub(t.shown) > chatToastLife
	})
	if len(a.chatToasts) == 0 {
		return
	}
	ui.Column(c.Key("chat-toasts")).Absolute().Top(52).Right(16).Width(360).Gap(8).TextColor(theme.Text).Children(func() {
		for _, t := range slices.Clone(a.chatToasts) {
			a.chatToastCard(c.Key("toast:"+t.msg.ID), t, now)
		}
	})
}

func (a *App) chatToastCard(c *ui.Context, t *chatToast, now time.Time) {
	m := t.msg
	border := theme.BorderStrong
	if t.mention {
		border = theme.AccentLine
	}
	if t.urgent != "" {
		border = theme.Error
	}
	card := widget.Float(ui.Column(c).FillWidth().Padding(12, 12, 12, 12).Gap(8).Radius(theme.RadiusCard).
		Background(theme.Dialog).Border(1, border)).Transition(widget.OverlayMotion)
	if card.Hovered() {
		t.shown = now // read: it waits
	} else if !t.replying {
		c.After(chatToastLife - now.Sub(t.shown) + 50*time.Millisecond)
	}
	card.Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
			memberAvatar(c, m.Account, m.Name, 30)
			body := ui.ButtonBase(c.Key("open")).Grow(1).MinWidth(0).Cursor(ui.CursorPointer).Label("Open the chat").Tooltip("Open the chat")
			body.Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, m.Name).FontSize(12.5).FontWeight(600).TextColor(widget.TeamColor(m.Account)).SingleLine().Shrink(1)
						where := a.spaceName(m.Team)
						if t.mention {
							where = "mentioned you · " + where
						}
						if t.urgent != "" {
							where = "urgent · " + where
						}
						ui.Text(c, where).FontSize(11).TextColor(theme.Muted).SingleLine().Shrink(1)
					})
					text := snippet(m.Text)
					if text == "" && len(m.Attachments) > 0 {
						text = "Sent a picture"
					}
					ui.Text(c, text).FontSize(12.5).TextColor(theme.TextSoft).MaxLines(3)
					if t.urgent != "" {
						ui.Text(c, "Urgent, the judge says: "+t.urgent).FontSize(11).TextColor(theme.Muted).MaxLines(2)
					}
				})
			})
			if body.Clicked() {
				a.openChatAt(m)
			}
			if widget.IconButton(c.Key("close"), icon.Close, "Dismiss", false).Clicked() {
				a.chatToasts = slices.DeleteFunc(a.chatToasts, func(x *chatToast) bool { return x == t })
			}
		})
		if t.err != "" {
			ui.Text(c, t.err).FontSize(11.5).TextColor(theme.Error)
		}
		if !t.replying {
			ui.Row(c).FillWidth().Gap(6).Padding(0, 0, 0, 40).Children(func() {
				if a.chatWriter(m.Team) && widget.Button(c.Key("reply"), widget.Secondary, icon.Reply, "Reply", "").Height(26).Clicked() {
					t.replying, t.focus = true, true
				}
				if widget.GhostButton(c.Key("open-chat"), "Open chat").Height(26).Clicked() {
					a.openChatAt(m)
				}
			})
			return
		}
		ui.Row(c).FillWidth().Gap(6).Padding(0, 0, 0, 40).AlignItems(ui.Center).Children(func() {
			in := widget.Input(c.Key("reply-in"), &t.draft, "Reply to "+m.Name).Grow(1).Height(30)
			if t.focus {
				in.Focus()
				t.focus = false
			}
			if t.sending {
				in.Disabled(true)
			}
			send := widget.IconButton(c.Key("send"), icon.Send, "Send (Enter)", false).Clicked()
			if (in.Submitted() || send) && strings.TrimSpace(t.draft) != "" && !t.sending {
				a.sendToastReply(t)
			}
			if c.Shortcut(0, ui.KeyEscape) {
				t.replying, t.shown = false, now
			}
		})
	})
}

// sendToastReply sends the card's draft as an answer to its message.
func (a *App) sendToastReply(t *chatToast) {
	cl := a.client
	if cl == nil {
		return
	}
	t.sending, t.err = true, ""
	p := proto.ChatSendParams{Team: t.msg.Team, Text: strings.TrimSpace(t.draft), ReplyTo: t.msg.ID}
	go func() {
		_, err := cl.SendChat(p)
		a.onMain(func() {
			t.sending = false
			if err != nil {
				t.err = client.Explain(err)
				return
			}
			a.chatToasts = slices.DeleteFunc(a.chatToasts, func(x *chatToast) bool { return x == t })
		})
	}()
}
