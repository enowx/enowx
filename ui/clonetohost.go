package ui

import (
	"strconv"
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

// Clone on another host clones a project's repository on another host of
// the user's, from its Git remote: the same project there, its notes
// found by the repository, its files the host's own (not copied).

// cloneToForm is the Clone on another host dialog and the clone it runs,
// the main thread's.
type cloneToForm struct {
	open    bool
	project proto.ProjectInfo
	remote  string // the project's origin, "" until read
	looking bool
	target  hosts.Host
	dir     string
	running bool
	status  proto.CloneStatus
	err     string
}

// openCloneTo opens the dialog for project p, reading its remote.
func (a *App) openCloneTo(p proto.ProjectInfo) {
	f := &cloneToForm{open: true, project: p, dir: "~/code", looking: true}
	for _, h := range a.allHosts() {
		if h.ID != a.host.ID {
			f.target = h
			break
		}
	}
	a.cloningTo = f
	cl, id := a.client, a.daemonInfo.ID
	if cl == nil {
		return
	}
	go func() {
		cats, err := cl.Catalogs()
		prefs, _ := cl.ClonePrefs()
		a.onMain(func() {
			f.looking = false
			if prefs.Default != "" {
				f.dir = prefs.Default
			}
			if err != nil {
				f.err = client.Explain(err)
				return
			}
			for _, cat := range cats {
				if cat.Host != id && id != "" {
					continue
				}
				for _, cp := range cat.Projects {
					if cp.Path == p.Path {
						f.remote = cp.Remote
					}
				}
			}
			if f.remote == "" {
				f.err = p.Name + " has no Git remote to clone from: push it somewhere first, or copy it to the other host"
			}
		})
	}()
}

// cloneToDialog draws the dialog while it is open.
func (a *App) cloneToDialog(c *ui.Context) {
	f := a.cloningTo
	if f == nil {
		return
	}
	if !f.open {
		a.cloningTo = nil
		return
	}
	widget.Sheet(c, &f.open, 480, func() {
		widget.SheetHead(c, &f.open, "Clone “"+f.project.Name+"” on another host", "Its repository cloned there from its remote, as a project of that host: the same project for its notes and agents, its files that host's own.")
	}, func() {
		if f.remote != "" {
			widget.Running(c.Key("remote"), f.remote, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft, Mono: true})
		}
		widget.Field(c, "On", func() {
			if h, ok := a.otherHostPicker(c, f.target); ok && !f.running {
				f.target = h
			}
		})
		widget.Field(c, "Into the folder", func() {
			widget.Input(c, &f.dir, "~/code").FillWidth()
		})
		switch {
		case f.err != "":
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		case f.status.State == proto.CloneDone:
			widget.StatusLine(c.Key("done"), theme.Done, false, "Cloned on "+f.target.Name+" into "+f.status.Path)
		case f.running:
			line := "Connecting to " + f.target.Name + "…"
			if f.status.Phase != "" {
				line = f.status.Phase
				if f.status.Percent > 0 {
					line += " " + strconv.Itoa(f.status.Percent) + "%"
				}
			}
			ui.Row(c.Key("running")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
				widget.Spinner(c, 12)
				ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, line, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft}) })
			})
		}
	}, func() {
		ui.Spacer(c)
		if widget.GhostButton(c, "Close").Clicked() {
			f.open = false
		}
		b := widget.PrimaryButton(c, nil, "Clone there", "")
		if f.remote == "" || f.target.ID == "" || f.running || f.status.State == proto.CloneDone {
			b.Disabled(true)
		} else if b.Clicked() {
			a.runCloneTo(f)
		}
	})
}

// runCloneTo clones f's repository on its host, off the main thread.
func (a *App) runCloneTo(f *cloneToForm) {
	f.running, f.err, f.status = true, "", proto.CloneStatus{}
	target, opts := f.target, a.hostOptions(a.gen)
	params := proto.ProjectCloneParams{URL: f.remote, Dir: f.dir}
	go func() {
		fail := func(err error) { a.onMain(func() { f.running, f.err = false, client.Explain(err) }) }
		dst, err := hostconn.Connect(target, opts)
		if err != nil {
			fail(err)
			return
		}
		defer dst.Close()
		st, err := dst.CloneProject(params)
		if err != nil {
			fail(err)
			return
		}
		for st.State == proto.CloneRunning {
			time.Sleep(500 * time.Millisecond)
			if st, err = dst.CloneStatus(st.ID); err != nil {
				fail(err)
				return
			}
			s := st
			a.onMain(func() { f.status = s })
		}
		a.onMain(func() {
			f.running, f.status = false, st
			if st.State != proto.CloneDone {
				f.err = st.Error
			}
		})
	}()
}
