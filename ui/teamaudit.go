package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A team's log (docs/cloud.md): who signed in, was invited, changed roles,
// entered a shared host, watched or took over a session; its admins read
// it in the team's Activity tab.

// auditActions say what each action of the log was.
var auditActions = map[string]string{
	"account.created": "made their account", "device.added": "signed in a device", "device.removed": "signed a device out",
	"team.created": "made the team", "team.renamed": "renamed the team", "team.copy_out": "changed copying out",
	"invite.created": "invited", "invite.deleted": "withdrew an invite", "member.joined": "joined",
	"member.role": "changed a role", "member.removed": "removed a member", "member.left": "left",
	"key.shared": "shared the team's key", "host.entered": "entered a shared host",
	api.AuditRulesChanged: "changed a project's rules", api.AuditMemberSession: "started a session",
	api.AuditWatched: "watched a session", api.AuditTakeover: "took over a session", api.AuditRead: "had their agent read a session",
}

type teamAudit struct {
	team    string
	entries []api.AuditEntry
	loaded  bool
	err     string
}

// cloudTeamAudit is the team dialog's Activity tab.
func (a *App) cloudTeamAudit(c *ui.Context, cs *cloudSettings) {
	team := cs.teams.team.ID
	ta := cs.teams.audit
	if ta == nil || ta.team != team {
		ta = &teamAudit{team: team}
		cs.teams.audit = ta
		cl := a.client
		go func() {
			var list []api.AuditEntry
			err := cl.CloudCall("GET", teamPath(api.PathAudit, team), nil, &list)
			a.onMain(func() {
				ta.loaded, ta.entries = true, list
				if err != nil {
					ta.err = client.Explain(err)
				}
			})
		}()
	}
	if ta.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, ta.err, "", nil)
	}
	if !ta.loaded {
		ui.Text(c, "Reading the team's log…").FontSize(theme.TextLabel).TextColor(theme.Muted)
		return
	}
	if len(ta.entries) == 0 {
		ui.Text(c, "Nothing yet.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	now := time.Now()
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		for _, e := range ta.entries {
			ui.Row(c.Key(e.ID)).FillWidth().Gap(10).AlignItems(ui.Start).Padding(6, 4).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
				who := e.Account.Name
				if e.Account.GitHub != "" {
					who = "@" + e.Account.GitHub
				}
				if who == "" {
					who = "The cloud"
				}
				what := auditActions[e.Action]
				if what == "" {
					what = e.Action
				}
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, who+" "+what).FontSize(theme.TextUI).SingleLine()
					if e.Detail != "" {
						ui.Text(c, e.Detail).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
					}
				})
				ui.Text(c, textfmt.Age(e.At, now)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(0).
					Tooltip(e.At.Local().Format("2 Jan 2006 15:04"))
			})
		}
	})
}
