package ui

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Answering a team chat's message and reacting to it: the answer quotes
// the message it answers, a click going back to it; reactions show under
// a message, by emoji, the user's own lit, a click taking theirs back.

// quickEmojis are the reactions offered.
var quickEmojis = []string{"👍", "❤️", "😂", "🎉", "😮", "😭", "🙏", "🔥", "👀", "✅"}

// chatFlash is how long the message a quote went back to stays lit.
const chatFlash = 1500 * time.Millisecond

// chatWriter says the user writes in team's chat: not a viewer.
func (a *App) chatWriter(team string) bool {
	for _, sp := range a.spacesOf().list {
		if sp.Team == team {
			return sp.Role != "viewer"
		}
	}
	return false
}

// startReply has the composer answer m.
func (a *App) startReply(m proto.ChatMessage) {
	hc := a.chatsOf()
	if hc.editing != nil {
		a.stopEditing()
	}
	hc.replyTo, hc.focus = &m, true
}

// replyBar says which message the draft answers, in the composer.
func (a *App) replyBar(c *ui.Context) {
	hc := a.chatsOf()
	if hc.replyTo == nil {
		return
	}
	m := *hc.replyTo
	ui.Row(c).FillWidth().Gap(theme.S2).PaddingX(theme.S1).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon.Reply).FontSize(12).TextColor(theme.Accent)
		ui.Text(c, "Replying to").FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(0)
		ui.Text(c, m.Name).FontSize(theme.TextLabel).FontWeight(600).TextColor(widget.TeamColor(m.Account)).SingleLine().Shrink(0)
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			widget.Running(c, snippet(m.Text), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		if widget.IconButton(c, icon.Close, "Don't reply (Esc)", false).Clicked() {
			hc.replyTo = nil
		}
	})
}

// snippet is a message's text on one line.
func snippet(text string) string { return strings.Join(strings.Fields(text), " ") }

// replyQuote is the message m answers, at the top of its bubble: a
// block in its author's color, a click scrolling back to it.
func (a *App) replyQuote(c *ui.Context, team string, m proto.ChatMessage) {
	if m.ReplyTo == "" {
		return
	}
	hc := a.chatsOf()
	msgs := hc.byTeam[team]
	i := slices.IndexFunc(msgs, func(x proto.ChatMessage) bool { return x.ID == m.ReplyTo })
	col, name, text := theme.Idle, "", "A message no longer in the chat"
	if i >= 0 {
		o := msgs[i]
		col, name, text = widget.TeamColor(o.Account), o.Name, snippet(o.Text)
		if o.Account == a.cloudOf().st.AccountID {
			name = "You"
		}
	}
	// In the user's own bubble it lies on the accent, in the accent's text color.
	soft := theme.TextSoft
	if m.Account == a.cloudOf().st.AccountID && m.Account != "" {
		col, soft = theme.OnAccent, theme.OnAccent
	}
	b := ui.ButtonBase(c).FillWidth().MinWidth(160).Radius(theme.RadiusSmall).Background(theme.Fade(col, 0.12)).Label("The message it answers")
	b.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Stretch).Children(func() {
			ui.Box(c).Width(3).Shrink(0).Radius(theme.RadiusTail).Background(col)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Padding(theme.S1, theme.S2, theme.S1, theme.S2).Children(func() {
				if name != "" {
					ui.Text(c, name).FontSize(theme.TextLabel).FontWeight(650).TextColor(col).SingleLine()
				}
				t := ui.Text(c, text).FontSize(theme.TextLabel).TextColor(soft).MaxLines(2)
				if i < 0 {
					t.Italic().TextColor(theme.Idle)
				}
			})
		})
	})
	if i >= 0 {
		b.Cursor(ui.CursorPointer).Tooltip("Go to the message")
		if b.Hovered() {
			b.Background(theme.Fade(col, 0.2))
		}
		if b.Clicked() {
			hc.list.ScrollTo(i, ui.Center)
			hc.flash, hc.flashAt = m.ReplyTo, time.Now()
		}
	}
}

// flashing says message id is the one a quote just went back to.
func (a *App) flashing(c *ui.Context, id string) bool {
	hc := a.chatsOf()
	if hc.flash != id {
		return false
	}
	if left := chatFlash - time.Since(hc.flashAt); left > 0 {
		c.After(left)
		return true
	}
	hc.flash = ""
	return false
}

// reactions are m's reactions, under it: who reacted in each's tooltip,
// the user's own lit; a click adds the user's or takes it back.
func (a *App) reactions(c *ui.Context, team string, m proto.ChatMessage) {
	if len(m.Reactions) == 0 {
		return
	}
	me, writer := a.cloudOf().st.AccountID, a.chatWriter(team)
	ui.Row(c).Gap(theme.S1).Wrap().Children(func() {
		for _, r := range m.Reactions {
			mine := slices.Contains(r.Accounts, me)
			b := ui.ButtonBase(c.Key("r:"+r.Emoji)).Height(22).Padding(0, theme.S2).Gap(theme.S1).Radius(theme.RadiusSmall).AlignItems(ui.Center).
				Label(r.Emoji + " by " + a.reactors(team, r.Accounts)).Tooltip(a.reactors(team, r.Accounts))
			bg, border := theme.Sunken, theme.Border
			if mine {
				bg, border = theme.Fade(theme.Accent, 0.16), theme.Fade(theme.Accent, 0.5)
			}
			if writer {
				b.Cursor(ui.CursorPointer)
				if b.Hovered() {
					border = theme.BorderStrong
				}
			}
			b.Background(bg).Border(1, border).Children(func() {
				ui.Text(c, r.Emoji).FontSize(theme.TextLabel)
				ui.Text(c, strconv.Itoa(len(r.Accounts))).FontSize(theme.TextLabel).FontWeight(600).TextColor(theme.TextSoft)
			})
			if writer && b.Clicked() {
				a.react(team, m.ID, r.Emoji, mine)
			}
		}
	})
}

// reactors names the members of accounts.
func (a *App) reactors(team string, accounts []string) string {
	me := a.cloudOf().st.AccountID
	names := make([]string, 0, len(accounts))
	for _, acc := range accounts {
		name := "a former member"
		if acc == me {
			name = "you"
		}
		for _, m := range a.teamMembers(team) {
			if m.Account.ID == acc && acc != me {
				name = memberTitle(m.Account.Name, m.Account.GitHub)
			}
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// react adds the user's reaction emoji to message id, or with off takes
// it back; the chat shows it by its event.
func (a *App) react(team, id, emoji string, off bool) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		if _, err := cl.ReactChat(proto.ChatReactParams{Team: team, ID: id, Emoji: emoji, Off: off}); err != nil {
			a.onMain(func() { a.chatsOf().err = strings.TrimPrefix(client.Explain(err), "teams: ") })
		}
	}()
}

// reactButton offers the quick reactions to m, in a popover.
func (a *App) reactButton(c *ui.Context, team string, m proto.ChatMessage) {
	hc := a.chatsOf()
	b := widget.IconButton(c, icon.SmilePlus, "React", hc.reacting == m.ID)
	if b.Clicked() {
		if hc.reacting == m.ID {
			hc.reacting = ""
		} else {
			hc.reacting = m.ID
		}
	}
	if hc.reacting != m.ID {
		return
	}
	open := true
	ui.PopoverBase(c, b, &open, func(panel ui.Element) {
		widget.Popup(panel.AttachTo(b, ui.AnchorBottomRight, ui.AnchorTopRight).Margin(0, 0, theme.S2, 0))
		me := a.cloudOf().st.AccountID
		ui.Row(c).Gap(2).Children(func() {
			for _, e := range quickEmojis {
				mine := slices.ContainsFunc(m.Reactions, func(r proto.ChatReaction) bool { return r.Emoji == e && slices.Contains(r.Accounts, me) })
				eb := ui.ButtonBase(c.Key(e)).Size(30, 30).Radius(theme.RadiusControl).Center().Label("React with " + e).Cursor(ui.CursorPointer)
				if mine {
					eb.Background(theme.Fade(theme.Accent, 0.18))
				} else if eb.Hovered() {
					eb.Background(theme.Hover)
				}
				eb.Children(func() { ui.Text(c, e).FontSize(theme.TextRead) })
				if eb.Clicked() {
					a.react(team, m.ID, e, mine)
					hc.reacting = ""
				}
			}
		})
	})
	if !open {
		hc.reacting = ""
	}
}
