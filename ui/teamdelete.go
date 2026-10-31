package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Deleting a team (docs/cloud.md, Deleting a team): its owner alone, from
// the team's Options, in a dialog saying what goes and asking for the
// team's name typed, as nothing of it can be brought back.

// teamDeleteGoes is what deleting a team takes, for everyone in it.
var teamDeleteGoes = []string{
	"Its chat, notes and canvases, from the cloud and from every member's hosts as they next sync",
	"Its projects list and its kit",
	"Its members, invites and keys",
	"Hosts shared with it: they stop being shared, and visitors are let go",
}

// cloudTeamDeleteSection is the Options tab's last part, for the owner.
func (a *App) cloudTeamDeleteSection(c *ui.Context, cs *cloudSettings) {
	ts := &cs.teams
	if ts.team.Role != api.RoleOwner {
		return
	}
	widget.Field(c, "Deleting the team", func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Its space goes for every member, from the cloud and from their hosts. It can't be undone.").FontSize(11.5).TextColor(theme.Muted).Grow(1).Shrink(1)
			if widget.DangerButton(c, "Delete "+ts.team.Name+"…").Clicked() {
				ts.deleting, ts.deleteName = true, ""
			}
		})
	})
}

// cloudTeamDeleteDialog asks for the open team's name before deleting it.
func (a *App) cloudTeamDeleteDialog(c *ui.Context, cs *cloudSettings) {
	ts := &cs.teams
	if !ts.deleting || ts.team.ID == "" {
		return
	}
	t := ts.team
	widget.FormSheet(c, &ts.deleting, 460, "Delete "+t.Name+"?", "Nothing of it can be brought back.", func() {
		if cs.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, cs.err, "", nil)
		}
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Text(c, "What goes:").FontSize(12.5)
			for _, s := range teamDeleteGoes {
				ui.Row(c.Key(s)).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
					ui.Text(c, "•").FontSize(12).TextColor(theme.Muted)
					ui.Text(c, s).FontSize(12).TextColor(theme.TextSoft).Grow(1).Shrink(1)
				})
			}
			ui.Text(c, "Members' own notes and canvases stay theirs.").FontSize(11.5).TextColor(theme.Muted)
		})
		widget.Field(c, "Type "+t.Name+" to confirm", func() {
			widget.Input(c, &ts.deleteName, t.Name).FillWidth()
		})
	}, func() {
		b := widget.DangerButton(c, "Delete the team")
		if !teamNameTyped(ts.deleteName, t.Name) || cs.busy != "" {
			b.Disabled(true)
		} else if b.Clicked() {
			a.deleteCloudTeam(cs, t.ID)
		}
	})
}

// teamNameTyped says typed is the team's name, but for spaces around it.
func teamNameTyped(typed, name string) bool { return strings.TrimSpace(typed) == name }

// deleteCloudTeam deletes team; its space leaves this host as the cloud
// says it's done (the daemon's learnFrom).
func (a *App) deleteCloudTeam(cs *cloudSettings, team string) {
	ts := &cs.teams
	a.cloudDo("Deleting the team…", func(cl *client.Client) error {
		return cl.CloudCall("DELETE", teamPath(api.PathTeam, team), nil, nil)
	}, func() {
		ts.deleting, ts.deleteName = false, ""
		if ts.open == team {
			ts.open, ts.team, ts.invite = "", api.Team{}, api.Invite{}
		}
		delete(a.chatsOf().membersAt, team)
		a.loadCloud()
	})
}
