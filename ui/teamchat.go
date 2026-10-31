package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A team's chat (docs/cloud.md), the side panel's Team group: the team of
// the selected session's project, else the space shown. Its messages come
// through the team's space, read again every few seconds while it shows;
// "@" in a message mentions the team's members, notes, canvases, projects
// and the files of the session's project (teamchatcompose.go).

// panelChat is the side panel's chat tool.
const panelChat = "chat"

// chatRefresh is how often an open chat takes what the team sent.
const chatRefresh = 5 * time.Second

// membersRefresh is how often members shown are read again: who joined,
// left or changed role shows without reconnecting.
const membersRefresh = 15 * time.Second

// hostChats is the connected host's teams' chats, as last read, and the
// composer's draft. It belongs to the main thread.
type hostChats struct {
	host   string
	byTeam map[string][]proto.ChatMessage
	list   ui.ListState // the messages, following the latest
	// listFor is the team list shows; shown and wasShown say the chat
	// was drawn this frame and the last, so it opens at its latest.
	listFor         string
	shown, wasShown bool
	draft           string
	mentions        []proto.ChatMention // picked for the draft, kept while "@Title" is in it
	pulled          time.Time
	err             string

	members    map[string][]api.Member // by team, read from the cloud
	membersAt  map[string]time.Time    // when each team's were last asked for
	membersErr map[string]string

	// The "@" being typed: its menu's pick, the draft it was closed for,
	// and the caret to put back after a completion.
	editing *proto.ChatMessage // the user's message the draft changes, not a new one
	replyTo *proto.ChatMessage // the message the draft answers (teamchatreply.go)
	// reacting is the message whose reactions are offered; flash the one
	// a quote went back to, since flashAt.
	reacting string
	flash    string
	flashAt  time.Time
	// judgeOff is when the host's judge last couldn't weigh a message (its
	// use chat off, an older daemon): it isn't asked again for a while.
	judgeOff time.Time

	// The draft's images, the images fetched by ID, and the one opened
	// large, of message viewingOf (teamchatimages.go).
	pending   []*pendingImage
	nextKey   int
	images    map[string]*chatImage
	viewing   *proto.ChatAttachment
	viewingOf proto.ChatMessage
	saved     string // the draft before editing began, put back after

	pick      int
	dismissed string
	setCaret  int // -1: none
	focus     bool

	// Files of the session's project found for a query, as last asked.
	files      []string
	filesFor   string
	filesAsked string
}

func (a *App) chatsOf() *hostChats {
	if a.teamChats == nil || a.teamChats.host != a.host.ID {
		a.teamChats = &hostChats{host: a.host.ID, byTeam: map[string][]proto.ChatMessage{},
			members: map[string][]api.Member{}, membersAt: map[string]time.Time{}, membersErr: map[string]string{}, setCaret: -1,
			images: map[string]*chatImage{}}
	}
	return a.teamChats
}

// chatTeam is the team the panel's Team group shows: the one the user
// switched to, else the selected session's project's, else the page's.
func (a *App) chatTeam() string {
	if t := a.state.Panel.Team; t != "" && slices.ContainsFunc(a.spacesOf().list, func(sp proto.Space) bool { return sp.Team == t }) {
		return t
	}
	if s, ok := a.selectedSession(); ok {
		if p, ok := a.teamOfProject(s.Project); ok {
			return p.Team
		}
	}
	if t := a.space(); t != "" {
		return t
	}
	if list := a.spacesOf().list; len(list) > 0 {
		return list[0].Team
	}
	return ""
}

// loadChat reads a team's chat from the host.
func (a *App) loadChat(team string) {
	hc, cl := a.chatsOf(), a.client
	if cl == nil {
		return
	}
	go func() {
		list, err := cl.TeamChat(team, 0)
		a.onMain(func() {
			if err != nil {
				hc.err = client.Explain(err)
				return
			}
			hc.byTeam[team] = list
		})
	}()
}

// teamMembers is team's members as the cloud last told, read again when
// asked for after membersRefresh.
func (a *App) teamMembers(team string) []api.Member {
	hc, cl := a.chatsOf(), a.client
	if time.Since(hc.membersAt[team]) > membersRefresh && cl != nil {
		hc.membersAt[team] = time.Now()
		go func() {
			var full api.Team
			err := cl.CloudCall("GET", teamPath(api.PathTeam, team), nil, &full)
			a.onMain(func() {
				if err != nil {
					hc.membersErr[team] = client.Explain(err)
					return
				}
				hc.members[team], hc.membersErr[team] = full.Members, ""
			})
		}()
	}
	return hc.members[team]
}

// teamChatEvent takes messages that came or were sent.
func (a *App) teamChatEvent(data json.RawMessage) func() {
	var list []proto.ChatMessage
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return func() {
		hc, me := a.chatsOf(), a.cloudOf().st.AccountID
		for _, m := range list {
			have := hc.byTeam[m.Team]
			if i := slices.IndexFunc(have, func(x proto.ChatMessage) bool { return x.ID == m.ID }); i >= 0 {
				have[i] = m // edited
				continue
			}
			hc.byTeam[m.Team] = append(have, m)
			seen := a.panelShows(panelChat) && a.chatTeam() == m.Team && a.win != nil && a.win.IsFocused()
			if m.Account == me || seen {
				continue
			}
			a.notifyChatJudged(m, mentions(m, me) || a.repliesTo(m, me))
		}
	}
}

// teamChatGoneEvent takes messages that were deleted.
func (a *App) teamChatGoneEvent(data json.RawMessage) func() {
	var refs []proto.ChatRef
	if json.Unmarshal(data, &refs) != nil {
		return nil
	}
	return func() {
		hc := a.chatsOf()
		for _, r := range refs {
			hc.byTeam[r.Team] = slices.DeleteFunc(hc.byTeam[r.Team], func(m proto.ChatMessage) bool { return m.ID == r.ID })
			if hc.editing != nil && hc.editing.ID == r.ID {
				a.stopEditing()
			}
		}
	}
}

// mentions says m mentions account.
func mentions(m proto.ChatMessage, account string) bool {
	return account != "" && slices.ContainsFunc(m.Mentions, func(mn proto.ChatMention) bool {
		return mn.Kind == proto.MentionUser && mn.ID == account
	})
}

// chatUnread counts the messages of others since the user last read
// team's chat.
func (a *App) chatUnread(team string) int {
	if a.teamChats == nil || team == "" {
		return 0
	}
	read := a.state.ChatRead[team]
	n := 0
	for _, m := range a.teamChats.byTeam[team] {
		if m.ID > read && m.Account != a.cloudOf().st.AccountID {
			n++
		}
	}
	return n
}

// chatTab is the side panel's chat.
func (a *App) chatTab(c *ui.Context) {
	team := a.chatTeam()
	if team == "" {
		panelNote(c, "Sign in to enowX Cloud and join a team to chat with it.")
		return
	}
	hc := a.chatsOf()
	if !hc.wasShown || hc.listFor != team {
		// Opened, or another team's: at its latest message, as a fresh
		// list starts.
		hc.list, hc.listFor = ui.ListState{}, team
	}
	hc.shown = true
	msgs, read := hc.byTeam[team]
	if !read {
		hc.byTeam[team] = nil
		a.loadChat(team)
	}
	if time.Since(hc.pulled) > chatRefresh {
		hc.pulled = time.Now()
		cl := a.client
		go func() { _ = cl.RefreshChat(team) }() // what came shows by its event
	}
	c.After(chatRefresh)
	if n := len(msgs); n > 0 {
		if a.state.ChatRead == nil {
			a.state.ChatRead = map[string]string{}
		}
		a.state.ChatRead[team] = msgs[n-1].ID
	}
	a.teamHeader(c, team)
	if len(msgs) == 0 {
		ui.Column(c).Grow(1).FillWidth().Center().Gap(6).Padding(24).Children(func() {
			ui.Icon(c, icon.Chat).FontSize(22).TextColor(theme.Idle)
			ui.Text(c, "No message yet: say hello to "+a.spaceName(team)+".").FontSize(12).TextColor(theme.Muted)
			ui.Text(c, "Type @ to mention a member, a note or a file.").FontSize(11).TextColor(theme.Idle)
		})
	} else {
		a.chatMessages(c, team, msgs)
	}
	a.typingLine(c, team)
	a.chatComposer(c, team)
	a.imageViewer(c, team)
}

// teamHeader names the team the Team group's tools show, with how many
// members it has.
func (a *App) teamHeader(c *ui.Context, team string) {
	ui.Row(c).FillWidth().Height(36).Shrink(0).PaddingX(12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		var teams []widget.Space
		for _, sp := range a.spacesOf().list {
			teams = append(teams, widget.Space{Team: sp.Team, Name: sp.Name})
		}
		widget.SpacePicker(c, widget.Space{Team: team, Name: a.spaceName(team)}, "", 12.5, teams, func(t string) {
			a.state.Panel.Team = t
		})
		if n := len(a.teamMembers(team)); n > 0 {
			ui.Text(c, textfmt.Plural(n, "1 member", fmt.Sprintf("%d members", n))).FontSize(11).TextColor(theme.Muted).SingleLine()
		}
		if n := len(a.hereOf().byTeam[team].Online); n > 0 {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(theme.Done)
				ui.Text(c, fmt.Sprintf("%d online", n)).FontSize(11).TextColor(theme.Muted).SingleLine()
			})
		}
		ui.Spacer(c)
		tip := "Sealed with the team's key: the cloud keeps only what it can't read"
		ui.Icon(c, icon.Lock).FontSize(12).TextColor(theme.Idle).Tooltip(tip).Label(tip)
	})
}

// repliesTo says m answers a message of account's.
func (a *App) repliesTo(m proto.ChatMessage, account string) bool {
	if m.ReplyTo == "" || account == "" {
		return false
	}
	for _, x := range a.chatsOf().byTeam[m.Team] {
		if x.ID == m.ReplyTo {
			return x.Account == account
		}
	}
	return false
}

// chatUnreadAll counts the unread messages of every team, and the team
// with the latest.
func (a *App) chatUnreadAll() (int, string) {
	n, team, latest := 0, "", ""
	for _, sp := range a.spacesOf().list {
		if sp.Team == "" {
			continue
		}
		if k := a.chatUnread(sp.Team); k > 0 {
			n += k
			if list := a.chatsOf().byTeam[sp.Team]; len(list) > 0 && list[len(list)-1].ID > latest {
				team, latest = sp.Team, list[len(list)-1].ID
			}
		}
	}
	return n, team
}

// chatChip says, in the title bar, how many team messages are unread; a
// click opens the chat with the latest.
func (a *App) chatChip(c *ui.Context) {
	n, team := a.chatUnreadAll()
	if n == 0 || a.panelShows(panelChat) && a.chatTeam() == team {
		return
	}
	label := fmt.Sprintf("%d unread", n)
	b := titleChip(c, "chat-unread", label).Tooltip("Team chat: messages you haven't read")
	b.TextColor(theme.Accent).Children(func() {
		ui.Icon(c, icon.Chat).FontSize(11)
		ui.Text(c, label).FontSize(11.5).SingleLine()
	})
	if b.Clicked() {
		a.state.Panel.Team = team
		a.openPanel(panelChat)
	}
}
