package ui

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/projectsync"
	"github.com/enowx/enowx/hostconn"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Sync with another host brings a project's copy on the connected host
// and its copy on another to the same state, both ways
// (core/projectsync): both daemons scan their copy, the app compares them
// with what they had at the last sync, shows what goes each way and what
// changed on both, then each side packs what the other lacks.

// Steps of a sync.
const (
	syncComparing = "comparing"
	syncReview    = "review"
	syncRunning   = "running"
	syncDone      = "done"
)

// projectSyncForm is the Sync with another host dialog and the sync it runs. It
// belongs to the main thread; other is the other host's client, closed
// with the dialog.
type projectSyncForm struct {
	open    bool
	project proto.ProjectInfo
	target  hosts.Host
	folder  string // the project's folder there
	env     bool

	step   string
	doing  string
	signIn string // a link the other host gave while signing in: a Tailscale SSH check
	err    string
	other  *client.Client
	states [2]proto.ProjectSyncState
	plan   projectsync.Plan
	result [2]proto.ProjectSyncResult
	// link is the copies' link, kept on both hosts with its base; empty
	// until a sync makes it.
	link proto.ProjectLink
}

// syncStore keeps the pairs synced from this device.
func (a *App) syncStore() *projectsync.Store {
	if a.syncPairs == nil {
		dir, err := transport.Dir()
		if err != nil {
			dir = filepath.Join(".", transport.HomeName)
		}
		a.syncPairs = projectsync.NewStore(filepath.Join(dir, "projectsync.json"))
	}
	return a.syncPairs
}

// openProjectSync opens the dialog for project p, on the host it last synced
// with, else the first other host.
func (a *App) openProjectSync(p proto.ProjectInfo) {
	f := &projectSyncForm{open: true, project: p}
	partners := a.syncStore().Partners(projectsync.End{Host: a.host.ID, Path: p.Path})
	for _, h := range a.allHosts() {
		if h.ID == a.host.ID {
			continue
		}
		if f.target.ID == "" {
			f.target = h
		}
		if len(partners) > 0 && partners[0].Host == h.ID {
			f.target = h
			break
		}
	}
	f.folder = a.syncFolder(f, partners)
	a.projectSyncing = f
}

// syncFolder is the project's folder on f's target: where it was synced
// with before, else where Copy to another host puts it.
func (a *App) syncFolder(f *projectSyncForm, partners []projectsync.End) string {
	for _, e := range partners {
		if e.Host == f.target.ID {
			return e.Path
		}
	}
	return strings.TrimSuffix(a.copyParent(f.project), "/") + "/" + path.Base(f.project.Path)
}

func (a *App) projectSyncDialog(c *ui.Context) {
	f := a.projectSyncing
	if f == nil {
		return
	}
	if !f.open {
		if f.other != nil {
			f.other.Close()
		}
		a.projectSyncing = nil
		return
	}
	busy := (f.step == syncComparing || f.step == syncRunning) && f.err == ""
	widget.Sheet(c, &f.open, 560, func() {
		widget.SheetHead(c, &f.open, "Sync “"+f.project.Name+"” with another host",
			"Brings this copy and the other host's to the same state, both ways: files (what git keeps), commits, sessions and Claude Code's conversations. What changed on both sides since the last sync is yours to pick.")
	}, func() { a.projectSyncBody(c, f) }, func() { a.projectSyncFooter(c, f, busy) })
	if busy && f.step == syncRunning {
		f.open = true // the sync finishes before the dialog goes
	}
}

// projectSyncBody is the step the dialog is at.
func (a *App) projectSyncBody(c *ui.Context, f *projectSyncForm) {
	if f.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
	}
	switch f.step {
	case "", syncComparing:
		a.syncPick(c, f)
	case syncReview:
		a.syncReview(c, f)
	default:
		a.syncOutcome(c, f)
	}
}

// projectSyncFooter is what can be done at that step.
func (a *App) projectSyncFooter(c *ui.Context, f *projectSyncForm, busy bool) {
	switch {
	case busy:
		ui.Row(c.Key("busy")).Grow(1).Basis(0).MinWidth(0).Gap(theme.S2).Children(func() {
			widget.Spinner(c, 14)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Running(c, f.doing, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		})
		if link := f.signIn; link != "" && f.step == syncComparing {
			if widget.PrimaryButton(c, nil, "Open link", "").Clicked() {
				go func() { _ = mygo.Shell.OpenExternal(link) }()
			}
		}
		if f.step == syncComparing && widget.GhostButton(c, "Cancel").Clicked() {
			f.open = false // the connection still being made closes as it ends
		}
	case f.step == syncDone:
		ui.Spacer(c)
		if widget.PrimaryButton(c, nil, "Close", "").Clicked() {
			f.open = false
		}
	case f.step == syncReview:
		ui.Spacer(c)
		if widget.GhostButton(c, "Back").Clicked() {
			f.step, f.err = "", ""
		}
		if f.plan.Empty() {
			if widget.PrimaryButton(c, nil, "Close", "").Clicked() {
				f.open = false
			}
			return
		}
		if widget.PrimaryButton(c, nil, "Sync", "").Clicked() {
			a.runSync(f)
		}
	default:
		ui.Spacer(c)
		if widget.GhostButton(c, "Cancel").Clicked() {
			f.open = false
		}
		ok := f.target.ID != "" && strings.TrimSpace(f.folder) != ""
		if b := widget.PrimaryButton(c, nil, "Compare", "").Disabled(!ok); b.Clicked() && ok {
			a.compareSync(f)
		}
	}
}

// syncPick asks which host and which folder there.
func (a *App) syncPick(c *ui.Context, f *projectSyncForm) {
	widget.Field(c, "With", func() {
		if h, ok := a.otherHostPicker(c, f.target); ok && h.ID != f.target.ID {
			f.target = h
			f.folder = a.syncFolder(f, a.syncStore().Partners(projectsync.End{Host: a.host.ID, Path: f.project.Path}))
			if f.other != nil {
				f.other.Close()
				f.other = nil
			}
		}
	})
	widget.Field(c, "Its folder there", func() {
		widget.Input(c, &f.folder, "~/code/app").FillWidth()
		widget.CardNote(c.Key("about"), "The project's copy on "+f.target.Name+", from Copy to another host or cloned there.", 0)
	})
	ui.Column(c.Key("env")).FillWidth().Shrink(0).Gap(theme.S1).AlignItems(ui.Start).Children(func() {
		widget.Checkbox(c, &f.env, "Include .env files")
		if f.env {
			widget.CardNote(c, "They usually hold secrets (keys, passwords): they will go between both hosts.", 0).TextColor(theme.Warning)
		}
	})
}

// compareSync connects to f's target, scans both copies and plans the
// sync, off the main thread.
func (a *App) compareSync(f *projectSyncForm) {
	here := a.client
	if here == nil {
		return
	}
	f.step, f.err, f.doing, f.signIn = syncComparing, "", "Connecting to "+f.target.Name+"…", ""
	target, opts, folder, env, other := f.target, a.hostOptions(a.gen), strings.TrimSpace(f.folder), f.env, f.other
	opts.Progress = func(s string) { a.onMain(func() { f.doing = s + "…" }) }
	// Tailscale SSH waits for the user to check in a browser: its link shows.
	opts.Banner = func(msg string) error {
		a.onMain(func() { f.doing, f.signIn = strings.TrimSpace(msg), linkIn(msg) })
		return nil
	}
	project := f.project.Path
	go func() {
		fail := func(err error) {
			a.onMain(func() { f.step, f.err = "", client.Explain(err) })
		}
		if other == nil {
			c, err := hostconn.Connect(target, opts)
			if err != nil {
				fail(err)
				return
			}
			other = c
			a.onMain(func() {
				if f.open {
					f.other = c
				} else {
					c.Close()
				}
			})
		}
		a.onMain(func() { f.doing, f.signIn = "Comparing both copies…", "" })
		var states [2]proto.ProjectSyncState
		var err error
		if states[projectsync.Here], err = here.ScanProjectSync(project, env); err != nil {
			fail(err)
			return
		}
		if states[projectsync.There], err = other.ScanProjectSync(folder, env); err != nil {
			fail(err)
			return
		}
		link, base := findLink(here, other, states)
		if base == nil {
			// Synced before links kept their base: this device's record.
			base = a.syncStore().Base(projectsync.End{Host: a.host.ID, Path: states[0].Path}, projectsync.End{Host: target.ID, Path: states[1].Path})
		}
		plan := projectsync.Compare(base, states[0], states[1])
		a.onMain(func() { f.step, f.states, f.plan, f.link = syncReview, states, plan, link })
	}()
}

// syncReview shows what goes each way and what changed on both sides,
// each conflict's side to pick.
func (a *App) syncReview(c *ui.Context, f *projectSyncForm) {
	if f.plan.Empty() {
		ui.Text(c, "Both copies are the same.").FontSize(theme.TextUI)
		return
	}
	names := [2]string{a.host.Name, f.target.Name}
	ui.Scroll(c).FillWidth().MaxHeight(420).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.S3).Children(func() {
			for _, from := range []projectsync.Side{projectsync.There, projectsync.Here} {
				a.syncDirection(c.Key(fmt.Sprint(from)), f, from, names)
			}
			if n := f.plan.Conflicts(); n > 0 {
				a.syncConflicts(c, f, names)
			}
		})
	})
}

// syncDirection sums up what goes from one side to the other.
func (a *App) syncDirection(c *ui.Context, f *projectSyncForm, from projectsync.Side, names [2]string) {
	var lines, paths []string
	for _, x := range f.plan.Files {
		if x.From != from || x.Conflict {
			continue
		}
		p := x.Path
		if x.Root != "" {
			p = a.syncSessionName(f, x.Root) + ": " + p
		}
		if x.Delete {
			p += " (deleted)"
		}
		paths = append(paths, p)
	}
	if n := len(paths); n > 0 {
		lines = append(lines, textfmt.Plural(n, "A file", fmt.Sprintf("%d files", n))+":")
		const shown = 6
		for _, p := range paths[:min(n, shown)] {
			lines = append(lines, "    "+p)
		}
		if n > shown {
			lines = append(lines, fmt.Sprintf("    and %d more", n-shown))
		}
	}
	for _, b := range f.plan.Branches {
		if b.From == from {
			lines = append(lines, "Branch "+b.Name+" at "+shortCommit(b.Commit))
		}
	}
	for _, s := range f.plan.Sessions {
		if s.From != from || s.Conflict {
			continue
		}
		name := s.Name
		if name == "" {
			name = "a session"
		}
		switch {
		case s.New:
			lines = append(lines, "New session “"+name+"”")
		case s.Restarts:
			lines = append(lines, "“"+name+"”'s conversation; its agent on "+names[from.Other()]+" starts again on it")
		default:
			lines = append(lines, "“"+name+"”'s conversation")
		}
	}
	widget.Card(c).FillWidth().Shrink(0).Gap(theme.S1).Children(func() {
		ui.Row(c.Key("way")).FillWidth().Gap(theme.S2).Children(func() {
			ui.Text(c, names[from]).FontSize(theme.TextUI).FontWeight(500).SingleLine().Shrink(1)
			ui.Icon(c, icon.ArrowRight).FontSize(12).TextColor(theme.TextSoft)
			ui.Text(c, names[from.Other()]).FontSize(theme.TextUI).FontWeight(500).SingleLine().Shrink(1)
		})
		if len(lines) == 0 {
			widget.CardNote(c.Key("nothing"), "Nothing", 0)
		}
		for i, l := range lines {
			widget.CardNote(c.Key(fmt.Sprint(i)), l, 0)
		}
	})
}

// syncConflicts lists what changed on both sides, each with the side
// whose version wins.
func (a *App) syncConflicts(c *ui.Context, f *projectSyncForm, names [2]string) {
	options := []string{names[projectsync.Here], names[projectsync.There]}
	widget.Card(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Text(c, "Changed on both: keep whose?").FontSize(theme.TextUI).FontWeight(500)
		ui.Row(c.Key("all-row")).Gap(theme.S2).Children(func() {
			ui.Text(c, "All").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			if i := widget.Segmented(c.Key("all"), options, -1, nil); i >= 0 {
				for k := range f.plan.Files {
					if f.plan.Files[k].Conflict {
						f.plan.Files[k].From = projectsync.Side(i)
					}
				}
				for k := range f.plan.Sessions {
					if f.plan.Sessions[k].Conflict {
						f.plan.Sessions[k].From = projectsync.Side(i)
					}
				}
			}
		})
		for k := range f.plan.Files {
			x := &f.plan.Files[k]
			if !x.Conflict {
				continue
			}
			label := x.Path
			if x.Root != "" {
				label = a.syncSessionName(f, x.Root) + ": " + x.Path
			}
			a.syncConflictRow(c.Key("f"+x.Root+"\x00"+x.Path), label, options, &x.From)
		}
		for k := range f.plan.Sessions {
			s := &f.plan.Sessions[k]
			if s.Conflict {
				a.syncConflictRow(c.Key("s"+s.Key), "“"+s.Name+"”'s conversation", options, &s.From)
			}
		}
	})
}

func (a *App) syncConflictRow(c *ui.Context, label string, options []string, from *projectsync.Side) {
	ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Row(c.Key("label")).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, label, widget.RunStyle{Size: theme.TextLabel}) })
		if i := widget.Segmented(c, options, int(*from), nil); i >= 0 {
			*from = projectsync.Side(i)
		}
	})
}

// syncSessionName names the session of key, for a worktree's file.
func (a *App) syncSessionName(f *projectSyncForm, key string) string {
	for _, st := range f.states {
		for _, s := range st.Sessions {
			if s.Key == key && s.Name != "" {
				return s.Name
			}
		}
	}
	return "a worktree"
}

func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// runSync runs f's plan, off the main thread, and keeps the base for the
// next sync.
func (a *App) runSync(f *projectSyncForm) {
	here, other := a.client, f.other
	if here == nil || other == nil {
		return
	}
	f.step, f.err, f.doing = syncRunning, "", "Syncing…"
	names := [2]string{a.host.Name, f.target.Name}
	clients := [2]*client.Client{here, other}
	link := f.link
	// This device remembers which host and folder it synced with, to offer
	// them again.
	ends := [2]projectsync.End{{Host: a.host.ID, Path: f.states[0].Path}, {Host: f.target.ID, Path: f.states[1].Path}}
	run := projectsync.Run{
		Daemons: [2]projectsync.Daemon{here, other}, Names: names, States: f.states, Plan: f.plan, Env: f.env,
		Carry: func(from projectsync.Side, id string, size int64) error {
			return client.CopyTransfer(clients[from], clients[from.Other()], id, size, nil)
		},
		Step: func(from projectsync.Side, what string) {
			doing := map[string]string{
				"packing":  "Packing on " + names[from] + "…",
				"carrying": "Carrying to " + names[from.Other()] + "…",
				"applying": "Applying on " + names[from.Other()] + "…",
			}[what]
			a.onMain(func() { f.doing = doing })
		},
	}
	go func() {
		res, base, err := run.Do()
		if err == nil {
			err = keepLink(here, other, names, run.States, link, base)
		}
		if err == nil {
			err = a.syncStore().Save(ends[0], ends[1], base)
		}
		a.onMain(func() {
			f.result = res
			if err != nil {
				f.step, f.err = syncReview, client.Explain(err)
				return
			}
			f.step = syncDone
		})
	}()
}

// syncOutcome shows the sync under way, then what it did.
func (a *App) syncOutcome(c *ui.Context, f *projectSyncForm) {
	if f.step != syncDone {
		return
	}
	ui.Text(c, "Both copies are the same now.").FontSize(theme.TextUI)
	names := [2]string{a.host.Name, f.target.Name}
	for side, r := range f.result {
		for i, n := range r.Notes {
			widget.CardNote(c.Key(fmt.Sprint(side, i)), names[side]+": "+n, 0)
		}
	}
}
