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
		ui.Column(c).Grow(1).FillWidth().Center().Gap(6).Padding(24).Children(func() {
			ui.Icon(c, icon.Folder).FontSize(22).TextColor(theme.Idle)
			ui.Text(c, "No projects yet").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			ui.Text(c, "Admins add them by their Git link, in Settings › Cloud › the team › Projects.").FontSize(theme.TextLabel).TextColor(theme.Muted).TextAlign(ui.Center)
		})
		return
	}
	ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Gap(8).Padding(10, 12).Children(func() {
			for _, p := range list {
				a.teamProjectCard(c.Key(p.ID), p)
			}
		})
	})
}

// teamProjectCard is a team's project: its link, its copy on this host
// or a way to clone it, and the sessions working in it.
func (a *App) teamProjectCard(c *ui.Context, p proto.TeamProject) {
	var sessions []proto.SessionInfo
	for _, s := range a.sessions {
		if !s.Exited && slices.ContainsFunc(p.Here, func(h proto.TeamProjectCopy) bool { return h.Path == s.Project }) {
			sessions = append(sessions, s)
		}
	}
	ui.Column(c).FillWidth().Radius(theme.RadiusCard).Background(theme.Raised).Border(1, theme.Border).Children(func() {
		ui.Row(c).FillWidth().Gap(10).Padding(10, 12).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(28, 28).Radius(theme.RadiusSmall).Background(theme.Sunken).Center().Shrink(0).Children(func() {
				ui.Icon(c, icon.Folder).FontSize(14).TextColor(theme.TextSoft)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, p.Name).FontSize(theme.TextUI).FontWeight(600).SingleLine()
				ui.Text(c, strings.TrimPrefix(p.Ref, "git:")).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			})
			if len(p.Here) == 0 {
				if widget.Button(c, widget.Secondary, icon.Download, "Clone", "").Tooltip("Clone " + p.Repo + " on " + a.host.Name).Clicked() {
					a.cloneTeamProject(p)
				}
				return
			}
			if proj, ok := a.projectAt(p.Here[0].Path); ok {
				if widget.IconButton(c, icon.Plus, "New session in "+p.Name, false).Clicked() {
					a.openNewSession(proj)
				}
			}
		})
		ui.Column(c).FillWidth().Gap(6).Padding(0, 12, 10, 12).Children(func() {
			ui.Row(c).FillWidth().Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
				if len(p.Here) > 0 {
					widget.Tag(c, "on "+a.host.Name, theme.Done)
					ui.Text(c, textfmt.TildePath(p.Here[0].Path, a.daemonInfo.Home)).FontSize(theme.TextLabel).Font(theme.MonoFont).TextColor(theme.Muted).SingleLine().Shrink(1)
				} else {
					widget.Tag(c, "not on "+a.host.Name, theme.Muted)
				}
				if n := a.teamProjectNotes(p); n > 0 {
					widget.Tag(c, textfmt.Plural(n, "1 note", fmt.Sprintf("%d notes", n)), theme.TextSoft)
				}
				if p.AddedBy != "" {
					ui.Text(c, "added by "+p.AddedBy).FontSize(theme.TextLabel).TextColor(theme.Idle).SingleLine()
				}
			})
			for _, s := range sessions {
				row := ui.ButtonBase(c.Key("s:"+s.ID)).FillWidth().Gap(8).Padding(5, 8).Radius(theme.RadiusControl).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Label("Show " + a.sessionName(s))
				if row.Hovered() || s.ID == a.selected {
					row.Background(theme.Hover)
				}
				row.Children(func() {
					ui.Icon(c, icon.Terminal).FontSize(12).TextColor(theme.TextSoft)
					ui.Text(c, a.sessionName(s)).FontSize(theme.TextLabel).SingleLine().Shrink(1).Grow(1)
					if s.Member != nil {
						memberAvatar(c, s.Member.Account, nonEmptyStr(s.Member.Name, s.Member.GitHub), 16).Tooltip("Started by " + nonEmptyStr(s.Member.Name, "@"+s.Member.GitHub))
					}
				})
				if row.Clicked() {
					a.selectSession(s)
				}
			}
		})
	})
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
		ui.Column(c).Grow(1).FillWidth().Center().Gap(8).Padding(24).Children(func() {
			ui.Text(c, "Couldn't read the team's members: "+err).FontSize(theme.TextLabel).TextColor(theme.Muted).TextAlign(ui.Center)
			if widget.Button(c, widget.Secondary, nil, "Try again", "").Clicked() {
				delete(hc.membersAt, team)
				hc.membersErr[team] = ""
			}
		})
		return
	}
	if len(members) == 0 {
		ui.Column(c).Grow(1).FillWidth().Center().Gap(6).Padding(24).Children(func() {
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
		ui.Column(c).FillWidth().Gap(2).Padding(8, 8).Children(func() {
			for _, m := range members {
				acc := m.Account
				name := memberTitle(acc.Name, acc.GitHub)
				vi := slices.IndexFunc(visitors, func(v proto.Visitor) bool { return v.Account == acc.ID && v.Team == team })
				ui.Row(c.Key(acc.ID)).FillWidth().Gap(10).Padding(7, 8).Radius(theme.RadiusControl).AlignItems(ui.Center).Children(func() {
					online := a.online(team, acc.ID)
					av := memberAvatar(c, acc.ID, name, 28)
					if online {
						av.Children(func() { onlineDot(c, 28) })
					}
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, name).FontSize(theme.TextUI).FontWeight(600).SingleLine().Shrink(1)
							if acc.ID == me {
								ui.Text(c, "you").FontSize(theme.TextLabel).TextColor(theme.Muted)
							}
						})
						sub := m.Role
						if acc.GitHub != "" {
							sub = "@" + acc.GitHub + " · " + m.Role
						}
						if vi >= 0 {
							switch n := len(visitors[vi].Watching); {
							case n > 0:
								sub += " · watching " + textfmt.Plural(n, "a session", fmt.Sprintf("%d sessions", n))
							default:
								sub += " · on " + a.host.Name
							}
						}
						ui.Text(c, sub).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
					})
					if vi >= 0 {
						ui.Box(c).Size(8, 8).Radius(theme.RadiusTail).Background(theme.Done).Shrink(0).Tooltip("On " + a.host.Name + " now")
					} else if online {
						ui.Text(c, "online").FontSize(theme.TextLabel).TextColor(theme.Done).Shrink(0)
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
	a.openPanel(panelChat)
}
