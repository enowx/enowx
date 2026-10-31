package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Settings › Cloud's teams: the account's as cards, each opening a dialog
// of its members and invites; a new team, and joining one with an invite,
// each a dialog of its own.

type cloudTeams struct {
	list    []api.Team
	open    string   // the team whose dialog shows
	team    api.Team // that team, with its members
	tab     string   // the dialog's tab, of teamTabs
	making  bool     // the new team's dialog shows
	newName string
	joining bool // the invite's dialog shows
	join    string
	role    int // the invite's role, of inviteRoles
	invite  api.Invite
	copied  widget.CopyFeedback
	audit   *teamAudit // the Activity tab's log (teamaudit.go)
	// deleting shows the open team's delete dialog, deleteName the name
	// typed in it (teamdelete.go).
	deleting   bool
	deleteName string
}

var (
	inviteRoles = []string{api.RoleMember, api.RoleViewer, api.RoleAdmin}
	teamTabs    = []string{tabMembers, tabProjects, tabHosts, tabKit, tabInvite, tabOptions, tabActivity}
	adminTabs   = []string{tabInvite, tabOptions, tabActivity} // inviting, the rules and the log are the admins'
)

// The team dialog's tabs.
const (
	tabMembers  = "Members"
	tabProjects = "Projects"
	tabHosts    = "Hosts"
	tabKit      = "Kit"
	tabInvite   = "Invite"
	tabOptions  = "Options"
	tabActivity = "Activity"
)

func teamPath(p, team string) string { return strings.Replace(p, "{team}", team, 1) }

func canAdmin(role string) bool { return role == api.RoleOwner || role == api.RoleAdmin }

// cloudTeamsTab is the Teams tab: the account's teams, and making or
// joining one.
func (a *App) cloudTeamsTab(c *ui.Context, cs *cloudSettings) {
	ts := &cs.teams
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, "Teams").FontSize(13.5).FontWeight(600)
			ui.Text(c, "Each has its own space of notes and canvases, picked in the rail.").FontSize(11.5).TextColor(theme.Muted)
		})
		if widget.Button(c, widget.Secondary, icon.Link, "Join with an invite", "").Clicked() {
			ts.joining, ts.join = true, ""
		}
		if widget.PrimaryButton(c, icon.Plus, "New team", "").Clicked() {
			ts.making, ts.newName = true, ""
		}
	})
	if len(ts.list) == 0 {
		ui.Column(c).FillWidth().Height(200).Radius(theme.RadiusCard).Border(1, theme.Border).Children(func() {
			widget.EmptyState(c, icon.Account, "No teams yet", "Make one and invite the people you work with, or join one with the link someone sent you.", nil)
		})
		return
	}
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		for _, t := range ts.list {
			a.cloudTeamCard(c, cs, t)
		}
	})
}

// cloudTeamCard is one team; a click opens its dialog.
func (a *App) cloudTeamCard(c *ui.Context, cs *cloudSettings, t api.Team) {
	card := ui.ButtonBase(c.Key("team:"+t.ID)).FillWidth().Padding(12, 14).Gap(12).AlignItems(ui.Center).Justify(ui.Start).
		Radius(theme.RadiusCard).Background(theme.Raised).Border(1, theme.Border).Cursor(ui.CursorPointer).Label("Team " + t.Name)
	if card.Hovered() {
		card.Background(theme.Hover).Border(1, theme.BorderStrong)
	}
	card.Children(func() {
		widget.TeamMark(c, t.ID, t.Name, 32)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, t.Name).FontSize(13.5).FontWeight(600).SingleLine()
			ui.Text(c, "Since "+t.Created.Local().Format("2 Jan 2006")).FontSize(11.5).TextColor(theme.Muted).SingleLine()
		})
		widget.Tag(c, t.Role, theme.TextSoft)
		ui.Icon(c, icon.ChevronRight).FontSize(12).TextColor(theme.Muted)
	})
	if card.Clicked() {
		a.openCloudTeam(cs, t.ID)
	}
}

// openCloudTeam opens team id's dialog and reads its members.
func (a *App) openCloudTeam(cs *cloudSettings, id string) {
	ts := &cs.teams
	ts.open, ts.team, ts.invite, ts.tab, ts.audit = id, api.Team{}, api.Invite{}, tabMembers, nil
	a.cloudDo("", func(cl *client.Client) error {
		var full api.Team
		err := cl.CloudCall("GET", teamPath(api.PathTeam, id), nil, &full)
		a.onMain(func() {
			if ts.open == id {
				ts.team = full
			}
		})
		return err
	}, nil)
}

// cloudTeamDialogs are the Teams tab's dialogs: a new team, joining one,
// and the team open.
func (a *App) cloudTeamDialogs(c *ui.Context, cs *cloudSettings) {
	ts := &cs.teams
	if ts.making {
		widget.FormSheet(c, &ts.making, 440, "New team", "You'll own it, and invite its members once it's made.", func() {
			widget.Field(c, "Name", func() {
				widget.Input(c, &ts.newName, "enowX Labs").FillWidth()
			})
		}, func() {
			name := strings.TrimSpace(ts.newName)
			b := widget.PrimaryButton(c, icon.Plus, "Make team", "")
			if name == "" {
				b.Disabled(true)
			} else if b.Clicked() {
				var t api.Team
				a.cloudDo("Making "+name+"…", func(cl *client.Client) error {
					return cl.CloudCall("POST", api.PathTeams, api.TeamParams{Name: &name}, &t)
				}, func() {
					ts.making = false
					a.loadCloud()
					a.openCloudTeam(cs, t.ID)
					ts.tab = tabInvite // the next thing to do
				})
			}
		})
	}
	if ts.joining {
		widget.FormSheet(c, &ts.joining, 480, "Join a team", "Paste the invite link a team's admin sent you. It works once.", func() {
			widget.Field(c, "Invite link", func() {
				widget.Input(c, &ts.join, "https://cloud.enowx.ai/invite#enxi_…").FillWidth().Font(theme.MonoFont)
			})
		}, func() {
			link := strings.TrimSpace(ts.join)
			b := widget.PrimaryButton(c, icon.Link, "Join", "")
			if link == "" {
				b.Disabled(true)
			} else if b.Clicked() {
				a.cloudDo("Joining…", func(cl *client.Client) error {
					return cl.CloudCall("POST", api.PathJoin, api.JoinParams{Token: link}, nil)
				}, func() {
					ts.joining, ts.join = false, ""
					a.loadCloud()
				})
			}
		})
	}
	if ts.open != "" {
		a.cloudTeamDialog(c, cs)
		a.cloudTeamDeleteDialog(c, cs)
	}
}

// cloudTeamDialog is the open team: its members, and for admins an
// invite to make.
func (a *App) cloudTeamDialog(c *ui.Context, cs *cloudSettings) {
	ts := &cs.teams
	t := ts.team
	if t.ID == "" {
		for _, x := range ts.list {
			if x.ID == ts.open {
				t = x
			}
		}
	}
	open := true
	tabs := teamTabs
	if !canAdmin(t.Role) {
		tabs = slices.DeleteFunc(slices.Clone(tabs), func(x string) bool { return slices.Contains(adminTabs, x) })
	}
	tab := ts.tab
	if !slices.Contains(tabs, tab) {
		tab = tabMembers // kept while the team's role is read
	}
	widget.Sheet(c, &open, 560, func() {
		// The tabs sit in the header, so they stay as a long tab scrolls.
		ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				widget.TeamMark(c, t.ID, t.Name, 34)
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, t.Name).FontSize(15).SingleLine()
					sub := "You're its " + t.Role
					if n := len(ts.team.Members); n > 0 {
						sub += " · " + fmt.Sprintf("%d members", n)
						if n == 1 {
							sub = "You're its " + t.Role + " · 1 member"
						}
					}
					ui.Text(c, sub).FontSize(11.5).TextColor(theme.Muted).SingleLine()
				})
				if widget.IconButton(c, icon.Close, "Close", false).Clicked() {
					open = false
				}
			})
			if len(tabs) > 1 {
				ui.Row(c).FillWidth().Children(func() {
					if i := widget.Segmented(c, tabs, slices.Index(tabs, tab), nil); i >= 0 {
						ts.tab = tabs[i]
					}
				})
			}
		})
	}, func() {
		if cs.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, cs.err, "", nil)
		}
		if ts.team.ID == "" {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				widget.Spinner(c, 12)
				ui.Text(c, "Reading the team…").FontSize(12).TextColor(theme.Muted)
			})
			return
		}
		switch tab {
		case tabProjects:
			a.cloudTeamProjects(c, cs)
			return
		case tabHosts:
			a.cloudTeamHosts(c, cs)
			return
		case tabKit:
			a.cloudTeamKit(c, cs)
			return
		case tabInvite:
			a.cloudInvite(c, cs)
			return
		case tabOptions:
			a.cloudTeamSettings(c, cs)
			return
		case tabActivity:
			a.cloudTeamAudit(c, cs)
			return
		}
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			for _, m := range ts.team.Members {
				a.cloudMemberRow(c, cs, m)
			}
		})
	}, func() {
		if m, ok := cs.member(); ok && m.Role != api.RoleOwner {
			if widget.GhostButton(c, "Leave the team").Clicked() {
				acc := m.Account.ID
				a.confirm("Leave "+t.Name+"?", "Its notes and canvases leave this host's Notes and Canvas, and you need a new invite to come back.", "Leave the team", func() {
					a.removeMember(cs, acc)
				})
			}
		}
		ui.Spacer(c)
		if widget.PrimaryButton(c, nil, "Done", "").Clicked() {
			open = false
		}
	})
	if !open {
		ts.open, ts.team, ts.invite, ts.deleting = "", api.Team{}, api.Invite{}, false
	}
}

// member is the user as a member of the open team.
func (cs *cloudSettings) member() (api.Member, bool) {
	for _, m := range cs.teams.team.Members {
		if m.Account.ID == cs.st.AccountID {
			return m, true
		}
	}
	return api.Member{}, false
}

func (a *App) removeMember(cs *cloudSettings, account string) {
	ts := &cs.teams
	team, me := ts.team.ID, account == cs.st.AccountID
	a.cloudDo("", func(cl *client.Client) error {
		return cl.CloudCall("DELETE", strings.Replace(teamPath(api.PathMember, team), "{account}", account, 1), nil, nil)
	}, func() {
		delete(a.chatsOf().membersAt, team)
		if me {
			ts.open = ""
		} else {
			a.openCloudTeam(cs, team)
		}
		a.loadCloud()
	})
}

func (a *App) cloudMemberRow(c *ui.Context, cs *cloudSettings, m api.Member) {
	ts := &cs.teams
	ui.Row(c.Key("member:"+m.Account.ID)).FillWidth().Gap(10).AlignItems(ui.Center).Padding(8, 10).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
		initial := "?"
		if m.Account.Name != "" {
			initial = strings.ToUpper(string([]rune(m.Account.Name)[0]))
		}
		ui.Box(c).Size(28, 28).Radius(14).Shrink(0).Background(theme.Hover).Center().Children(func() {
			ui.Text(c, initial).FontSize(12).FontWeight(600)
		})
		me := m.Account.ID == cs.st.AccountID
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			name := m.Account.Name
			if me {
				name += " (you)"
			}
			ui.Text(c, name).FontSize(12.5).SingleLine()
			if m.Account.GitHub != "" {
				ui.Text(c, "@"+m.Account.GitHub).FontSize(11).TextColor(theme.Muted).SingleLine()
			}
		})
		if me || m.Role == api.RoleOwner || !canAdmin(ts.team.Role) {
			widget.Tag(c, m.Role, theme.TextSoft)
			return
		}
		a.memberRoleMenu(c, cs, m)
		if widget.IconButton(c, icon.Close, "Remove "+m.Account.Name, false).Clicked() {
			acc, name := m.Account.ID, m.Account.Name
			a.confirm("Remove "+name+" from "+ts.team.Name+"?", name+" loses the team's notes, canvases and hosts; new items use a key they never had. What they could read before, they may have kept.", "Remove", func() {
				a.removeMember(cs, acc)
			})
		}
	})
}

// memberRoleMenu is a member's role, which an admin changes from it; the
// owner also hands the team over from it, after a confirmation.
func (a *App) memberRoleMenu(c *ui.Context, cs *cloudSettings, m api.Member) {
	ts := &cs.teams
	b := ui.ButtonBase(c).Height(24).PaddingX(9).Gap(5).Radius(12).Shrink(0).Border(1, theme.Border).Cursor(ui.CursorPointer).
		Label("Role of " + m.Account.Name).Tooltip("Change " + m.Account.Name + "'s role")
	if b.Hovered() {
		b.Background(theme.Hover)
	}
	b.Children(func() {
		ui.Text(c, m.Role).FontSize(11.5).TextColor(theme.TextSoft)
		ui.Icon(c, icon.ChevronDown).FontSize(10).TextColor(theme.Muted)
	})
	key := "role:" + m.Account.ID
	a.menu.OpenBelow(b, key, 24)
	a.menu.Show(c, b, key, func(done func()) {
		for _, r := range []string{api.RoleAdmin, api.RoleMember, api.RoleViewer} {
			label := r
			if r == m.Role {
				label += " (now)"
			}
			svg := map[string]*ui.SVG{api.RoleAdmin: icon.Shield, api.RoleMember: icon.Account, api.RoleViewer: icon.Eye}[r]
			if widget.MenuItemTip(c.Key(r), svg, label, roleAbout(r), false) {
				done()
				if r != m.Role {
					a.setMemberRole(cs, m.Account.ID, r)
				}
			}
		}
		if ts.team.Role == api.RoleOwner {
			widget.MenuSeparator(c)
			if widget.MenuItem(c.Key("owner"), icon.Shield, "Make owner…", true) {
				done()
				acc, name := m.Account.ID, m.Account.Name
				a.confirm("Make "+name+" the owner of "+ts.team.Name+"?", name+" becomes its owner, who alone can delete it or hand it over again; you become an admin.", "Make owner", func() {
					a.setMemberRole(cs, acc, api.RoleOwner)
				})
			}
		}
	})
}

// setMemberRole gives account role in the open team.
func (a *App) setMemberRole(cs *cloudSettings, account, role string) {
	team := cs.teams.team.ID
	a.cloudDo("Changing the role…", func(cl *client.Client) error {
		return cl.CloudCall("PATCH", strings.Replace(teamPath(api.PathMember, team), "{account}", account, 1), api.RoleParams{Role: role}, nil)
	}, func() {
		delete(a.chatsOf().membersAt, team) // the panel's Members show it now
		a.openCloudTeam(cs, team)
	})
}

// cloudInvite makes a one-time invite link to the open team, as a role.
func (a *App) cloudInvite(c *ui.Context, cs *cloudSettings) {
	ts := &cs.teams
	widget.Field(c, "Invite as", func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if picked := widget.Segmented(c, []string{"Member", "Viewer", "Admin"}, ts.role, nil); picked >= 0 {
				ts.role = picked
			}
			ui.Spacer(c)
			if widget.Button(c, widget.Secondary, icon.Link, "Make a link", "").Clicked() {
				team, role := ts.team.ID, inviteRoles[ts.role]
				a.cloudDo("", func(cl *client.Client) error {
					var in api.Invite
					err := cl.CloudCall("POST", teamPath(api.PathInvites, team), api.InviteParams{Role: role}, &in)
					a.onMain(func() { ts.invite = in })
					return err
				}, nil)
			}
		})
		ui.Text(c, roleAbout(inviteRoles[ts.role])).FontSize(11.5).TextColor(theme.Muted)
	})
	if in := ts.invite; in.Token != "" {
		link := in.URL
		if link == "" {
			link = in.Token
		}
		widget.Field(c, "Link", func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Padding(8, 10).Radius(theme.RadiusControl).Background(theme.Sunken).Border(1, theme.Border).Children(func() {
				ui.Text(c, link).Font(theme.MonoFont).FontSize(11.5).Grow(1).Shrink(1).MinWidth(0).SingleLine()
				widget.CopyIcon(c.Key("invite-copy"), &ts.copied, icon.Copy, "Copy the link", "Copy the invite link", link)
			})
			ui.Text(c, "For one person, as "+in.Role+", until "+in.Expires.Local().Format("2 Jan 15:04")+". It shows once: copy it now.").FontSize(11.5).TextColor(theme.Muted)
		})
	}
}

// roleAbout says what a role may do (docs/cloud.md, Roles).
func roleAbout(role string) string {
	switch role {
	case api.RoleAdmin:
		return "Admins invite and remove members, change roles, and set the team's rules."
	case api.RoleViewer:
		return "Viewers read the team's notes and canvases and watch its sessions."
	}
	return "Members edit the team's notes and canvases and work in its projects."
}

// cloudTeamSettings is the open team's rules, for its admins.
func (a *App) cloudTeamSettings(c *ui.Context, cs *cloudSettings) {
	ts := &cs.teams
	out := ts.team.CopyOut
	widget.Field(c, "Copying out", func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			label := "Members may copy the team's notes and canvases to their own space"
			if widget.Switch(c, &out, label) {
				team := ts.team.ID
				a.cloudDo("", func(cl *client.Client) error {
					return cl.CloudCall("PATCH", teamPath(api.PathTeam, team), api.TeamParams{CopyOut: &out}, nil)
				}, func() { a.openCloudTeam(cs, team); ts.tab = tabOptions })
			}
			ui.Text(c, label).FontSize(12.5).Grow(1).Shrink(1)
		})
		ui.Text(c, "Off, the team's items stay in the team: members read and edit them, and nobody takes a copy into their own space.").FontSize(11.5).TextColor(theme.Muted)
	})
	a.cloudTeamDeleteSection(c, cs)
}
