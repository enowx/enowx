package ui

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// syncSection is Settings' Sync: how the user's data is synced with each
// host, the conflicts to settle and what was deleted lately.
func (a *App) syncSection(c *ui.Context) {
	summary, alarm := a.syncSummary()
	var others []hosts.Host
	synced := 0
	for _, h := range a.allHosts() {
		if h.ID == hosts.LocalID {
			continue
		}
		others = append(others, h)
		if st, ok := a.dsync.hosts[h.ID]; ok && st.state == syncIdle {
			synced++
		}
	}
	widget.PageHeader(c, "Sync", "The same on every host", func() {
		if widget.PrimaryButton(c, icon.Sync, "Sync now", "").Clicked() {
			a.syncData()
		}
	})
	hub := a.dsync.hub
	widget.FigsBand(c, []widget.Fig{
		{Label: "Files", Value: fmt.Sprint(hub.Files)},
		{Label: "Size", Value: textfmt.Bytes(uint64(hub.Size))},
		{Label: "Hosts synced", Value: fmt.Sprintf("%d of %d", synced, len(others))},
		{Label: "Conflicts", Value: fmt.Sprint(len(a.dsync.conflicts))},
		{Label: "Recently deleted", Value: fmt.Sprint(len(a.dsync.trash))},
	})
	widget.PageBody(c, true, func() {
		ui.Row(c.Key("summary")).FillWidth().Gap(theme.S2).Children(func() {
			col := theme.Done
			if alarm {
				col = theme.Warning
			}
			widget.Dot(c, col, a.dsync.running, 8)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, summary, widget.RunStyle{Weight: 500}) })
		})
		widget.CardNote(c, "Your notes, their groups, canvases, settings and the vault's encrypted data are the same on every host. This device syncs each host it reaches with its own, when it connects, every 30 seconds and right after a change. Project files are not synced here.", 0)
		ui.Row(c.Key("sync-lower")).FillWidth().Gap(theme.S3).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(7).Basis(0).MinWidth(0).Gap(theme.S2).Children(func() {
				widget.Heading(c, "Hosts")
				if len(others) == 0 {
					widget.CardNote(c, "No other host yet: add a server or a computer in Hosts, and its notes join these.", 0)
				}
				widget.Cols(c, 2).Children(func() {
					for _, h := range others {
						a.syncHostCard(c.Key(h.ID), h)
					}
				})
			})
			ui.Column(c).Grow(5).Basis(0).MinWidth(0).Gap(theme.S3).Children(func() {
				if len(a.dsync.conflicts) > 0 {
					ui.Column(c.Key("conflicts")).FillWidth().Gap(theme.S2).Children(func() {
						widget.Heading(c, "Conflicts")
						for _, cf := range a.dsync.conflicts {
							a.conflictCard(c.Key(cf.ID), cf)
						}
					})
				}
				ui.Column(c.Key("deleted")).FillWidth().Gap(theme.S2).Children(func() {
					widget.Heading(c, "Recently deleted")
					widget.Card(c).FillWidth().Children(func() { a.syncTrash(c) })
				})
			})
		})
	})
}

// syncTrash lists what was deleted lately, to bring back.
func (a *App) syncTrash(c *ui.Context) {
	if len(a.dsync.trash) == 0 {
		widget.CardNote(c, fmt.Sprintf("Nothing. What's deleted, here or on another host, is kept %d days.", 30), 0)
		return
	}
	widget.CardNote(c, fmt.Sprintf("What's deleted, here or on another host, is kept %d days.", 30), 0)
	widget.CardLines(c, func() {
		for i, it := range a.dsync.trash {
			if i == 20 {
				widget.CardNote(c, fmt.Sprintf("and %d more", len(a.dsync.trash)-20), 0)
				break
			}
			name := it.Title
			if name == "" {
				name = it.Path
			}
			widget.CardLine(c.Key(it.ID), i == 0, name, textfmt.Age(it.Deleted, time.Now()), func() {
				if widget.GhostButton(c, "Restore").Clicked() {
					a.syncCall(func(hub *client.Client) error { return hub.SyncRestore(it.ID) })
				}
			})
		}
	})
}

// syncHostCard is how syncing host h goes.
func (a *App) syncHostCard(c *ui.Context, h hosts.Host) {
	st, ok := a.dsync.hosts[h.ID]
	state, about, col := syncOffline, "Not connected: syncs when it is", theme.Idle
	hollow := true
	r := st.rep
	if ok {
		state, col, hollow = st.state, theme.Done, false
		switch st.state {
		case syncIdle:
			about = "Synced " + textfmt.Age(st.last, time.Now())
			if n := len(st.rep.Skipped); n > 0 {
				about += " · " + textfmt.Plural(n, "1 file", fmt.Sprintf("%d files", n)) + " too large to sync"
			}
		case syncSyncing:
			about = "Syncing…"
		default:
			about, col = st.err, theme.Warning
		}
	}
	widget.Card(c).Children(func() {
		widget.CardTitle(c, nil, h.Name, nil)
		widget.StatusLine(c, col, hollow, state)
		note := widget.CardNote(c, about, 2)
		if col == theme.Warning {
			note.TextColor(theme.Warning)
		}
		figure := func(n int) string {
			if !ok || st.last.IsZero() {
				return "–"
			}
			return fmt.Sprint(n)
		}
		widget.CardStats(c, []widget.Stat{{Value: figure(r.Sent), Label: "sent"}, {Value: figure(r.Received), Label: "received"}, {Value: figure(r.Merged), Label: "merged"}})
	})
}

// conflictCard is a file changed on two hosts in ways that don't merge,
// with a button per version to keep, and one to keep them all.
func (a *App) conflictCard(c *ui.Context, cf proto.SyncConflict) {
	name := cf.Title
	if name == "" {
		name = cf.Path
	}
	widget.Card(c).FillWidth().Background(theme.Fade(theme.Error, 0.13)).Children(func() {
		widget.CardTitle(c, icon.Alert, name, nil)
		widget.CardNote(c, "Both changed the same lines, "+textfmt.Age(cf.Created, time.Now())+". The first version is in place until you pick.", 0)
		ui.Row(c).FillWidth().Gap(theme.S2).Wrap().Children(func() {
			for i, v := range cf.Versions {
				label := "Keep " + v.Host + "'s"
				if v.Deleted {
					label = "Delete, as " + v.Host + " did"
				}
				if widget.GhostButton(c.Key(i), label).Clicked() {
					keep := fmt.Sprint(i + 1)
					a.syncCall(func(hub *client.Client) error { return hub.SyncResolve(cf.ID, keep) })
				}
			}
			if widget.GhostButton(c, "Keep both").Tooltip("The other version becomes a copy beside it").Clicked() {
				a.syncCall(func(hub *client.Client) error { return hub.SyncResolve(cf.ID, "both") })
			}
		})
	})
}

// syncCall runs fn on this device's daemon, then syncs.
func (a *App) syncCall(fn func(hub *client.Client) error) {
	hub := a.hubClient()
	if hub == nil {
		a.status = "This device's enowX isn't connected"
		return
	}
	go func() {
		err := fn(hub)
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
			}
			a.syncData()
		})
	}()
}
