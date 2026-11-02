package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// How a team's chat reads: a member's messages in a row grouped under
// their avatar and name, the user's own on the right in the accent's
// tint, days apart, and what a message mentions lit in its text.

// chatGroupGap is how far apart a member's messages may be and still be
// grouped under one name.
const chatGroupGap = 5 * time.Minute

const chatAvatar = 26

// chatPad is how far the chat keeps from its frame's sides: the page's margin,
// a little less in the narrow side panel.
func (a *App) chatPad() float32 {
	if a.teamPageChat() != "" {
		return theme.S5
	}
	return theme.S3
}

// chatTextSize is what a message reads at: larger on the page.
func (a *App) chatTextSize() float32 {
	if a.teamPageChat() != "" {
		return theme.TextRead
	}
	return theme.TextUI
}

func (a *App) chatMessages(c *ui.Context, team string, msgs []proto.ChatMessage) {
	hc, me, now := a.chatsOf(), a.cloudOf().st.AccountID, time.Now()
	hc.list.FollowEnd = true
	ui.List(c, &hc.list, len(msgs), func(i int) {
		m := msgs[i]
		newDay := i == 0 || !sameDay(msgs[i-1].At, m.At)
		first := newDay || msgs[i-1].Account != m.Account || m.At.Sub(msgs[i-1].At) > chatGroupGap
		last := i == len(msgs)-1 || msgs[i+1].Account != m.Account || msgs[i+1].At.Sub(m.At) > chatGroupGap || !sameDay(m.At, msgs[i+1].At)
		ui.Column(c.Key("m:" + a.rowKey(m.ID))).FillWidth().Children(func() {
			if newDay {
				chatDay(c, m.At, now, a.chatPad())
			}
			bottom := float32(2)
			if last {
				bottom = 8
			}
			if m.Account == me && me != "" || a.outgoingOf(m.ID) != nil {
				a.ownMessage(c, team, m, first, bottom, now)
			} else {
				a.memberMessage(c, team, m, first, bottom, now)
			}
		})
	}).Grow(1).FillWidth().MinHeight(0).Padding(theme.S2, widget.ScrollGutter, theme.S2, 0)
}

func sameDay(x, y time.Time) bool {
	x, y = x.Local(), y.Local()
	return x.Year() == y.Year() && x.YearDay() == y.YearDay()
}

// chatDay is the line starting a day's messages.
func chatDay(c *ui.Context, at, now time.Time, pad float32) {
	label := at.Local().Format("Mon 2 Jan")
	switch {
	case sameDay(at, now):
		label = "Today"
	case sameDay(at, now.AddDate(0, 0, -1)):
		label = "Yesterday"
	}
	ui.Row(c).FillWidth().Gap(theme.S3).Padding(theme.S3, pad, theme.S2, pad).Children(func() {
		ui.Box(c).Grow(1).Height(theme.Rule).Background(theme.Border)
		ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
		ui.Box(c).Grow(1).Height(theme.Rule).Background(theme.Border)
	})
}

// memberAvatar is a member's initial on their color.
func memberAvatar(c *ui.Context, account, name string, size float32) ui.Element {
	initial := "?"
	if f := strings.Fields(strings.TrimPrefix(name, "@")); len(f) > 0 {
		initial = strings.ToUpper(string([]rune(f[0])[0]))
	}
	return ui.Box(c).Size(size, size).Radius(size / 2).Shrink(0).Background(widget.TeamColor(account)).Center().Children(func() {
		ui.Text(c, initial).FontSize(size * 0.48).FontWeight(700).TextColor(ui.RGB(255, 255, 255))
	})
}

// memberMessage is another member's message: on the left, under their
// avatar and name when it starts a group.
func (a *App) memberMessage(c *ui.Context, team string, m proto.ChatMessage, first bool, bottom float32, now time.Time) {
	ui.Row(c).FillWidth().Gap(theme.S2).Padding(0, a.chatPad(), bottom, a.chatPad()).AlignItems(ui.Start).Children(func() {
		if first {
			av := memberAvatar(c, m.Account, m.Name, chatAvatar).Margin(2, 0, 0, 0)
			if a.online(team, m.Account) {
				av.Children(func() { onlineDot(c, chatAvatar) })
			}
		} else {
			ui.Box(c).Width(chatAvatar).Shrink(0)
		}
		col := ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(theme.S1).AlignItems(ui.Start)
		col.Children(func() {
			if first {
				ui.Row(c.Key("who")).Gap(theme.S2).Shrink(1).MinWidth(0).Children(func() {
					ui.Text(c, m.Name).FontSize(theme.TextUI).FontWeight(500).SingleLine().Shrink(1)
					if m.GitHub != "" {
						ui.Text(c, "@"+m.GitHub).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(1)
					}
					chatTime(c, m.At, theme.TextSoft)
				})
			}
			ui.Row(c.Key("text")).MaxWidth(560).Shrink(1).MinWidth(0).Children(func() {
				// Another's message has no bubble but while a quote points at it.
				bg, border := theme.Fade(theme.Accent, 0), theme.Fade(theme.Accent, 0)
				if a.flashing(c, m.ID) {
					bg, border = theme.Fade(theme.Accent, 0.16), theme.Accent
				}
				a.chatBubble(c, team, m, bg, border, nil)
			})
			a.reactions(c, team, m)
			a.messageActions(c, team, m, 0, col.Hovered())
		})
	})
}

// ownMessage is the user's message, on the right in the accent's tint.
func (a *App) ownMessage(c *ui.Context, team string, m proto.ChatMessage, first bool, bottom float32, now time.Time) {
	col := ui.Column(c).FillWidth().Gap(theme.S1).Padding(0, a.chatPad(), bottom, a.chatPad()).AlignItems(ui.End)
	col.Children(func() {
		ui.Row(c).Gap(theme.S1).Shrink(1).MinWidth(0).Children(func() {
			border := theme.Accent
			if e := a.chatsOf().editing; e != nil && e.ID == m.ID || a.flashing(c, m.ID) {
				border = theme.OnAccent
			}
			head := func() {
				ui.Row(c.Key("who")).Gap(theme.S2).Children(func() {
					ui.Text(c, "You").FontSize(theme.TextUI).FontWeight(500).TextColor(theme.OnAccent)
					chatTime(c, m.At, theme.OnAccent)
				})
			}
			if !first {
				head = nil
			}
			a.chatBubble(c, team, m, theme.Accent, border, head)
		})
		if o := a.outgoingOf(m.ID); o != nil {
			a.outgoingState(c, o)
			return // nothing to react to or change until it is sent
		}
		a.reactions(c, team, m)
		a.messageActions(c, team, m, a.chatPad(), col.Hovered())
	})
}

// messageActions react to a message and answer it, as a member; edit the
// user's own; and delete it, theirs or as an admin any. They float over the
// message's top corner, right of its column's edge, while it's hovered, so a
// message doesn't change its width as the pointer passes over it.
func (a *App) messageActions(c *ui.Context, team string, m proto.ChatMessage, right float32, shown bool) {
	own := m.Account == a.cloudOf().st.AccountID && m.Account != ""
	writer, admin := a.chatWriter(team), a.teamAdmin(team)
	if !writer && !own && !admin {
		return
	}
	shown = shown || a.chatsOf().reacting == m.ID
	if !shown {
		return
	}
	widget.Popup(ui.Row(c.Key("actions")).Absolute().Top(0).Right(right).Gap(0).Padding(0)).Children(func() {
		if writer {
			a.reactButton(c, team, m)
			if widget.IconButton(c, icon.Reply, "Reply", false).Clicked() {
				a.startReply(m)
			}
		}
		if own && widget.IconButton(c, icon.Pencil, "Edit", false).Clicked() {
			a.startEditing(m)
		}
		if (own || admin) && widget.IconButton(c, icon.Trash, "Delete", false).Clicked() {
			cl := a.client
			a.confirm("Delete this message?", "It goes from the chat for everyone in "+a.spaceName(team)+", and the cloud keeps nothing of it.", "Delete", func() {
				go func() {
					if err := cl.DeleteChat(team, m.ID); err != nil {
						a.onMain(func() { a.chatsOf().err = strings.TrimPrefix(client.Explain(err), "teams: ") })
					}
				}()
			})
		}
	})
}

// teamAdmin says the user administers team.
func (a *App) teamAdmin(team string) bool {
	for _, sp := range a.spacesOf().list {
		if sp.Team == team {
			return canAdmin(sp.Role)
		}
	}
	return false
}

func chatTime(c *ui.Context, at time.Time, col ui.Color) {
	ui.Text(c, at.Local().Format("15:04")).FontSize(theme.TextLabel).TextColor(col).SingleLine().Tooltip(at.Local().Format("Mon 2 Jan 15:04"))
}

// chatBubble is a message's text, its mentions lit and opening what they
// name; one its text no longer names shows as a chip under it. head, on the
// user's own, goes first in it; what's on the accent reads in its text's color.
func (a *App) chatBubble(c *ui.Context, team string, m proto.ChatMessage, bg, border ui.Color, head func()) {
	box := ui.Column(c).Gap(theme.S1).MaxWidth(520).Shrink(1).MinWidth(0).Radius(theme.RadiusCard).Background(bg).Border(1, border)
	own := bg == theme.Accent
	if bg.A != 0 {
		box.Padding(theme.S2, theme.S4)
	}
	box.Children(func() {
		if head != nil {
			head()
		}
		a.replyQuote(c, team, m)
		a.messageImages(c, team, m)
		var loose []proto.ChatMention
		parts := splitMentions(m.Text, m.Mentions)
		if m.Text == "" && m.Edited == nil {
			return // images only
		}
		fg, quiet := theme.Text, theme.Idle
		if own {
			fg, quiet = theme.OnAccent, theme.OnAccent
		}
		ui.RichText(c).FontSize(a.chatTextSize()).TextColor(fg).Selectable().Children(func() {
			for i, p := range parts {
				if p.mention == nil {
					a.chatText(c.Key(i), team, p.text, own)
					continue
				}
				a.inlineMention(c.Key(i), team, *p.mention, own)
			}
			if m.Edited != nil {
				ui.Text(c.Key("edited"), "  (edited)").FontSize(theme.TextLabel).TextColor(quiet).Tooltip("Edited " + m.Edited.Local().Format("Mon 2 Jan 15:04"))
			}
		})
		for _, mn := range m.Mentions {
			if !strings.Contains(m.Text, "@"+mn.Title) {
				loose = append(loose, mn)
			}
		}
		if len(loose) > 0 {
			ui.Row(c).Gap(theme.S1).Wrap().Children(func() {
				for _, mn := range loose {
					a.mentionChip(c.Key(mn.Kind+":"+mn.ID), team, mn)
				}
			})
		}
	})
}

// textPart is a run of a message's text: plain, or a mention.
type textPart struct {
	text    string
	mention *proto.ChatMention
}

// splitMentions cuts text at each "@Title" of its mentions, longest title
// first where two start at the same place.
func splitMentions(text string, list []proto.ChatMention) []textPart {
	var out []textPart
	for text != "" {
		at, pick := -1, -1
		for i, mn := range list {
			if mn.Title == "" {
				continue
			}
			j := strings.Index(text, "@"+mn.Title)
			if j < 0 {
				continue
			}
			if at < 0 || j < at || (j == at && len(mn.Title) > len(list[pick].Title)) {
				at, pick = j, i
			}
		}
		if at < 0 {
			out = append(out, textPart{text: text})
			break
		}
		if at > 0 {
			out = append(out, textPart{text: text[:at]})
		}
		out = append(out, textPart{text: "@" + list[pick].Title, mention: &list[pick]})
		text = text[at+1+len(list[pick].Title):]
	}
	return out
}

// inlineMention is a mention within a message's text: the user's own in
// their color, what opens underlined on hover.
func (a *App) inlineMention(c *ui.Context, team string, mn proto.ChatMention, onAccent bool) {
	me := mn.Kind == proto.MentionUser && mn.ID == a.cloudOf().st.AccountID
	col := theme.Accent
	if mn.Kind == proto.MentionUser {
		col = widget.TeamColor(mn.ID)
	}
	if onAccent {
		col = theme.OnAccent
	}
	t := ui.Text(c, "@"+mn.Title).FontWeight(600).TextColor(col).Tooltip(mentionTip(mn))
	if me {
		t.Background(theme.Fade(col, 0.2))
	}
	if a.mentionOpens(mn) {
		t.Cursor(ui.CursorPointer)
		if t.Hovered() {
			t.Underline()
		}
		if t.Clicked() {
			a.openMention(team, mn)
		}
	}
}

func mentionTip(mn proto.ChatMention) string {
	switch mn.Kind {
	case proto.MentionUser:
		return "A member of the team"
	case proto.MentionFile:
		return "A file of " + strings.TrimPrefix(mn.Project, "git:")
	}
	return "The team's " + mn.Kind
}

// mentionChip is what a message names, under its text.
func (a *App) mentionChip(c *ui.Context, team string, mn proto.ChatMention) {
	b := ui.ButtonBase(c).Height(22).Padding(0, theme.S2, 0, theme.S2).Gap(theme.S1).Radius(theme.RadiusSmall).Background(theme.Sunken).Border(1, theme.Border).AlignItems(ui.Center).
		Label(mn.Kind + " " + mn.Title)
	if a.mentionOpens(mn) {
		b.Cursor(ui.CursorPointer)
	}
	b.Children(func() {
		ui.Icon(c, mentionIcon(mn.Kind)).FontSize(11).TextColor(theme.TextSoft)
		ui.Text(c, mn.Title).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
	})
	if b.Clicked() && a.mentionOpens(mn) {
		a.openMention(team, mn)
	}
}

func mentionIcon(kind string) *ui.SVG {
	switch kind {
	case proto.MentionNote:
		return icon.Notebook
	case proto.MentionCanvas:
		return icon.Canvas
	case proto.MentionProject:
		return icon.Folder
	case proto.MentionSession:
		return icon.Terminal
	case proto.MentionUser:
		return icon.Account
	case proto.MentionFile:
		return icon.File
	}
	return icon.Link
}

// mentionOpens says clicking mn shows what it names here.
func (a *App) mentionOpens(mn proto.ChatMention) bool {
	switch mn.Kind {
	case proto.MentionNote, proto.MentionCanvas, proto.MentionProject:
		return true
	case proto.MentionFile:
		_, ok := a.sessionOfTeamProject(mn.Project)
		return ok
	}
	return false
}

// openMention shows what mn names: a note or canvas on its page, a
// project in the Team group, a file in a session of its project.
func (a *App) openMention(team string, mn proto.ChatMention) {
	switch mn.Kind {
	case proto.MentionNote:
		a.openNote(mn.ID, team)
	case proto.MentionCanvas:
		a.showSpace(team)
		a.page = pageCanvas
		if hc := a.canvasesOf(); hc != nil {
			hc.open = mn.ID
		}
	case proto.MentionProject:
		a.openPanel(panelTeamProjects)
	case proto.MentionFile:
		if s, ok := a.sessionOfTeamProject(mn.Project); ok {
			a.selectSession(s)
			a.openFileTab(s, mn.ID)
		}
	}
}

// sessionOfTeamProject is the selected session when it works in the
// team's project ref, else another one that does.
func (a *App) sessionOfTeamProject(ref string) (proto.SessionInfo, bool) {
	if ref == "" {
		return proto.SessionInfo{}, false
	}
	in := func(s proto.SessionInfo) bool {
		p, ok := a.teamOfProject(s.Project)
		return ok && p.Ref == ref && !s.Exited
	}
	if s, ok := a.selectedSession(); ok && in(s) {
		return s, true
	}
	for _, s := range a.sessions {
		if in(s) {
			return s, true
		}
	}
	return proto.SessionInfo{}, false
}

// canvasLinkRe finds canvas:// links in a message: a canvas, or one of its
// designs.
var canvasLinkRe = regexp.MustCompile(`canvas://[A-Za-z0-9]+(?:/design-\d+)?`)

// chatText is a run of a message's plain text, its canvas:// links opening
// the design they name, in the team's space.
func (a *App) chatText(c *ui.Context, team, text string, onAccent bool) {
	link := theme.Accent
	if onAccent {
		link = theme.OnAccent
	}
	at := canvasLinkRe.FindAllStringIndex(text, -1)
	if len(at) == 0 {
		ui.Text(c, text)
		return
	}
	last := 0
	for i, span := range at {
		if span[0] > last {
			ui.Text(c.Key(fmt.Sprint("t", i)), text[last:span[0]])
		}
		target := text[span[0]:span[1]]
		l := ui.Text(c.Key(fmt.Sprint("l", i)), target).TextColor(link).Cursor(ui.CursorPointer).Tooltip(canvasLinkTip(target))
		if l.Hovered() {
			l.Underline()
		}
		if l.Clicked() {
			a.showSpace(team)
			a.openCanvasLink(target)
		}
		last = span[1]
	}
	if last < len(text) {
		ui.Text(c.Key("t-end"), text[last:])
	}
}

// canvasLinkTip says what a canvas:// link opens.
func canvasLinkTip(link string) string {
	if _, design, ok := canvas.ParseLink(link); ok && design != "" {
		return "Open " + strings.ReplaceAll(design, "-", " ") + " of the canvas"
	}
	return "Open the canvas"
}
