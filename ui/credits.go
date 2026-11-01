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
	h := creditview.Host{
		Name:      a.host.Name,
		AgentName: a.agentName,
		OpenPage:  func() { a.page = pageCredits },
	}
	for _, s := range a.sessions {
		if s.ID == a.selected {
			h.Focus = s.Agent
		}
	}
	if p, ok := a.currentProject(); ok {
		h.SignIn = func(agent string) { a.signInFromCredits(p, agent) }
	}
	return h
}

// creditChip is the title bar's credits button, once connected.
func (a *App) creditChip(c *ui.Context) {
	if a.client == nil {
		return
	}
	a.credits.Chip(c, a.creditsHost())
}

// creditsPage is the Credits page of the connected host.
func (a *App) creditsPage(c *ui.Context) {
	a.viewHeader(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Breadcrumb(c, a.host.Name, "Credits")
		})
		ui.Text(c, a.credits.CheckedLabel()).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		if widget.GhostButton(c, "Read again").Clicked() {
			a.credits.Refresh()
		}
		if widget.IconButton(c, icon.Close, "Back to sessions", false).Clicked() {
			a.page = ""
		}
	})
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(16).Children(func() {
			a.credits.Page(c, a.creditsHost())
		})
	})
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
