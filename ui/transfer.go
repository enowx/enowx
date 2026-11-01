package ui

import (
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/hostconn"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Copy to another host copies a project, and the sessions picked, from the
// connected host to another (core/transfer): its daemon packs them, the
// app connects to the other host beside this one and carries the archive
// across, and the other's daemon unpacks it and starts the sessions again.

// Steps of a copy.
const (
	copyPacking   = "packing"
	copyConnect   = "connecting"
	copyCarrying  = "carrying"
	copyUnpacking = "unpacking"
	copyDone      = "done"
)

// copyForm is the Copy to another host dialog and the copy it runs. It
// belongs to the main thread.
type copyForm struct {
	open    bool
	project proto.ProjectInfo
	target  hosts.Host
	parent  string          // the folder there the project's folder goes into
	picked  map[string]bool // sessions, by ID
	env     bool

	step        string
	sent, total int64
	err         string
	result      proto.TransferResult
}

// openCopy opens the dialog for project p, every session picked.
func (a *App) openCopy(p proto.ProjectInfo) {
	f := &copyForm{open: true, project: p, picked: map[string]bool{}}
	for _, s := range a.projectSessions(p.Path) {
		f.picked[s.ID] = true
	}
	for _, h := range a.allHosts() {
		if h.ID != a.host.ID {
			f.target = h
			break
		}
	}
	f.parent = a.copyParent(p)
	a.copying = f
}

// copyParent is the folder p's folder goes into on the other host by
// default: the same place under its home folder, else the home folder.
func (a *App) copyParent(p proto.ProjectInfo) string {
	if home := a.daemonInfo.Home; home != "" {
		if rel, ok := strings.CutPrefix(path.Dir(p.Path), strings.TrimSuffix(home, "/")); ok {
			return "~" + rel + "/"
		}
	}
	return "~/"
}

// dest is where the project's folder lands: in parent, by its name.
func (f *copyForm) dest() string {
	parent := strings.TrimSpace(f.parent)
	if parent == "" {
		return ""
	}
	return strings.TrimSuffix(parent, "/") + "/" + path.Base(f.project.Path)
}

func (a *App) copyDialog(c *ui.Context) {
	f := a.copying
	if f == nil {
		return
	}
	if !f.open {
		a.copying = nil
		return
	}
	busy := f.step != "" && f.step != copyDone && f.err == ""
	widget.Dialog(c, &f.open, 520, 14, func() {
		widget.DialogHeader(c, "Copy “"+f.project.Name+"” to another host",
			"Its files (what git keeps, without dependencies such as node_modules) and the sessions picked, each starting again there: Claude Code on its own conversation, a worktree with its work. This host keeps everything.", false)
		if f.step == "" || f.err != "" {
			a.copyForm(c, f)
		} else {
			a.copyProgress(c, f)
		}
		widget.DialogFooter(c, "", func() {
			switch {
			case f.step == copyDone:
				if widget.GhostButton(c, "Close").Clicked() {
					f.open = false
				}
				if widget.PrimaryButton(c, icon.ArrowRight, "Open on "+f.target.Name, "").Clicked() {
					a.openProject = f.result.Project.Path
					f.open = false
					a.connect(f.target)
				}
			case busy:
				ui.Row(c).Gap(8).Children(func() {
					widget.Spinner(c, 14)
					ui.Text(c, "Copying…").FontSize(theme.TextLabel).TextColor(theme.Muted)
				})
			default:
				if widget.GhostButton(c, "Cancel").Clicked() {
					f.open = false
				}
				ok := f.target.ID != "" && f.dest() != ""
				b := widget.PrimaryButton(c, icon.Server, "Copy", "").Disabled(!ok)
				if b.Clicked() && ok {
					a.startCopy(f)
				}
			}
		})
	})
	if busy {
		f.open = true // the copy finishes before the dialog goes
	}
}

// copyForm asks where to, which sessions, and whether .env files go.
func (a *App) copyForm(c *ui.Context, f *copyForm) {
	if f.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
	}
	widget.Field(c, "To", func() {
		if h, ok := a.otherHostPicker(c, f.target); ok {
			f.target = h
		}
	})
	widget.Field(c, "Into the folder", func() {
		ui.Column(c).FillWidth().Gap(5).Children(func() {
			widget.Input(c, &f.parent, "~/").FillWidth()
			if d := f.dest(); d != "" {
				ui.Text(c, "The folder “"+path.Base(f.project.Path)+"” goes there whole: "+d).FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
		})
	})
	if sessions := a.projectSessions(f.project.Path); len(sessions) > 0 {
		widget.Field(c, "Sessions", func() {
			ui.Column(c).FillWidth().Gap(4).AlignItems(ui.Start).Children(func() {
				for _, s := range sessions {
					on := f.picked[s.ID]
					label := a.sessionName(s) + " · " + a.agentName(s.Agent)
					if tabs := len(a.tabsOf(s.ID)); tabs > 0 {
						label += " · " + textfmt.Plural(tabs, "a tab", "tabs")
					}
					if widget.Checkbox(c.Key(s.ID), &on, label) {
						f.picked[s.ID] = on
					}
				}
			})
		})
	}
	ui.Column(c).FillWidth().Gap(4).AlignItems(ui.Start).Children(func() {
		widget.Checkbox(c, &f.env, "Include .env files")
		if f.env {
			ui.Text(c, "They usually hold secrets (keys, passwords): they will be on "+f.target.Name+" too.").
				FontSize(11.5).TextColor(theme.Warning)
		}
	})
}

// otherHostPicker lists the hosts but the connected one, picked marked;
// it returns the one clicked.
func (a *App) otherHostPicker(c *ui.Context, picked hosts.Host) (hosts.Host, bool) {
	var clicked hosts.Host
	ok := false
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		for _, h := range a.allHosts() {
			if h.ID == a.host.ID {
				continue
			}
			row := ui.ButtonBase(c.Key(h.ID)).FillWidth().Height(36).PaddingX(10).Gap(10).Radius(theme.RadiusControl).
				Label(h.Name).Cursor(ui.CursorPointer).Border(1, theme.Border).Justify(ui.Start)
			if picked.ID == h.ID {
				row.Border(1, theme.AccentLine).Background(theme.Selected)
			}
			row.Children(func() {
				ui.Icon(c, hostIcon(h)).FontSize(14)
				ui.Text(c, h.Name).FontSize(theme.TextUI).SingleLine()
				ui.Text(c, hostDetail(h)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
			})
			if row.Clicked() {
				clicked, ok = h, true
			}
		}
	})
	return clicked, ok
}

// copyProgress shows the copy under way, then what it became.
func (a *App) copyProgress(c *ui.Context, f *copyForm) {
	steps := []struct{ id, label string }{
		{copyPacking, "Packing on " + a.host.Name},
		{copyConnect, "Connecting to " + f.target.Name},
		{copyCarrying, "Copying"},
		{copyUnpacking, "Starting the sessions on " + f.target.Name},
	}
	reached := true
	for _, st := range steps {
		current := st.id == f.step
		ui.Row(c.Key(st.id)).FillWidth().Gap(10).Children(func() {
			switch {
			case current:
				widget.Spinner(c, 14)
			case reached:
				ui.Icon(c, icon.Check).FontSize(13).TextColor(theme.Done)
			default:
				widget.Dot(c, theme.Idle, true, 7)
			}
			label := st.label
			if st.id == copyCarrying && f.total > 0 {
				label += fmt.Sprintf(" · %s of %s", textfmt.Bytes(uint64(f.sent)), textfmt.Bytes(uint64(f.total)))
			}
			col := theme.Text
			if !reached || current {
				col = theme.TextSoft
			}
			ui.Text(c, label).FontSize(theme.TextUI).TextColor(col)
		})
		if current {
			reached = false
		}
	}
	if f.step != copyDone {
		return
	}
	r := f.result
	ui.Text(c, fmt.Sprintf("“%s” is on %s at %s, with %s.", f.project.Name, f.target.Name, r.Project.Path,
		textfmt.Plural(len(r.Sessions), "a session", fmt.Sprintf("%d sessions", len(r.Sessions))))).FontSize(theme.TextUI)
	for _, n := range r.Notes {
		ui.Text(c, n).FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
}

// startCopy runs the copy f asks for, off the main thread.
func (a *App) startCopy(f *copyForm) {
	src := a.client
	if src == nil {
		return
	}
	var sessions []string
	for _, s := range a.projectSessions(f.project.Path) {
		if f.picked[s.ID] {
			sessions = append(sessions, s.ID)
		}
	}
	if sessions == nil {
		sessions = []string{} // none, not all
	}
	params := proto.TransferPackParams{Project: f.project.Path, Sessions: sessions, Env: f.env}
	target, dest, opts, srcName := f.target, f.dest(), a.hostOptions(a.gen), a.host.Name
	f.step, f.err, f.sent, f.total = copyPacking, "", 0, 0
	set := func(fn func()) { a.onMain(fn) }
	fail := func(err error) { set(func() { f.err = client.Explain(err) }) }
	go func() {
		info, err := src.PackTransfer(params)
		if err != nil {
			fail(err)
			return
		}
		defer func() { _ = src.DiscardTransfer(info.ID) }()
		set(func() { f.step, f.total = copyConnect, info.Size })
		dst, err := hostconn.Connect(target, opts)
		if err != nil {
			fail(err)
			return
		}
		defer dst.Close()
		set(func() { f.step = copyCarrying })
		err = client.CopyTransfer(src, dst, info.ID, info.Size, func(sent int64) { set(func() { f.sent = sent }) })
		if err != nil {
			_ = dst.DiscardTransfer(info.ID)
			fail(err)
			return
		}
		set(func() { f.step = copyUnpacking })
		res, err := dst.UnpackTransfer(info.ID, dest)
		if err != nil {
			_ = dst.DiscardTransfer(info.ID)
			fail(err)
			return
		}
		// The copy remembers where it came from, for syncs later.
		if l, err := recordCopy(src, srcName, params.Project, dst, target.Name, res.Project.Path); err != nil {
			res.Notes = append(res.Notes, "Not linked to this copy for syncs: "+client.Explain(err))
		} else {
			set(func() { a.mergeLinks([]proto.ProjectLinkState{{ProjectLink: l}}) })
		}
		set(func() { f.step, f.result = copyDone, res })
	}()
}
