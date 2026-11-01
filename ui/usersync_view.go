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
	sectionIntro(c, "Sync", "Your notes, their groups, canvases, settings and the vault's encrypted data are the same on every host. This device syncs each host it reaches with its own, when it connects, every 30 seconds and right after a change. Project files are not synced here.")
	summary, alarm := a.syncSummary()
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			col := theme.Done
			if alarm {
				col = theme.Warning
			}
			widget.Dot(c, col, a.dsync.running, 8)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, summary).FontSize(theme.TextUI).TextColor(theme.TextStrong)
				st := a.dsync.hub
				if st.ID != "" {
					ui.Text(c, fmt.Sprintf("This device: %s, %s", textfmt.Plural(st.Files, "1 file", fmt.Sprintf("%d files", st.Files)), textfmt.Bytes(uint64(st.Size)))).
						FontSize(11.5).TextColor(theme.Muted)
				}
			})
			if widget.Button(c, widget.Secondary, icon.Sync, "Sync now", "").Clicked() {
				a.syncData()
			}
		})
	})
	widget.Field(c, "Hosts", func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			any := false
			for _, h := range a.allHosts() {
				if h.ID == hosts.LocalID {
					continue
				}
				any = true
				a.syncHostRow(c.Key(h.ID), h)
			}
			if !any {
				ui.Text(c, "No other host yet: add a server or a computer in Hosts, and its notes join these.").FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
		})
	})
	if len(a.dsync.conflicts) > 0 {
		widget.Field(c, "Conflicts", func() {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				for _, cf := range a.dsync.conflicts {
					a.conflictRow(c.Key(cf.ID), cf)
				}
			})
		})
	}
	widget.Field(c, "Recently deleted", func() {
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			if len(a.dsync.trash) == 0 {
				ui.Text(c, fmt.Sprintf("Nothing. What's deleted, here or on another host, is kept %d days.", 30)).FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
			for i, it := range a.dsync.trash {
				if i == 20 {
					ui.Text(c, fmt.Sprintf("and %d more", len(a.dsync.trash)-20)).FontSize(theme.TextLabel).TextColor(theme.Muted)
					break
				}
				ui.Row(c.Key(it.ID)).FillWidth().Height(32).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icon.Trash).FontSize(12).TextColor(theme.Muted)
					name := it.Title
					if name == "" {
						name = it.Path
					}
					ui.Text(c, name).FontSize(theme.TextUI).TextColor(theme.Text).SingleLine().Shrink(1)
					ui.Text(c, textfmt.Age(it.Deleted, time.Now())).FontSize(theme.TextLabel).TextColor(theme.Muted)
					ui.Spacer(c)
					if widget.GhostButton(c, "Restore").Clicked() {
						a.syncCall(func(hub *client.Client) error { return hub.SyncRestore(it.ID) })
					}
				})
			}
		})
	})
}

// syncHostRow is how syncing host h goes.
func (a *App) syncHostRow(c *ui.Context, h hosts.Host) {
	st, ok := a.dsync.hosts[h.ID]
	state, about, col := syncOffline, "Not connected: syncs when it is", ui.Color{}
	if ok {
		state, col = st.state, theme.Done
		switch st.state {
		case syncIdle:
			about = "Synced " + textfmt.Age(st.last, time.Now())
			if r := st.rep; r.Sent+r.Received+r.Merged > 0 {
				about += fmt.Sprintf(" · %d sent, %d received, %d merged", r.Sent, r.Received, r.Merged)
			}
			if n := len(st.rep.Skipped); n > 0 {
				about += " · " + textfmt.Plural(n, "1 file", fmt.Sprintf("%d files", n)) + " too large to sync"
			}
		case syncSyncing:
			about = "Syncing…"
		default:
			about, col = st.err, theme.Warning
		}
	}
	ui.Row(c).FillWidth().Padding(10).Gap(10).Radius(theme.RadiusControl).Border(1, theme.Border).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon.Server).FontSize(14).TextColor(theme.TextSoft)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, h.Name).FontSize(theme.TextUI).TextColor(theme.Text)
			ui.Text(c, about).FontSize(theme.TextLabel).TextColor(theme.Muted).Shrink(1)
		})
		widget.Dot(c, col, col == (ui.Color{}), 7)
		ui.Text(c, state).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	})
}

// conflictRow is a file changed on two hosts in ways that don't merge,
// with a button per version to keep, and one to keep them all.
func (a *App) conflictRow(c *ui.Context, cf proto.SyncConflict) {
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Alert).FontSize(13).TextColor(theme.Warning)
			name := cf.Title
			if name == "" {
				name = cf.Path
			}
			ui.Text(c, name).FontSize(theme.TextUI).TextColor(theme.TextStrong).SingleLine().Shrink(1)
			ui.Spacer(c)
			ui.Text(c, "changed on two hosts "+textfmt.Age(cf.Created, time.Now())).FontSize(theme.TextLabel).TextColor(theme.Muted)
		})
		ui.Text(c, "Both changed the same lines. The first version is in place until you pick.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
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
