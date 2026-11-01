package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The Sync & push dialog of a project: where it came from and where it
// was copied, each copy's sync on its own (when, which way, what), its
// conflicts to settle, the project's auto pushes, and what syncs and
// pushes did.

// Tabs of the dialog.
const (
	linksTabCopies = iota
	linksTabPush
	linksTabHistory
)

// linksForm is the dialog's state. It belongs to the main thread.
type linksForm struct {
	open    bool
	project proto.ProjectInfo
	id      string // the project's ID
	tab     int
	err     string
	// editing is the link whose settings show, with their fields' text.
	editing  string
	draft    proto.ProjectLink
	fields   linkFields
	push     proto.AutopushStatus
	pushText pushFields
	pushRead bool
	history  []proto.ProjectHistoryEntry
	busy     map[string]string // what runs, by link ID or "push"
	diff     *conflictDiff
}

// openLinks opens the dialog of project p.
func (a *App) openLinks(p proto.ProjectInfo) {
	cl := a.client
	if cl == nil {
		return
	}
	f := &linksForm{open: true, project: p, busy: map[string]string{}}
	a.linksForm = f
	a.readLinks()
	go func() {
		id, err := cl.ProjectID(p.Path)
		if err != nil {
			a.onMain(func() { f.err = client.Explain(err) })
			return
		}
		pushes, _ := cl.Autopushes()
		history, _ := cl.ProjectHistory(id.ID)
		a.onMain(func() {
			f.id, f.history = id.ID, history
			f.push = proto.AutopushStatus{AutopushSettings: proto.AutopushSettings{Project: id.ID, Name: p.Name}}
			for _, st := range pushes {
				if st.Project == id.ID {
					f.push = st
				}
			}
			f.pushText = pushFieldsOf(f.push)
			f.pushRead = true
		})
	}()
}

// projectLinks are the links of the project with ID id, oldest first.
func (a *App) projectLinks(id string) []proto.ProjectLinkState {
	var out []proto.ProjectLinkState
	for _, l := range a.sortedLinks() {
		if id != "" && l.Project == id {
			out = append(out, l)
		}
	}
	return out
}

func (a *App) linksDialog(c *ui.Context) {
	f := a.linksForm
	if f == nil {
		return
	}
	if !f.open {
		a.linksForm = nil
		return
	}
	if f.diff != nil {
		a.conflictDiffDialog(c, f)
		return
	}
	widget.Sheet(c, &f.open, 640, func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, "Sync & push “"+f.project.Name+"”").FontSize(15).TextColor(theme.TextStrong)
			ui.Text(c, a.originLine(f)).FontSize(11.5).TextColor(theme.Muted)
		})
		if widget.IconButton(c, icon.Close, "Close", false).Clicked() {
			f.open = false
		}
	}, func() {
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
		if i := widget.Segmented(c, []string{"Copies", "Autopush", "History"}, f.tab, nil); i >= 0 {
			f.tab = i
		}
		switch f.tab {
		case linksTabCopies:
			a.linksCopies(c, f)
		case linksTabPush:
			a.linksPush(c, f)
		default:
			a.linksHistory(c, f)
		}
	}, func() {
		ui.Spacer(c)
		if f.tab == linksTabCopies && widget.GhostButton(c, "Copy to another host…").Clicked() {
			f.open = false
			a.openCopy(f.project)
		}
		if widget.PrimaryButton(c, icon.Check, "Done", "").Clicked() {
			f.open = false
		}
	})
}

// originLine says where the project came from and where it went.
func (a *App) originLine(f *linksForm) string {
	here := a.daemonInfo.ID
	var from string
	var to []string
	for _, l := range a.projectLinks(f.id) {
		switch here {
		case l.Target.Daemon:
			from = l.Source.Host + " · " + textfmt.ShortHome(l.Source.Path)
		case l.Source.Daemon:
			to = append(to, l.Target.Host)
		}
	}
	var parts []string
	if from != "" {
		parts = append(parts, "From "+from)
	}
	if len(to) > 0 {
		parts = append(parts, "Copied to "+strings.Join(to, ", "))
	}
	if len(parts) == 0 {
		return "On " + a.host.Name + " only, for now"
	}
	return strings.Join(parts, " · ")
}

// linksCopies lists the project's copies, each with its sync.
func (a *App) linksCopies(c *ui.Context, f *linksForm) {
	links := a.projectLinks(f.id)
	if len(links) == 0 {
		widget.EmptyState(c, icon.Server, "Not on another host yet",
			"Copy it to another host: the copy remembers it came from here, and both can then sync on their own.", nil)
		return
	}
	for _, l := range links {
		a.linkCard(c.Key(l.ID), f, l)
	}
}

// linkCard is one copy: which hosts, how it syncs, how it went.
func (a *App) linkCard(c *ui.Context, f *linksForm, l proto.ProjectLinkState) {
	_, srcOn := a.reach(l.Source.Daemon)
	_, dstOn := a.reach(l.Target.Daemon)
	widget.Card(c).FillWidth().Gap(10).Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Server).FontSize(15).TextColor(theme.Muted)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					endLabel(c, l.Source, srcOn)
					ui.Icon(c, directionIcon(l.Direction)).FontSize(12).TextColor(theme.Muted)
					endLabel(c, l.Target, dstOn)
				})
				ui.Text(c, textfmt.ShortHome(l.Source.Path)+"  →  "+textfmt.ShortHome(l.Target.Path)).
					FontSize(11).TextColor(theme.Muted).SingleLine()
			})
			auto := l.Auto
			if widget.Switch(c, &auto, "Sync on its own") {
				d := l.ProjectLink
				d.Auto = auto
				a.saveLink(f, d)
			}
		})
		ui.Text(c, a.linkStatus(l, srcOn && dstOn)).FontSize(11.5).TextColor(statusColor(l))
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			doing := a.links.running[l.ID]
			b := widget.Button(c, widget.Secondary, icon.Sync, "Sync now", "").Disabled(!srcOn || !dstOn || doing != "")
			if b.Clicked() && srcOn && dstOn && doing == "" {
				a.syncLinkNow(l.ID, func(_ proto.ProjectLinkState, err error) {
					if err != nil {
						f.err = client.Explain(err)
					}
				})
			}
			if doing != "" {
				widget.Spinner(c, 13)
			}
			ui.Spacer(c)
			label := "Settings"
			if f.editing == l.ID {
				label = "Hide settings"
			}
			if widget.GhostButton(c, label).Clicked() {
				if f.editing == l.ID {
					f.editing = ""
				} else {
					f.editing, f.draft, f.fields = l.ID, l.ProjectLink, linkFieldsOf(l.ProjectLink)
				}
			}
			widget.MoreMenu(c, &a.menu, "link:"+l.ID, func(done func()) {
				other := l.Target
				if widget.MenuItem(c, icon.Sync, "Make "+other.Host+" the source", false) {
					d := l.ProjectLink
					d.Source, d.Target = d.Target, d.Source
					a.saveLink(f, d)
					done()
				}
				if widget.MenuItemTip(c, icon.Trash, "Unlink", "Stop syncing them; both copies stay", true) {
					a.askUnlink(f, l.ProjectLink)
					done()
				}
			})
		})
		if f.editing == l.ID {
			a.linkSettings(c, f)
		}
		if len(l.Status.Conflicts) > 0 {
			a.linkConflicts(c, f, l)
		}
	})
}

func endLabel(c *ui.Context, e proto.ProjectEnd, online bool) {
	col := theme.Done
	if !online {
		col = theme.Idle
	}
	widget.Dot(c, col, !online, 6)
	ui.Text(c, e.Host).FontSize(12.5).FontWeight(600).SingleLine().TextColor(theme.Text)
}

func directionIcon(dir string) *ui.SVG {
	switch dir {
	case proto.SyncOneWay:
		return icon.ArrowRight
	case proto.SyncReverse:
		return icon.ArrowLeft
	}
	return icon.Sync
}

func statusColor(l proto.ProjectLinkState) ui.Color {
	switch {
	case l.Status.Err != "":
		return theme.Error
	case len(l.Status.Conflicts) > 0:
		return theme.Warning
	}
	return theme.TextSoft
}

// linkStatus says how link l syncs and how it went.
func (a *App) linkStatus(l proto.ProjectLinkState, online bool) string {
	now := time.Now()
	var parts []string
	switch {
	case !l.Auto:
		parts = append(parts, "Syncs when you say")
	case l.Realtime:
		parts = append(parts, "Syncs as files change")
	default:
		every := defaultLinkInterval
		if l.Interval > 0 {
			every = time.Duration(l.Interval) * time.Second
		}
		parts = append(parts, "Syncs every "+formatEvery(every))
	}
	switch {
	case l.Status.Last.IsZero():
		parts = append(parts, "never synced")
	case l.Status.Err != "":
		parts = append(parts, "failed "+textfmt.Age(l.Status.Last, now)+": "+l.Status.Err)
	default:
		parts = append(parts, "last "+textfmt.Age(l.Status.Last, now))
	}
	if l.Auto && !l.Realtime && online && !l.Status.Last.IsZero() {
		every := defaultLinkInterval
		if l.Interval > 0 {
			every = time.Duration(l.Interval) * time.Second
		}
		parts = append(parts, "next "+textfmt.Until(l.Status.Last.Add(every), now))
	}
	if !online {
		parts = append(parts, "paused: a host is offline")
	}
	if n := len(l.Status.Conflicts); n > 0 {
		parts = append(parts, fmt.Sprintf("%d in conflict", n))
	}
	return strings.Join(parts, " · ")
}

// saveLink records l's settings on the hosts reached of both its copies
// (the user's data takes them to the others).
func (a *App) saveLink(f *linksForm, l proto.ProjectLink) {
	var clients []*client.Client
	for _, id := range []string{l.Source.Daemon, l.Target.Daemon} {
		if h, ok := a.reach(id); ok && !slices.Contains(clients, h.cl) {
			clients = append(clients, h.cl)
		}
	}
	if len(clients) == 0 {
		f.err = "Neither host of this link is online"
		return
	}
	if st, ok := a.links.links[l.ID]; ok {
		st.ProjectLink = l
		a.links.links[l.ID] = st
	}
	go func() {
		for _, cl := range clients {
			if _, err := cl.SaveProjectLink(l); err != nil {
				a.onMain(func() { f.err = client.Explain(err) })
				return
			}
		}
		a.onMain(func() { f.err = "" })
	}()
}

// unlink forgets l on the hosts reached; both copies stay.
// askUnlink unlinks l once the user agrees.
func (a *App) askUnlink(f *linksForm, l proto.ProjectLink) {
	a.confirm("Unlink these copies?", "They stop syncing with each other, on their own and when asked. Both copies stay as they are on "+l.Source.Host+" and "+l.Target.Host+".", "Unlink", func() { a.unlink(f, l) })
}

func (a *App) unlink(f *linksForm, l proto.ProjectLink) {
	delete(a.links.links, l.ID)
	for _, id := range []string{l.Source.Daemon, l.Target.Daemon} {
		if h, ok := a.reach(id); ok {
			go func() {
				if err := h.cl.DeleteProjectLink(l.ID); err != nil {
					a.onMain(func() { f.err = client.Explain(err) })
				}
			}()
		}
	}
}

// linksHistory lists what syncs and pushes did, the latest first.
func (a *App) linksHistory(c *ui.Context, f *linksForm) {
	if len(f.history) == 0 {
		widget.EmptyState(c, icon.List, "Nothing yet", "Syncs and pushes show here, done or failed, with why.", nil)
		return
	}
	now := time.Now()
	for i, e := range f.history {
		ui.Row(c.Key(i)).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
			svg, col := icon.Sync, theme.Done
			if e.Kind == "push" {
				svg = icon.ArrowRight
			}
			if !e.OK {
				col = theme.Error
			}
			ui.Icon(c, svg).FontSize(13).TextColor(col)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				label := map[string]string{"sync": "Sync", "push": "Push"}[e.Kind] + " · " + e.Summary
				ui.Text(c, label).FontSize(12).SingleLine().TextColor(theme.Text)
				if e.Err != "" {
					ui.Text(c, e.Err).FontSize(11).TextColor(theme.Error)
				}
			})
			ui.Text(c, textfmt.Age(e.Time, now)).FontSize(11).TextColor(theme.Muted).SingleLine()
		})
	}
}
