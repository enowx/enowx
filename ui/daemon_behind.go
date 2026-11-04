package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/hostconn"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// An update replaces the app at once, and a host's daemon only when that stops nothing:
// its sessions' programs are its own, and end with it. Until then the daemon stays at
// its version and a card says what replacing it would stop, for the user to choose.

// daemonBehindLook is how often a daemon left behind is asked what it runs now.
const daemonBehindLook = 30 * time.Second

// daemonBehind is a host's daemon left at another version.
type daemonBehind struct {
	host, name, version string
	plan                proto.RestartPlan
	// waiting: replace it as soon as that stops nothing. hidden: don't show the card.
	waiting, hidden bool
}

// daemonBehindOptions has a connection to h say so if it leaves a daemon behind, and
// replace it if the user asked.
func (a *App) daemonBehindOptions(opts *hostconn.Options, h hosts.Host, gen int) {
	opts.ReplaceBusy = a.replaceDaemon == h.ID
	a.replaceDaemon = ""
	if opts.ReplaceBusy {
		a.diag.Event("daemon.replaceAsked", "host", h.ID)
	}
	waiting := a.daemonBehind != nil && a.daemonBehind.host == h.ID && a.daemonBehind.waiting
	a.daemonBehind = nil
	opts.Kept = func(info proto.DaemonInfo, plan proto.RestartPlan) {
		a.diag.Event("daemon.kept", "host", h.ID, "version", info.Version, "agents", len(plan.Agents), "commands", len(plan.Commands))
		a.onMain(func() {
			if gen == a.gen {
				a.daemonBehind = &daemonBehind{host: h.ID, name: h.Name, version: info.Version, plan: plan, waiting: waiting}
			}
		})
	}
}

// followDaemonBehind says what the last restart ended, and keeps a daemon left behind's
// plan current, replacing it once that stops nothing if the user chose that.
func (a *App) followDaemonBehind(c *client.Client, h hosts.Host, gen int) {
	if plan, err := c.RestartPlan(); err == nil && len(plan.Lost) > 0 {
		a.onMain(func() {
			if gen == a.gen {
				a.notify("The daemon's restart ended: " + restartItems(plan.Lost))
			}
		})
	}
	for {
		select {
		case <-c.Done():
			return
		case <-time.After(daemonBehindLook):
		}
		// Only a daemon left behind has a plan to keep current: with none, asking every
		// half minute looked at every process, for nothing.
		behind := make(chan bool, 1)
		a.onMain(func() { behind <- gen == a.gen && a.daemonBehind != nil && a.daemonBehind.host == h.ID })
		select {
		case <-c.Done():
			return
		case yes := <-behind:
			if !yes {
				continue
			}
		}
		plan, err := c.RestartPlan()
		if err != nil {
			continue
		}
		a.onMain(func() {
			b := a.daemonBehind
			if gen != a.gen || b == nil || b.host != h.ID {
				return
			}
			b.plan = plan
			if b.waiting && plan.Empty() {
				a.diag.Event("daemon.replaceWhenDone", "host", h.ID)
				a.replaceDaemon = h.ID
				a.connect(h)
			}
		})
	}
}

// showsDaemonBehindCard says the card has something to show.
func (a *App) showsDaemonBehindCard() bool {
	return a.daemonBehind != nil && !a.daemonBehind.hidden && a.daemonBehind.host == a.host.ID
}

// daemonBehindCard offers replacing a daemon left behind: now, saying what stops, or
// once its agents are done.
func (a *App) daemonBehindCard(c *ui.Context) {
	if !a.showsDaemonBehindCard() {
		return
	}
	b := a.daemonBehind
	widget.Float(ui.Column(c).FillWidth().Padding(theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).
		Background(theme.Dialog).Border(1, theme.BorderStrong)).Transition(widget.OverlayMotion).Children(func() {
		widget.Running(c, "enowxd on "+b.name+" stays at "+b.version+" for now", widget.RunStyle{Weight: 500})
		ui.Text(c, restartCost(b.plan)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		ui.Row(c).FillWidth().Gap(theme.S2).Justify(ui.End).Children(func() {
			if widget.GhostButton(c, "When agents are done").Clicked() {
				b.waiting, b.hidden = true, true
			}
			if widget.PrimaryButton(c, icon.Restore, "Restart it now", "").Clicked() {
				a.confirm("Restart enowxd on "+b.name+"?", restartCost(b.plan), "Restart it", func() {
					a.replaceDaemon = b.host
					a.connect(a.host)
				})
			}
		})
	})
}

// restartCost says what replacing a daemon stops now.
func restartCost(plan proto.RestartPlan) string {
	if plan.Empty() {
		return "Replacing it stops nothing now."
	}
	var parts []string
	if len(plan.Agents) > 0 {
		parts = append(parts, fmt.Sprintf("Agents at work stop, then carry on from their conversation: %s.", restartItems(plan.Agents)))
	}
	if len(plan.Commands) > 0 {
		parts = append(parts, fmt.Sprintf("These end, unless they are agents, which come back: %s.", restartItems(plan.Commands)))
	}
	return strings.Join(parts, " ")
}

// restartItems lists items, the first few by what and session.
func restartItems(items []proto.RestartItem) string {
	const most = 5
	var words []string
	for i, it := range items {
		if i == most {
			words = append(words, fmt.Sprintf("and %d more", len(items)-most))
			break
		}
		word := it.What
		if it.Name != "" {
			word += " (" + it.Name + ")"
		}
		words = append(words, word)
	}
	return strings.Join(words, ", ")
}
