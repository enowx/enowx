package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
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
		widget.FormSheet(c, &ts.making, 480, "New team", "You'll own it, and invite its members once it's made.", func() {
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
			b := widget.PrimaryButton(c, nil, "Join", "")
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
	widget.Sheet(c, &open, 760, func() {
		// The tabs sit in the header, so they stay as a long tab scrolls.
		ui.Column(c).Grow(1).MinWidth(0).Gap(theme.S3).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Start).Children(func() {
				sub := "You're its " + t.Role
				if n := len(ts.team.Members); n > 0 {
					sub += " · " + textfmt.Plural(n, "1 member", fmt.Sprintf("%d members", n))
				}
				widget.SheetHead(c, &open, t.Name, sub)
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
			ui.Row(c).Gap(theme.S2).Children(func() {
				widget.Spinner(c, 12)
				ui.Text(c, "Reading the team…").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
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
		ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
			for _, m := range ts.team.Members {
				a.cloudMemberRow(c, cs, m)
			}
		})
		widget.CardNote(c, roleAbout(api.RoleAdmin)+" "+roleAbout(api.RoleMember)+" "+roleAbout(api.RoleViewer), 0)
	}, func() {
		if m, ok := cs.member(); ok && m.Role != api.RoleOwner {
			if widget.DangerButton(c, "Leave the team").Clicked() {
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
		if picked := segmentedInCard(c, []string{"Member", "Viewer", "Admin"}, ts.role, 240); picked >= 0 {
			ts.role = picked
		}
		widget.CardNote(c, roleAbout(inviteRoles[ts.role]), 0)
	})
	ui.Row(c.Key("invite-make")).FillWidth().Children(func() {
		if widget.PrimaryButton(c, nil, "Make a link", "").Clicked() {
			team, role := ts.team.ID, inviteRoles[ts.role]
			a.cloudDo("", func(cl *client.Client) error {
				var in api.Invite
				err := cl.CloudCall("POST", teamPath(api.PathInvites, team), api.InviteParams{Role: role}, &in)
				a.onMain(func() { ts.invite = in })
				return err
			}, nil)
		}
	})
	if in := ts.invite; in.Token != "" {
		link := in.URL
		if link == "" {
			link = in.Token
		}
		widget.Field(c, "Invite link", func() {
			box := ui.Row(c).FillWidth().MinWidth(0).Height(theme.ControlHeight).Padding(0, 0, 0, theme.S3).Gap(theme.S2).Radius(theme.RadiusFull)
			widget.Well(box, false).Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, link, widget.RunStyle{Mono: true}) })
				widget.CopyIcon(c.Key("invite-copy"), &ts.copied, icon.Copy, "Copy the link", "Copy the invite link", link)
			})
			widget.CardNote(c, "For one person, as "+in.Role+", until "+in.Expires.Local().Format("2 Jan 15:04")+". It shows once: copy it now.", 0)
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
	if switchRow(c, &out, "Copying out", "Members may copy the team's notes and canvases to their own space. Off, the team's items stay in the team.") {
		team := ts.team.ID
		a.cloudDo("", func(cl *client.Client) error {
			return cl.CloudCall("PATCH", teamPath(api.PathTeam, team), api.TeamParams{CopyOut: &out}, nil)
		}, func() { a.openCloudTeam(cs, team); ts.tab = tabOptions })
	}
	a.cloudTeamDeleteSection(c, cs)
}
