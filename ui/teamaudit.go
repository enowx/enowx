package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/ui/icon"
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

// teamAuditOf is the open team's log, read when first asked for.
func (a *App) teamAuditOf(cs *cloudSettings) *teamAudit {
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
	return ta
}

// auditLine is what an entry of the log says happened.
func auditLine(e api.AuditEntry) string {
	who := e.Account.Name
	if who == "" {
		who = "The cloud"
	}
	what := auditActions[e.Action]
	if what == "" {
		what = e.Action
	}
	line := who + " " + what
	if e.Detail != "" {
		line += ": " + e.Detail
	}
	return line
}

// teamAuditRow is a line of a team's log: its time, the full date in a tip, and
// what happened, days before today saying which.
func teamAuditRow(c *ui.Context, e api.AuditEntry, now time.Time) {
	line := auditLine(e)
	if day := ragDay(e.At, now); day != "Today" {
		line = day + ": " + line
	}
	ui.Row(c).FillWidth().Shrink(0).MinHeight(36).Gap(theme.S3).BorderWidth(theme.Rule, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Text(c, e.At.Local().Format("15:04")).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0).
			Tooltip(e.At.Local().Format("2 Jan 2006 15:04"))
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, line, widget.RunStyle{Size: theme.TextLabel}) })
	})
}

// cloudTeamAudit is the team dialog's Activity tab.
func (a *App) cloudTeamAudit(c *ui.Context, cs *cloudSettings) {
	ta := a.teamAuditOf(cs)
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
	ui.Column(c).FillWidth().Children(func() {
		for _, e := range ta.entries {
			teamAuditRow(c.Key(e.ID), e, now)
		}
	})
}
