package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The Team page's tabs of cards: a team's projects, members, hosts and kit.

// teamPageProjects is the Projects tab.
func (a *App) teamPageProjects(c *ui.Context, cs *cloudSettings, t api.Team) {
	tp := a.teamProjectsOf()
	admin := canAdmin(t.Role)
	var list []proto.TeamProject
	for _, p := range tp.list {
		if p.Team == t.ID {
			list = append(list, p)
		}
	}
	n := len(list)
	if admin || n == 0 {
		n++
	}
	widget.FigsBand(c, a.teamFigures(cs, t))
	widget.PageBodyGrid(c, 3, n, 220, func() {
		if tp.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, tp.err, "", nil)
		}
	}, func() {
		for _, p := range list {
			a.teamProjectPageCard(c.Key(p.ID), p, admin)
		}
		switch {
		case admin:
			a.addTeamProjectCard(c.Key("add"), t, len(list) == 0)
		case len(list) == 0:
			widget.GridCard(c.Key("none")).Children(func() {
				widget.EmptyState(c, icon.Folder, "No projects yet", "An admin adds one by its Git link.", nil)
			})
		}
	}, nil)
}

// teamProjectPageCard is a project of the team: its link, where it is here, and
// what to do about it.
func (a *App) teamProjectPageCard(c *ui.Context, p proto.TeamProject, admin bool) {
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, p.Name, nil)
		widget.Running(c.Key("link"), strings.TrimPrefix(p.Ref, "git:"), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		ui.Row(c.Key("chips")).FillWidth().Shrink(0).Wrap().Gap(theme.S1).Children(func() {
			if len(p.Here) > 0 {
				widget.Chip(c, "On "+a.host.Name, ui.Color{})
			} else {
				widget.Chip(c, "Not on "+a.host.Name, ui.Color{})
			}
			if n := a.teamProjectNotes(p); n > 0 {
				widget.Chip(c, textfmt.Plural(n, "1 note", fmt.Sprintf("%d notes", n)), ui.Color{})
			}
		})
		ui.Scroll(c.Key("sessions")).Grow(1).Basis(0).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				if len(p.Here) > 0 {
					widget.Running(c.Key("path"), textfmt.TildePath(p.Here[0].Path, a.daemonInfo.Home), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				}
				a.teamProjectSessions(c, p)
			})
		})
		widget.CardFoot(c, func() {
			if len(p.Here) == 0 {
				if widget.Button(c, widget.Secondary, nil, "Clone", "").Tooltip("Clone " + p.Repo + " on " + a.host.Name).Clicked() {
					a.cloneTeamProject(p)
				}
			} else if proj, ok := a.projectAt(p.Here[0].Path); ok {
				if widget.Button(c, widget.Secondary, nil, "New session", "").Tooltip("New session in " + p.Name).Clicked() {
					a.openNewSession(proj)
				}
			}
			ui.Spacer(c)
			if admin && widget.IconButton(c, icon.Trash, "Take "+p.Name+" out of the team", false).Clicked() {
				a.confirmTakeProjectOut(p)
			}
		})
	})
}

// addTeamProjectCard adds a project by its Git link, for admins.
func (a *App) addTeamProjectCard(c *ui.Context, t api.Team, first bool) {
	tp := a.teamProjectsOf()
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, "Add a project", nil)
		if first {
			widget.CardNote(c, "No projects yet: add one by its Git link. Members who have it already are found by its remote; the others clone it from here.", 0)
		}
		widget.Field(c, "Git link", func() {
			widget.Input(c, &tp.adding, "github.com/enowx/enowx-web").FillWidth().Font(theme.MonoFont)
		})
		widget.CardNote(c, "The link only, no token in it: each member clones with their own login.", 0)
		ui.Spacer(c)
		widget.CardFoot(c, func() {
			b := widget.Button(c, widget.Secondary, icon.Plus, "Add", "")
			link := strings.TrimSpace(tp.adding)
			if link == "" {
				b.Disabled(true)
			} else if b.Clicked() {
				a.addTeamProject(t.ID, link)
			}
		})
	})
}

// teamPageMembers is the Members tab, with the team's latest for those who may
// read its log.
func (a *App) teamPageMembers(c *ui.Context, cs *cloudSettings, t api.Team) {
	admin := canAdmin(t.Role)
	widget.FigsBand(c, a.teamFigures(cs, t))
	members := slices.Clone(t.Members)
	slices.SortStableFunc(members, func(x, y api.Member) int {
		return boolOrder(a.online(t.ID, y.Account.ID)) - boolOrder(a.online(t.ID, x.Account.ID))
	})
	widget.PageBodyGrid(c, 3, len(members), 160, func() {
		cloudErrAndBusy(c, cs)
		if t.Role == api.RoleViewer {
			widget.Banner(c, theme.Done, "You are a viewer here: you read its notes and canvases and watch its sessions.", nil)
		}
		ui.Row(c.Key("tools")).FillWidth().Shrink(0).MinHeight(theme.ControlHeight).Gap(theme.S3).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.CardNote(c, roleAbout(api.RoleAdmin), 0)
			})
			if admin && widget.PrimaryButton(c, icon.Plus, "Invite", "").Clicked() {
				cs.teams.inviting, cs.teams.invite = true, api.Invite{}
			}
		})
	}, func() {
		for _, m := range members {
			a.teamMemberCard(c.Key(m.Account.ID), cs, t, m)
		}
	}, func() {
		if !admin {
			return
		}
		ta := a.teamAuditOf(cs)
		if len(ta.entries) == 0 {
			return
		}
		widget.Heading(c, "Latest in the team")
		ui.Column(c.Key("latest")).FillWidth().Shrink(0).Children(func() {
			now := time.Now()
			for _, e := range ta.entries[:min(len(ta.entries), cloudLatestShown+2)] {
				teamAuditRow(c.Key(e.ID), e, now)
			}
		})
	})
}

// teamMemberCard is a member: who, their role, changed here by an admin, and
// whether they're here.
func (a *App) teamMemberCard(c *ui.Context, cs *cloudSettings, t api.Team, m api.Member) {
	acc := m.Account
	me := acc.ID == cs.st.AccountID
	name := memberTitle(acc.Name, acc.GitHub)
	widget.GridCard(c).Children(func() {
		ui.Row(c.Key("who")).FillWidth().Shrink(0).MinHeight(theme.ControlHeight).Gap(theme.S2).Children(func() {
			memberAvatar(c, acc.ID, name, 24)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, name, widget.RunStyle{Size: theme.TextRead, Weight: 600}) })
			if !me && widget.IconButton(c, icon.AtSign, "Mention "+name+" in the chat", false).Clicked() {
				a.mentionInChat(t.ID, proto.ChatMention{Kind: proto.MentionUser, ID: acc.ID, Title: name})
			}
		})
		ui.Row(c.Key("role")).FillWidth().Shrink(0).MinHeight(theme.ControlHeight).Gap(theme.S2).Children(func() {
			github := ""
			if acc.GitHub != "" {
				github = "@" + acc.GitHub
			}
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Running(c, github, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if me || m.Role == api.RoleOwner || !canAdmin(t.Role) {
				widget.Chip(c, roleName(m.Role), theme.TextStrong)
				return
			}
			a.memberRoleMenu(c, cs, m)
			a.memberMoreMenu(c, cs, m)
		})
		on, what := a.memberPresence(t.ID, m, me)
		col := theme.Idle
		if on {
			col = theme.Done
		}
		widget.StatusLine(c.Key("here"), col, false, what)
	})
}

// memberPresence says whether a member is here, and how it reads.
func (a *App) memberPresence(team string, m api.Member, me bool) (bool, string) {
	if a.presence != nil {
		i := slices.IndexFunc(a.presence.visitors, func(v proto.Visitor) bool { return v.Account == m.Account.ID && v.Team == team })
		if i >= 0 {
			if n := len(a.presence.visitors[i].Watching); n > 0 {
				return true, "Online · watching " + textfmt.Plural(n, "a session", fmt.Sprintf("%d sessions", n))
			}
			return true, "Online · on " + a.host.Name
		}
	}
	switch {
	case me:
		return true, "Online · " + a.host.Name
	case a.online(team, m.Account.ID):
		return true, "Online"
	}
	return false, "Away"
}

// teamPageHosts is the Hosts tab: what the connected host shares, a card, and
// each host members share, a card.
func (a *App) teamPageHosts(c *ui.Context, cs *cloudSettings, t api.Team) {
	th := a.teamHostsOf()
	list := a.membersHosts(t.ID)
	n := 1 + max(len(list), 1)
	widget.FigsBand(c, a.teamFigures(cs, t))
	widget.PageBodyGrid(c, 2, n, 260, func() {
		if th.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, th.err, "", nil)
		}
	}, func() {
		widget.GridCard(c.Key("own")).Children(func() {
			widget.CardTitle(c, nil, a.host.Name, nil)
			ui.Scroll(c.Key("share")).Grow(1).Basis(0).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(theme.S2).Children(func() { a.hostShareBody(c, t) })
			})
		})
		if len(list) == 0 {
			widget.GridCard(c.Key("none")).Children(func() {
				widget.CardTitle(c, nil, "Shared by members", nil)
				widget.CardNote(c, "No member shares a host yet.", 0)
			})
		}
		for _, h := range list {
			a.teamHostCard(c.Key(h.Device), cs, h)
		}
	}, nil)
}

// teamHostCard is a host a member shares: its projects, and opening it.
func (a *App) teamHostCard(c *ui.Context, cs *cloudSettings, h proto.TeamHost) {
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, h.Name, nil)
		widget.CardNote(c, teamHostAbout(h), 1)
		ui.Scroll(c.Key("projects")).Grow(1).Basis(0).FillWidth().MinHeight(0).Children(func() {
			widget.CardLines(c, func() {
				for i, p := range h.Projects {
					widget.CardLine(c.Key(p.Path), i == 0, p.Name, "", nil)
				}
			})
		})
		widget.CardFoot(c, func() {
			if widget.Button(c, widget.Secondary, nil, "Open", "").Tooltip("Open " + h.Name).Clicked() {
				cs.teams.open = ""
				a.openTeamHost(h)
			}
		})
	})
}

// teamPageKit is the Kit tab: what the team shares, filtered, and sharing more.
func (a *App) teamPageKit(c *ui.Context, cs *cloudSettings, t api.Team) {
	tk := a.teamKitOf()
	can := t.Role != api.RoleViewer
	q := strings.ToLower(strings.TrimSpace(tk.pageFilter))
	var items []proto.TeamKitItem
	total := 0
	for _, it := range tk.list {
		if it.Team != t.ID {
			continue
		}
		total++
		if q == "" || strings.Contains(strings.ToLower(it.Name+" "+it.Kind+" "+it.About), q) {
			items = append(items, it)
		}
	}
	n := len(items)
	if can {
		n++ // adding a template
	}
	widget.PageBodyGrid(c, 3, max(n, 1), 180, func() {
		if tk.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, tk.err, "", nil)
		}
		widget.CardNote(c, "Skills, commands and subagents the team shares: every session in its projects is told of them, whatever its agent.", 0)
		ui.Row(c.Key("tools")).FillWidth().Shrink(0).Gap(theme.S3).Children(func() {
			ui.Column(c).Width(320).Shrink(1).MinWidth(0).Children(func() {
				widget.SearchField(c, &tk.pageFilter, "Filter skills, commands, subagents", icon.Search)
			})
			ui.Spacer(c)
			if can && widget.Button(c, widget.Secondary, nil, "Share one of yours", "").Clicked() {
				cs.teams.sharing = true
				a.readKitItems()
			}
		})
	}, func() {
		for _, it := range items {
			a.teamKitCard(c.Key(it.Kind+":"+it.Name), cs, it, can)
		}
		switch {
		case can:
			a.addTemplateCard(c.Key("add-template"), t)
		case len(items) == 0:
			widget.GridCard(c.Key("none")).Children(func() {
				text := "Nothing shared yet."
				if total > 0 {
					text = "Nothing matches “" + tk.pageFilter + "”."
				}
				widget.CardNote(c, text, 0)
			})
		}
	}, nil)
}

// teamKitCard is an item of the team's kit, and what's done with one of its kind.
func (a *App) teamKitCard(c *ui.Context, cs *cloudSettings, it proto.TeamKitItem, can bool) {
	tk := a.teamKitOf()
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, it.Name, nil)
		sub := roleName(it.Kind)
		if about := a.kitDefinitionAbout(it); about != "" {
			sub += " · " + about
		}
		widget.CardNote(c, sub, 2)
		ui.Spacer(c)
		widget.CardFoot(c, func() {
			switch it.Kind {
			case proto.KitAutomation:
				if widget.Button(c, widget.Secondary, nil, "Set up", "").Tooltip("Set it up on " + a.host.Name).Clicked() {
					a.useKitAutomation(it, tk)
				}
			case proto.KitTemplate:
				if widget.Button(c, widget.Secondary, nil, "New project", "").Clicked() {
					a.cloneKitTemplate(cs, it)
				}
			}
			if can && widget.Button(c, widget.Secondary, nil, "Take it out", "").Tooltip("Take "+it.Name+" out of the kit").Clicked() {
				a.confirmTakeKitOut(it)
			}
		})
	})
}

// addTemplateCard adds a project template to the kit.
func (a *App) addTemplateCard(c *ui.Context, t api.Team) {
	tk := a.teamKitOf()
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, "Add a template", nil)
		ui.Scroll(c.Key("fields")).Grow(1).Basis(0).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
				widget.Input(c, &tk.templateName, "Template name").FillWidth()
				widget.Input(c.Key("repo"), &tk.templateRepo, "Its Git link").FillWidth().Font(theme.MonoFont)
			})
		})
		widget.CardFoot(c, func() {
			a.addTemplateButton(c, t.ID)
		})
	})
}
