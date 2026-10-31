package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/usageview"
	"github.com/enowx/enowx/ui/widget"
)

// pollUsage measures while connection gen lasts (see package usageview).
func (a *App) pollUsage(cl *client.Client, gen int) {
	a.usage.Follow(cl, a.onMain, func() bool { return gen == a.gen })
}

// usageHost describes the connected host to the usage views.
func (a *App) usageHost() usageview.Host {
	return usageview.Host{
		Name:      a.host.Name,
		Local:     a.host.ID == hosts.LocalID,
		LocalName: LocalHost().Name,
		Session: func(id string) (name, agent, project string) {
			for _, s := range a.sessions {
				if s.ID == id {
					if s.Project != "" {
						project = textfmt.LastPathPart(s.Project)
					}
					return a.sessionName(s), s.Agent, project
				}
			}
			return id, "", ""
		},
	}
}

// usagePage is the Resource usage page of the connected host.
func (a *App) usagePage(c *ui.Context) {
	a.viewHeader(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Breadcrumb(c, a.host.Name, "Resource usage")
		})
		if widget.IconButton(c, icon.Close, "Back to sessions", false).Clicked() {
			a.page = ""
		}
	})
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(16).Gap(16).Children(func() {
			tabs := []string{"Memory & CPU", "Tokens"}
			cur := 0
			if a.tok.pageTab == usageTabTokens {
				cur = 1
			}
			if i := widget.Segmented(c.Key("usage-tabs"), tabs, cur, nil); i >= 0 {
				a.tok.pageTab = []string{"", usageTabTokens}[i]
			}
			if a.tok.pageTab == usageTabTokens {
				a.tokensTab(c.Key("tokens"))
				return
			}
			a.usage.Details(c, a.usageHost())
		})
	})
}

// usageTabTokens is the Resource usage page's Tokens tab (token usage,
// tokens.go); "" its memory and CPU.
const usageTabTokens = "tokens"

// usageEvery is how often usage is measured: every 2 seconds on the
// Resource usage page, every 10 for the session list's sizes, every minute
// while the window isn't focused.
func (a *App) usageEvery() time.Duration {
	switch {
	case a.blurred:
		return time.Minute
	case a.page == pageUsage || a.usage.Open():
		return 2 * time.Second
	}
	return 10 * time.Second
}

// watchUsageShown measures at once when the Resource usage page opens.
func (a *App) watchUsageShown() {
	shown := a.page == pageUsage
	if shown && !a.usageShown {
		a.usage.Poke()
	}
	a.usageShown = shown
}

// memoryChip is the title bar's memory of the connected host.
func (a *App) memoryChip(c *ui.Context) {
	if a.client == nil {
		return
	}
	a.usage.Chip(c, a.usageHost(), usageview.ChipActions{
		OpenPage:   func() { a.page = pageUsage },
		StopAgents: a.stopAgentsHere,
		Select: func(id string) {
			for _, s := range a.sessions {
				if s.ID == id {
					a.selectSession(s)
				}
			}
		},
	})
}
