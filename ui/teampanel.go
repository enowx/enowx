package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The side panel's Team group beside the chat: the team's projects, where
// each is on the connected host and the sessions working in it; and its
// members, who is on this host and what they watch.

const (
	panelTeamProjects = "team-projects"
	panelTeamMembers  = "team-members"
)

// teamProjectsTab lists the team's projects.
func (a *App) teamProjectsTab(c *ui.Context) {
	team := a.chatTeam()
	if team == "" {
		panelNote(c, "Sign in to enowX Cloud and join a team to see its projects.")
		return
	}
	a.teamHeader(c, team)
	var list []proto.TeamProject
	for _, p := range a.teamProjectsOf().list {
		if p.Team == team {
			list = append(list, p)
		}
	}
	if len(list) == 0 {
		ui.Column(c).Grow(1).FillWidth().Center().Gap(theme.S2).Padding(theme.S5).Children(func() {
			ui.Icon(c, icon.Folder).FontSize(22).TextColor(theme.Idle)
			ui.Text(c, "No projects yet").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			ui.Text(c, "Admins add them by their Git link, in Settings › Cloud › the team › Projects.").FontSize(theme.TextLabel).TextColor(theme.Muted).TextAlign(ui.Center)
		})
		return
	}
	ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.S1).Padding(theme.S2).Children(func() {
			for _, p := range list {
				a.teamProjectCard(c.Key(p.ID), p)
			}
		})
	})
}

// teamProjectCard is a team's project in the panel: where it is here and its
// notes, a way to start or clone there, and the sessions working in it.
func (a *App) teamProjectCard(c *ui.Context, p proto.TeamProject) {
	ui.Column(c).FillWidth().Shrink(0).Children(func() {
		ui.Row(c.Key("row")).FillWidth().Shrink(0).MinHeight(36).Padding(theme.S1, 0, theme.S1, theme.S3).Gap(theme.S2).Children(func() {
			parts := []string{"Not on " + a.host.Name}
			if len(p.Here) > 0 {
				parts = []string{textfmt.TildePath(p.Here[0].Path, a.daemonInfo.Home)}
				if n := len(a.sessionsOfTeamProject(p)); n > 0 {
					parts = append(parts, textfmt.Plural(n, "1 session", fmt.Sprintf("%d sessions", n)))
				}
			}
			if n := a.teamProjectNotes(p); n > 0 {
				parts = append(parts, textfmt.Plural(n, "1 note", fmt.Sprintf("%d notes", n)))
			}
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Running(c, p.Name, widget.RunStyle{})
				widget.Running(c.Key("sub"), strings.Join(parts, " · "), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if len(p.Here) == 0 {
				if widget.Button(c, widget.Secondary, nil, "Clone", "").Tooltip("Clone " + p.Repo + " on " + a.host.Name).Clicked() {
					a.cloneTeamProject(p)
				}
				return
			}
			if proj, ok := a.projectAt(p.Here[0].Path); ok {
				if widget.IconButton(c, icon.Plus, "New session in "+p.Name, false).Clicked() {
					a.openNewSession(proj)
				}
			}
			widget.StatusLine(c, theme.Done, false, "Here")
		})
		a.teamProjectSessions(c, p)
	})
}

// sessionsOfTeamProject are the host's sessions working in p.
func (a *App) sessionsOfTeamProject(p proto.TeamProject) []proto.SessionInfo {
	var sessions []proto.SessionInfo
	for _, s := range a.sessions {
		if !s.Exited && slices.ContainsFunc(p.Here, func(h proto.TeamProjectCopy) bool { return h.Path == s.Project }) {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

// teamProjectSessions are the sessions working in p, each opening; one a member
// started shows who.
func (a *App) teamProjectSessions(c *ui.Context, p proto.TeamProject) {
	for _, s := range a.sessionsOfTeamProject(p) {
		row := ui.ButtonBase(c.Key("s:" + s.ID)).FillWidth().Shrink(0).MinHeight(theme.ControlHeight).Gap(theme.S2).PaddingX(theme.S3).Radius(theme.RadiusFull).Justify(ui.Start).
			Cursor(ui.CursorPointer).Label("Show " + a.sessionName(s))
		if row.Hovered() || s.ID == a.selected {
			row.Background(theme.Hover)
		}
		row.Children(func() {
			ui.Icon(c, icon.Terminal).FontSize(12).TextColor(theme.TextSoft)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, a.sessionName(s), widget.RunStyle{Size: theme.TextLabel}) })
			if s.Member != nil {
				memberAvatar(c, s.Member.Account, nonEmptyStr(s.Member.Name, s.Member.GitHub), 16).Tooltip("Started by " + nonEmptyStr(s.Member.Name, "@"+s.Member.GitHub))
			}
		})
		if row.Clicked() {
			a.page = ""
			a.selectSession(s)
		}
	}
}

func nonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// projectAt is the host's project at path.
func (a *App) projectAt(path string) (proto.ProjectInfo, bool) {
	i := slices.IndexFunc(a.projects, func(p proto.ProjectInfo) bool { return p.Path == path })
	if i < 0 {
		return proto.ProjectInfo{}, false
	}
	return a.projects[i], true
}

// teamMembersTab lists the team's members, those on this host first.
func (a *App) teamMembersTab(c *ui.Context) {
	team := a.chatTeam()
	if team == "" {
		panelNote(c, "Sign in to enowX Cloud and join a team to see its members.")
		return
	}
	a.teamHeader(c, team)
	members := a.teamMembers(team)
	c.After(membersRefresh)
	if err := a.chatsOf().membersErr[team]; err != "" && len(members) == 0 {
		hc := a.chatsOf()
		ui.Column(c).Grow(1).FillWidth().Center().Gap(theme.S2).Padding(theme.S5).Children(func() {
			ui.Text(c, "Couldn't read the team's members: "+err).FontSize(theme.TextLabel).TextColor(theme.Muted).TextAlign(ui.Center)
			if widget.Button(c, widget.Secondary, nil, "Try again", "").Clicked() {
				delete(hc.membersAt, team)
				hc.membersErr[team] = ""
			}
		})
		return
	}
	if len(members) == 0 {
		ui.Column(c).Grow(1).FillWidth().Center().Gap(theme.S2).Padding(theme.S5).Children(func() {
			widget.Spinner(c, 14)
			ui.Text(c, "Reading the team's members…").FontSize(theme.TextLabel).TextColor(theme.Muted)
		})
		return
	}
	var visitors []proto.Visitor
	if a.presence != nil {
		visitors = a.presence.visitors
	}
	me := a.cloudOf().st.AccountID
	// Those here now first, each group as the cloud lists them.
	members = slices.Clone(members)
	slices.SortStableFunc(members, func(x, y api.Member) int {
		return boolOrder(a.online(team, y.Account.ID)) - boolOrder(a.online(team, x.Account.ID))
	})
	ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.S1).Padding(theme.S2).Children(func() {
			for _, m := range members {
				acc := m.Account
				name := memberTitle(acc.Name, acc.GitHub)
				vi := slices.IndexFunc(visitors, func(v proto.Visitor) bool { return v.Account == acc.ID && v.Team == team })
				ui.Row(c.Key(acc.ID)).FillWidth().Shrink(0).MinHeight(36).Gap(theme.S2).Padding(theme.S1, 0, theme.S1, theme.S2).Children(func() {
					online := a.online(team, acc.ID)
					av := memberAvatar(c, acc.ID, name, 24)
					if online {
						av.Children(func() { onlineDot(c, 24) })
					}
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
						title := name
						if acc.ID == me {
							title += " (you)"
						}
						widget.Running(c, title, widget.RunStyle{})
						sub := roleName(m.Role)
						if acc.GitHub != "" {
							sub = "@" + acc.GitHub + " · " + roleName(m.Role)
						}
						if vi >= 0 {
							switch n := len(visitors[vi].Watching); {
							case n > 0:
								sub += " · watching " + textfmt.Plural(n, "a session", fmt.Sprintf("%d sessions", n))
							default:
								sub += " · on " + a.host.Name
							}
						}
						widget.Running(c.Key("sub"), sub, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
					})
					if vi >= 0 {
						ui.Row(c.Key("here")).Shrink(0).Tooltip("On " + a.host.Name + " now").Children(func() {
							widget.StatusLine(c, theme.Done, false, "Here")
						})
					} else if online {
						widget.StatusLine(c.Key("on"), theme.Done, false, "Online")
					}
					if acc.ID != me && widget.IconButton(c, icon.AtSign, "Mention "+name+" in the chat", false).Clicked() {
						a.mentionInChat(team, proto.ChatMention{Kind: proto.MentionUser, ID: acc.ID, Title: name})
					}
				})
			}
		})
	})
}

// mentionInChat opens the chat with mn written in its draft.
func (a *App) mentionInChat(team string, mn proto.ChatMention) {
	hc := a.chatsOf()
	if hc.draft != "" && !strings.HasSuffix(hc.draft, " ") {
		hc.draft += " "
	}
	hc.draft += "@" + mn.Title + " "
	hc.mentions = append(hc.mentions, mn)
	hc.setCaret, hc.focus = len([]rune(hc.draft)), true
	if a.page == pageTeam {
		a.cloudOf().teams.pageTab = tabChat
		return
	}
	a.openPanel(panelChat)
}
