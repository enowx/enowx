package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/creditview"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// pollCredits reads the host's agent accounts while connection gen lasts.
func (a *App) pollCredits(cl *client.Client, gen int) {
	a.credits.Follow(cl, a.onMain, func() bool { return gen == a.gen })
}

// creditsHost describes the connected host to the credit views.
func (a *App) creditsHost() creditview.Host {
	h := creditview.Host{Name: a.host.Name, AgentName: a.agentName}
	if p, ok := a.currentProject(); ok {
		h.SignIn = func(agent string) { a.signInFromCredits(p, agent) }
	}
	return h
}

// creditsPage is the Credits page of the connected host.
func (a *App) creditsPage(c *ui.Context) {
	widget.PageHeader(c, "Credits", "Accounts on "+a.host.Name, func() {
		if s := a.credits.CheckedLabel(); s != "" {
			ui.Text(c, s).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
		}
		if widget.GhostButton(c, "Agents' configs").Clicked() {
			a.page, a.settingsSection = pageSettings, sectionAgents
		}
		tip := "enowX reads these with each agent's own sign-in on " + a.host.Name + ", from the agent's vendor only, every few minutes while connected."
		if widget.Button(c, widget.Secondary, icon.Sync, "Read again", "").Tooltip(tip).Clicked() {
			a.credits.Refresh()
		}
	})
	a.credits.Page(c, a.creditsHost())
}

// signInFromCredits starts a session that signs agent in again, in
// project p, and shows it.
func (a *App) signInFromCredits(p proto.ProjectInfo, agent string) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		info, err := cl.CreateSession(proto.SessionCreateParams{Project: p.Path, Agent: agent, SignIn: true})
		a.onMain(func() {
			if err != nil {
				a.status = strings.TrimPrefix(err.Error(), proto.MethodSessionCreate+": ")
				return
			}
			a.upsert(info)
			a.selectSession(info)
		})
	}()
}
