package ui

import (
	"encoding/json"
	"path"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/gitclone"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/folderpicker"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Clone from Git: a repository's address, the folder of the host it goes
// in, then the host's daemon clones it with the host's credentials and
// adds it as a project, its progress shown as it goes.

// cloneForm is the Clone from Git dialog.
type cloneForm struct {
	open       bool
	url        string
	dir        string // the folder the repository's goes in
	name       string // the repository's folder
	named      bool   // the name was typed, not taken from the address
	branch     string
	shallow    bool
	submodules bool
	err        string
	taken      string // dir/name, when a folder of that name is there
	checked    string // dir/name last looked at
	job        *proto.CloneStatus
	picker     folderpicker.Picker
	recent     []string // repositories cloned lately, on any host
	dirTyped   bool     // the folder was changed by hand
}

// openClone opens the dialog, into the folder the user cloned into last on
// this host, else the one the projects are in.
func (a *App) openClone() {
	if a.client == nil {
		return
	}
	dir := a.state.CloneDirs[a.host.ID]
	if dir == "" && len(a.projects) > 0 {
		dir = path.Dir(a.projects[len(a.projects)-1].Path)
	}
	if dir == "" {
		dir = "~"
	}
	f := &cloneForm{open: true, dir: dir}
	a.clone = f
	// The folder this host cloned into last, or the user's usual one, and
	// what was cloned lately: kept with the user's synced settings.
	cl := a.client
	go func() {
		prefs, err := cl.ClonePrefs()
		if err != nil {
			return // a daemon of before
		}
		a.onMain(func() {
			f.recent = prefs.URLs
			switch {
			case f.dirTyped:
			case prefs.Dir != "":
				f.dir = prefs.Dir
			case a.state.CloneDirs[a.host.ID] == "" && prefs.Default != "":
				f.dir = prefs.Default
			}
		})
	}()
}

// cloneEvent follows the clone the dialog started.
func (a *App) cloneEvent(data json.RawMessage) func() {
	var st proto.CloneStatus
	if json.Unmarshal(data, &st) != nil {
		return nil
	}
	return func() { a.cloneMoved(st) }
}

func (a *App) cloneMoved(st proto.CloneStatus) {
	f := a.clone
	if f == nil || f.job == nil || f.job.ID != st.ID {
		return
	}
	f.job = &st
	switch st.State {
	case proto.CloneDone:
		a.clone = nil
		if st.Project != nil {
			a.project, a.selected, a.page = st.Project.Path, "", ""
			delete(a.collapsed, st.Project.Path)
		}
		a.notify("Cloned into " + st.Path)
	case proto.CloneFailed:
		f.job, f.err = nil, st.Error
	case proto.CloneCanceled:
		f.job, f.err = nil, "Stopped; nothing was left behind."
	}
}

func (a *App) cloneDialog(c *ui.Context) {
	f := a.clone
	if f == nil {
		return
	}
	if !f.open {
		if f.job != nil {
			a.stopClone(f) // closing stops it
		}
		a.clone = nil
		return
	}
	if !f.named {
		f.name = ""
		if strings.TrimSpace(f.url) != "" {
			f.name = gitclone.Name(f.url)
		}
	}
	a.checkCloneTarget(f)
	start := false
	widget.FormSheet(c, &f.open, 540, "Clone from Git",
		"Clones the repository on "+a.host.Name+" with its git and credentials (SSH keys, gh, credential helpers), then opens it as a project.", func() {
			running := f.job != nil
			widget.Field(c, "Repository", func() {
				in := widget.Input(c, &f.url, "https://github.com/user/repo.git or git@github.com:user/repo.git").FillWidth().Font(theme.MonoFont).AutoFocus()
				start = in.Submitted()
				if running {
					in.Disabled(true)
				}
				if u := strings.TrimSpace(f.url); u != "" {
					if err := gitclone.Valid(u); err != nil {
						widget.CardNote(c.Key("invalid"), err.Error(), 0).TextColor(theme.Warning)
					}
				} else if len(f.recent) > 0 && !running {
					ui.Row(c.Key("lately")).FillWidth().Gap(theme.S1).Wrap().Children(func() {
						ui.Text(c, "Lately").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Height(theme.ControlHeight)
						for i, u := range f.recent[:min(5, len(f.recent))] {
							if widget.GhostButton(c.Key(i), gitclone.Name(u)).Tooltip(u).Clicked() {
								f.url = u
							}
						}
					})
				}
			})
			widget.Field(c, "Into", func() {
				ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
					before := f.dir
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Input(c, &f.dir, "~/code").FillWidth() })
					if f.dir != before {
						f.dirTyped = true
					}
					if widget.Button(c, widget.Secondary, nil, "Choose…", "").Clicked() && a.client != nil {
						f.picker.Open(a.client, a.onMain)
					}
				})
			})
			widget.Field(c, "Folder name", func() {
				before := f.name
				widget.Input(c, &f.name, "repo").FillWidth()
				if f.name != before {
					f.named = true
				}
			})
			target := cloneTarget(f)
			widget.Running(c.Key("target"), "Clones into "+target, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			if f.taken != "" && f.taken == target {
				widget.CardNote(c.Key("taken"), "A folder of that name is there already: the clone needs it empty or gone. Pick another name.", 0).TextColor(theme.Warning)
			}
			widget.Field(c, "Branch or tag (optional)", func() {
				widget.Input(c, &f.branch, "main").FillWidth()
			})
			switchRow(c, &f.shallow, "Shallow clone", "Only the latest commit (--depth 1): quicker for big repositories, without their history.")
			switchRow(c, &f.submodules, "Submodules", "Clone the repositories it includes too.")
			if j := f.job; j != nil {
				ui.Column(c.Key("job")).FillWidth().Shrink(0).Gap(theme.S1).Children(func() {
					ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
						ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, j.Phase, widget.RunStyle{Size: theme.TextLabel}) })
						if j.Percent > 0 {
							ui.Text(c, strconv.Itoa(j.Percent)+"%").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
						}
					})
					widget.Meter(c, widget.Segment{Fraction: float64(j.Percent) / 100, Color: theme.Accent})
					if j.Line != "" {
						widget.Running(c.Key("line"), j.Line, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft, Mono: true})
					}
				})
			}
			if f.err != "" {
				widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
			}
		}, func() {
			if f.job != nil {
				if widget.Button(c, widget.Secondary, nil, "Stop", "").Clicked() {
					a.stopClone(f)
				}
				return
			}
			b := widget.PrimaryButton(c, nil, "Clone", "")
			if gitclone.Valid(f.url) != nil || strings.TrimSpace(f.dir) == "" || strings.TrimSpace(f.name) == "" {
				b.Disabled(true)
			} else if b.Clicked() {
				start = true
			}
		})
	f.picker.View(c, a.host.Name, func(p string) { f.dir, f.checked = p, "" })
	if start && f.job == nil && gitclone.Valid(f.url) == nil {
		a.startClone(f)
	}
}

// cloneTarget is the folder the clone makes.
func cloneTarget(f *cloneForm) string {
	dir := strings.TrimRight(strings.TrimSpace(f.dir), "/")
	if dir == "" {
		dir = "~"
	}
	return dir + "/" + strings.TrimSpace(f.name)
}

// checkCloneTarget asks the host whether a folder of the name is there.
func (a *App) checkCloneTarget(f *cloneForm) {
	target := cloneTarget(f)
	cl := a.client
	if cl == nil || target == f.checked || strings.TrimSpace(f.name) == "" {
		return
	}
	f.checked = target
	dir, name := strings.TrimSpace(f.dir), strings.TrimSpace(f.name)
	go func() {
		l, err := cl.FolderList(proto.FolderListParams{Path: dir, Hidden: true, WithFiles: true})
		a.onMain(func() {
			if err != nil || f.checked != target {
				return
			}
			f.taken = ""
			for _, e := range append(l.Folders, l.FileNames...) {
				if e.Name == name {
					f.taken = target
				}
			}
		})
	}()
}

func (a *App) startClone(f *cloneForm) {
	cl := a.client
	if cl == nil {
		return
	}
	p := proto.ProjectCloneParams{URL: strings.TrimSpace(f.url), Dir: strings.TrimSpace(f.dir), Name: strings.TrimSpace(f.name),
		Branch: strings.TrimSpace(f.branch), Submodules: f.submodules}
	if f.shallow {
		p.Depth = 1
	}
	if a.state.CloneDirs == nil {
		a.state.CloneDirs = map[string]string{}
	}
	a.state.CloneDirs[a.host.ID] = p.Dir
	f.err = ""
	f.job = &proto.CloneStatus{State: proto.CloneRunning, Phase: "Starting"}
	go func() {
		st, err := cl.CloneProject(p)
		a.onMain(func() {
			if a.clone != f {
				return
			}
			if err != nil {
				f.job, f.err = nil, client.Explain(err)
				return
			}
			f.job = &st
		})
	}()
}

func (a *App) stopClone(f *cloneForm) {
	cl, j := a.client, f.job
	if cl == nil || j == nil || j.ID == "" {
		return
	}
	go func() { _ = cl.CancelClone(j.ID) }()
}
