package ui

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
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
	h := usageview.Host{
		Name:      a.host.Name,
		Local:     a.host.ID == hosts.LocalID,
		LocalName: LocalHost().Name,
		AgentName: a.agentName,
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
		Select: func(id string) {
			for _, s := range a.sessions {
				if s.ID == id {
					a.page = ""
					a.selectSession(s)
				}
			}
		},
	}
	for _, s := range a.sessions {
		if s.State == proto.StateWorking {
			h.Working++
		}
	}
	for _, w := range a.watches {
		h.Others = append(h.Others, usageview.OtherHost{Name: w.host.Name, Connected: w.status.state == hostOnline,
			Sessions: len(w.status.sessions), Usage: w.status.usage})
	}
	slices.SortFunc(h.Others, func(x, y usageview.OtherHost) int { return cmp.Compare(x.Name, y.Name) })
	return h
}

// usagePage is the Resource usage page of the connected host.
func (a *App) usagePage(c *ui.Context) {
	h := a.usageHost()
	names := []string{a.host.Name}
	for _, o := range h.Others {
		names = append(names, o.Name)
	}
	widget.PageHeader(c, "Resource usage", strings.Join(names, ", "), func() {
		cur := 0
		if a.tok.pageTab == usageTabTokens {
			cur = 1
		}
		if i := widget.Segmented(c.Key("usage-tabs"), []string{"Memory & CPU", "Tokens"}, cur, nil); i >= 0 {
			a.tok.pageTab = []string{"", usageTabTokens}[i]
		}
		if len(a.sessions) > 0 && widget.DangerButton(c, "Stop all agents…").Clicked() {
			a.confirm("Stop all agents on "+a.host.Name+"?", "Their sessions stay, to continue.", "Stop all", a.stopAgentsHere)
		}
	})
	if a.tok.pageTab == usageTabTokens {
		a.tokensTab(c.Key("tokens"))
		return
	}
	a.usage.Figures(c, h)
	a.usage.Details(c, h)
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
	case a.page == pageUsage:
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
