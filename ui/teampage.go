package ui

import (
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The Team page: the account's teams beside the frame, one of them in it, its
// chat, projects, members, hosts, kit, log and options as tabs. It is the team
// dialog of Settings › Cloud at a page's size, reading and changing the same
// state: the open team of cloudTeams.

// pageTeam is the Team page.
const pageTeam = "team"

// teamPageTabs are the page's tabs, those of adminTabs for admins alone.
var teamPageTabs = []string{tabChat, tabProjects, tabMembers, tabHosts, tabKit, tabActivity, tabOptions}

// openTeamPage shows the Team page.
func (a *App) openTeamPage() {
	a.page = pageTeam
}

// leftTeamPage lets go of the team the page held open, so that Settings' dialog
// doesn't open on it.
func (a *App) leftTeamPage() {
	if cs := a.cloudSet; cs != nil && cs.teams.onPage {
		ts := &cs.teams
		ts.onPage, ts.inviting, ts.sharing, ts.deleting = false, false, false, false
		ts.open, ts.team, ts.invite = "", api.Team{}, api.Invite{}
	}
}

// teamPageChat is the team whose chat the page shows now, "" for none.
func (a *App) teamPageChat() string {
	if a.page != pageTeam || a.cloudSet == nil || a.cloudSet.host != a.host.ID {
		return ""
	}
	if ts := &a.cloudSet.teams; ts.onPage && ts.pageTab == tabChat {
		return ts.open
	}
	return ""
}

// teamPageTabsFor are the tabs a role sees.
func teamPageTabsFor(role string) []string {
	if canAdmin(role) {
		return teamPageTabs
	}
	return slices.DeleteFunc(slices.Clone(teamPageTabs), func(x string) bool { return x == tabActivity })
}

// teamSide lists the teams beside the frame, with making and joining one.
func (a *App) teamSide(c *ui.Context) {
	cs := a.cloudOf()
	if !cs.loaded || !cs.st.SignedIn {
		return
	}
	ts := &cs.teams
	widget.Glass(ui.Column(c.Key("team-side")).Width(navWidth).Shrink(0).FillHeight().Radius(theme.RadiusPanel).Clip()).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Shrink(0).MinHeight(32).Margin(theme.S2, 0, 0, 0).Padding(0, theme.S2, 0, theme.S4).Gap(theme.S2).Children(func() {
			ui.Text(c, "Teams").FontSize(theme.TextUI).FontWeight(600).SingleLine().Grow(1)
			if widget.IconButton(c, icon.Plus, "New team", false).Clicked() {
				ts.making, ts.newName = true, ""
			}
		})
		ui.Scroll(c.Key("list")).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(theme.S1, widget.ScrollGutter, theme.S2, theme.S2).Gap(theme.S1).Children(func() {
				for _, t := range ts.list {
					sub := t.Role
					if full, ok := cs.full[t.ID]; ok {
						n := len(full.Members)
						sub = textfmt.Plural(n, "1 member", fmt.Sprintf("%d members", n)) + " · " + t.Role
					}
					if widget.ListItem(c.Key(t.ID), ui.Color{}, t.Name, sub, t.ID == ts.open, nil).Clicked() && t.ID != ts.open {
						a.openCloudTeam(cs, t.ID)
					}
				}
			})
		})
		ui.Row(c.Key("foot")).FillWidth().Shrink(0).Padding(theme.S2).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				if widget.Button(c, widget.Secondary, nil, "Join with an invite", "").FillWidth().Clicked() {
					ts.joining, ts.join = true, ""
				}
			})
		})
	})
}

// teamPage fills the frame with the team picked.
func (a *App) teamPage(c *ui.Context) {
	cs := a.cloudOf()
	ts := &cs.teams
	ts.onPage = true
	switch {
	case !cs.loaded:
		widget.PageHeader(c, "Team", a.host.Name+"'s sign-in", nil)
		widget.PageBody(c, true, func() {
			cloudErrAndBusy(c, cs)
			ui.Text(c, "Asking "+a.host.Name+"…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		})
		return
	case !cs.st.SignedIn:
		widget.PageHeader(c, "Team", a.host.Name+"'s sign-in", nil)
		a.cloudSignedOut(c, cs)
		return
	case len(ts.list) == 0:
		widget.PageHeader(c, "Team", "No teams yet", nil)
		cloudErrAndBusy(c, cs)
		widget.EmptyState(c, icon.Users, "No teams yet", "Make one and invite the people you work with, or join one with the link someone sent you.", func() {
			if widget.PrimaryButton(c, icon.Plus, "New team", "").Clicked() {
				ts.making, ts.newName = true, ""
			}
		})
		a.cloudTeamDialogs(c, cs)
		return
	}
	a.keepTeamOpen(cs)
	t := ts.team
	if t.ID == "" {
		i := slices.IndexFunc(ts.list, func(x api.Team) bool { return x.ID == ts.open })
		t = ts.list[max(i, 0)]
	}
	tabs := teamPageTabsFor(t.Role)
	tab := ts.pageTab
	if !slices.Contains(tabs, tab) {
		tab = tabChat
		ts.pageTab = tab
	}
	widget.PageHeader(c, t.Name, a.teamPageSub(t), func() {
		if i := widget.Segmented(c, tabs, slices.Index(tabs, tab), nil); i >= 0 {
			ts.pageTab = tabs[i]
		}
	})
	body := c.Key("tab:" + t.ID + ":" + tab)
	ui.Column(body).Grow(1).FillWidth().MinHeight(0).Children(func() {
		switch tab {
		case tabChat:
			a.chatBody(body, t.ID)
		case tabProjects:
			a.teamPageProjects(body, cs, t)
		case tabMembers:
			a.teamPageMembers(body, cs, t)
		case tabHosts:
			a.teamPageHosts(body, cs, t)
		case tabKit:
			a.teamPageKit(body, cs, t)
		case tabActivity:
			a.teamPageActivity(body, cs)
		case tabOptions:
			a.teamPageOptions(body, cs, t)
		}
	})
	a.cloudTeamDialogs(c, cs)
	a.teamPageDialogs(c, cs, t)
}

// keepTeamOpen opens the team the page shows: the one it last showed while it's
// still among the account's, else the first.
func (a *App) keepTeamOpen(cs *cloudSettings) {
	ts := &cs.teams
	has := func(id string) bool {
		return slices.ContainsFunc(ts.list, func(t api.Team) bool { return t.ID == id })
	}
	if has(ts.open) {
		ts.pageTeam = ts.open
		return
	}
	id := ts.pageTeam
	if !has(id) {
		id = ts.list[0].ID
	}
	a.openCloudTeam(cs, id)
	ts.pageTeam = id
}

// teamPageSub says how many the team has and how many of them are here.
func (a *App) teamPageSub(t api.Team) string {
	n := len(a.teamMembers(t.ID))
	if n == 0 {
		n = len(t.Members)
	}
	if n == 0 {
		return ""
	}
	sub := textfmt.Plural(n, "1 member", fmt.Sprintf("%d members", n))
	if on := len(a.hereOf().byTeam[t.ID].Online); on > 0 {
		sub += fmt.Sprintf(", %d online", on)
	}
	return sub
}

// teamFigures are the figures over the page's cards.
func (a *App) teamFigures(cs *cloudSettings, t api.Team) []widget.Fig {
	projects := 0
	for _, p := range a.teamProjectsOf().list {
		if p.Team == t.ID {
			projects++
		}
	}
	hosts := len(a.membersHosts(t.ID))
	if slices.ContainsFunc(a.teamHostsOf().share.Teams, func(s proto.HostShareTeam) bool { return s.Team == t.ID && len(s.Projects) > 0 }) {
		hosts++
	}
	return []widget.Fig{
		{Label: "Members", Value: fmt.Sprint(len(t.Members))},
		{Label: "Online", Value: fmt.Sprint(len(a.hereOf().byTeam[t.ID].Online))},
		{Label: "Projects", Value: fmt.Sprint(projects)},
		{Label: "Shared hosts", Value: fmt.Sprint(hosts)},
	}
}

// teamPageActivity is the team's log as cards, for its admins.
func (a *App) teamPageActivity(c *ui.Context, cs *cloudSettings) {
	ta := a.teamAuditOf(cs)
	widget.PageBody(c, true, func() {
		if ta.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, ta.err, "", nil)
		}
		switch {
		case !ta.loaded:
			widget.CardNote(c, "Reading the team's log…", 0)
		case len(ta.entries) == 0:
			widget.CardNote(c, "Nothing yet.", 0)
		}
		now := time.Now()
		widget.LogGrid(c, len(ta.entries), func() {
			for _, e := range ta.entries {
				teamLogCard(c.Key(e.ID), e, now)
			}
		})
	})
}

// teamLogCard is an entry of the log, its time with the full date in a tip.
func teamLogCard(c *ui.Context, e api.AuditEntry, now time.Time) {
	ui.Column(c).MinWidth(0).MinHeight(0).Clip().Padding(theme.S3, theme.S4).Gap(2).Radius(theme.RadiusCard).Background(theme.Raised).Justify(ui.Center).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Shrink(0).MinHeight(20).Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, auditLine(e), widget.RunStyle{Weight: 500}) })
			ui.Text(c, e.At.Local().Format("15:04")).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0).
				Tooltip(e.At.Local().Format("2 Jan 2006 15:04"))
		})
		widget.Running(c.Key("day"), ragDay(e.At, now), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
	})
}

// teamPageOptions are the team's rules, for its admins, and leaving it or, for
// its owner, deleting it.
func (a *App) teamPageOptions(c *ui.Context, cs *cloudSettings, t api.Team) {
	ts := &cs.teams
	admin := canAdmin(t.Role)
	cols, n := 1, 1
	if admin {
		cols, n = 2, 2
	}
	widget.PageBodyGrid(c, cols, n, 220, func() { cloudErrAndBusy(c, cs) }, func() {
		if admin {
			widget.GridCard(c.Key("rules")).Children(func() {
				widget.CardTitle(c, nil, "Rules", nil)
				out := ts.team.CopyOut
				if widget.ToggleRow(c.Key("copy-out"), "Copying out", "Members may copy the team's notes and canvases to their own space", &out) {
					team := ts.team.ID
					a.setTeamCopyOut(cs, team, out)
				}
			})
		}
		widget.GridCard(c.Key("leaving")).Children(func() {
			if t.Role == api.RoleOwner {
				widget.CardTitle(c, nil, "Deleting", nil)
				widget.CardNote(c, "Deleting the team takes its space from every member, from the cloud and from their hosts. It can't be undone.", 0)
				ui.Spacer(c)
				widget.CardFoot(c, func() {
					if widget.DangerButton(c, "Delete the team…").Clicked() {
						ts.deleting, ts.deleteName = true, ""
					}
				})
				return
			}
			widget.CardTitle(c, nil, "Leaving", nil)
			widget.CardNote(c, "Your own notes stay. Its notes and canvases leave this host's Notes and Canvas, and you need a new invite to come back.", 0)
			ui.Spacer(c)
			widget.CardFoot(c, func() {
				if m, ok := cs.member(); ok && widget.DangerButton(c, "Leave the team").Clicked() {
					a.confirmLeaveTeam(cs, t.Name, m.Account.ID)
				}
			})
		})
	}, nil)
}

// teamPageDialogs are what the page opens in a dialog: an invite, and sharing
// one of the host's agents' items.
func (a *App) teamPageDialogs(c *ui.Context, cs *cloudSettings, t api.Team) {
	ts := &cs.teams
	if ts.inviting {
		widget.Sheet(c, &ts.inviting, 480, func() {
			widget.SheetHead(c, &ts.inviting, "Invite to "+t.Name, "The link works once.")
		}, func() {
			cloudErrAndBusy(c, cs)
			a.cloudInvite(c, cs)
		}, func() {
			ui.Spacer(c)
			if widget.PrimaryButton(c, nil, "Done", "").Clicked() {
				ts.inviting, ts.invite = false, api.Invite{}
			}
		})
	}
	if ts.sharing {
		widget.Sheet(c, &ts.sharing, 560, func() {
			widget.SheetHead(c, &ts.sharing, "Share one of yours", a.host.Name+"'s agents' skills, commands and subagents")
		}, func() {
			a.kitShareList(c, t)
		}, func() {
			ui.Spacer(c)
			if widget.PrimaryButton(c, nil, "Done", "").Clicked() {
				ts.sharing = false
			}
		})
	}
}
