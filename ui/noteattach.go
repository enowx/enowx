package ui

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/notes"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// noteAttachForm is the dialog attaching a note to projects.
type noteAttachForm struct {
	open   bool
	note   proto.NoteInfo
	picked map[string]proto.NoteProject // by ref
	filter string
	saving bool
	err    string
}

// openAttach opens the dialog attaching note n.
func (a *App) openAttach(n proto.NoteInfo) {
	f := &noteAttachForm{open: true, note: n, picked: map[string]proto.NoteProject{}}
	for _, p := range n.Projects {
		f.picked[p.Ref] = p
	}
	a.attaching = f
	a.catalogs.read = time.Time{} // fresh
	a.loadCatalogs()
}

// attachDialog draws the dialog while it is open.
func (a *App) attachDialog(c *ui.Context) {
	f := a.attaching
	if f == nil {
		return
	}
	if !f.open {
		a.attaching = nil
		return
	}
	widget.FormSheet(c, &f.open, 560, "Attach to projects",
		"“"+noteTitle(f.note)+"”, with projects of every host: the plan shows on each, and agents working in them find it.", func() {
			widget.SearchField(c, &f.filter, "Filter projects", icon.Search)
			q := strings.ToLower(strings.TrimSpace(f.filter))
			listed := map[string]bool{}
			for _, cat := range a.catalogs.list {
				cat.Name = a.daemonHostName(cat.Host, cat.Name)
				var shown []proto.NoteProject
				for _, cp := range cat.Projects {
					np := notes.CatalogProject(cat, cp)
					if listed[np.Ref] {
						continue // the same repository on another host: once
					}
					if q == "" || strings.Contains(strings.ToLower(cp.Name+" "+cp.Path+" "+cat.Name), q) {
						shown = append(shown, np)
						listed[np.Ref] = true
					}
				}
				if len(shown) == 0 {
					continue
				}
				widget.Field(c.Key("host:"+cat.Host), cat.Name, func() {
					ui.Column(c).FillWidth().Gap(2).Children(func() {
						for _, np := range shown {
							a.attachRow(c.Key(np.Ref), f, np)
						}
					})
				})
			}
			if f.note.Team != "" {
				// A team's note attaches to the team's projects, wherever they are.
				var shown []proto.NoteProject
				for _, np := range a.teamProjectsToAttach(f.note.Team) {
					if !listed[np.Ref] && (q == "" || strings.Contains(strings.ToLower(np.Name+" "+np.Remote), q)) {
						shown = append(shown, np)
						listed[np.Ref] = true
					}
				}
				if len(shown) > 0 {
					widget.Field(c.Key("team-projects"), a.spaceName(f.note.Team)+"'s projects", func() {
						ui.Column(c).FillWidth().Gap(2).Children(func() {
							for _, np := range shown {
								a.attachRow(c.Key(np.Ref), f, np)
							}
						})
					})
				}
			}
			var lost []proto.NoteProject
			for _, p := range f.note.Projects {
				if !listed[p.Ref] {
					lost = append(lost, p)
				}
			}
			if len(lost) > 0 {
				widget.Field(c.Key("lost"), "Attached, not found now", func() {
					ui.Column(c).FillWidth().Gap(2).Children(func() {
						for _, p := range lost {
							a.attachRow(c.Key(p.Ref), f, p)
						}
					})
				})
			}
			if len(a.catalogs.list) == 0 {
				ui.Text(c, "Reading the hosts' projects…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			}
			if f.err != "" {
				ui.Text(c, f.err).FontSize(theme.TextLabel).TextColor(theme.Error)
			}
		}, func() {
			b := widget.PrimaryButton(c, nil, "Done", "")
			if f.saving {
				b.Disabled(true).Opacity(0.5)
			} else if b.Clicked() {
				a.saveAttach(f)
			}
		})
}

func (a *App) attachRow(c *ui.Context, f *noteAttachForm, p proto.NoteProject) {
	_, on := f.picked[p.Ref]
	toggle := func() {
		if on {
			f.picked[p.Ref] = p
		} else {
			delete(f.picked, p.Ref)
		}
	}
	ui.Row(c).FillWidth().MinHeight(36).Shrink(0).Padding(theme.S1, theme.S3).Gap(theme.S2).Children(func() {
		// Its name turns it on and off as well as its switch.
		name := ui.ButtonBase(c).Grow(1).MinWidth(0).Column().AlignItems(ui.Start).Cursor(ui.CursorPointer).Label(p.Name)
		name.Children(func() {
			widget.Running(c, p.Name, widget.RunStyle{})
			widget.Running(c.Key("path"), p.Path, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		if name.Clicked() {
			on = !on
			toggle()
		}
		if widget.Switch(c, &on, "Attached") {
			toggle()
		}
	})
}

// saveAttach attaches the projects picked and detaches those unpicked.
func (a *App) saveAttach(f *noteAttachForm) {
	cl := a.client
	if cl == nil {
		return
	}
	var add []proto.NoteProject
	for ref, p := range f.picked {
		if !slices.ContainsFunc(f.note.Projects, func(x proto.NoteProject) bool { return x.Ref == ref }) {
			add = append(add, p)
		}
	}
	var drop []string
	for _, p := range f.note.Projects {
		if _, ok := f.picked[p.Ref]; !ok {
			drop = append(drop, p.Ref)
		}
	}
	f.saving = true
	id := f.note.ID
	go func() {
		var err error
		if len(add) > 0 {
			_, err = cl.AttachNote(id, add)
		}
		if err == nil && len(drop) > 0 {
			_, err = cl.DetachNote(id, drop, false)
		}
		a.onMain(func() {
			f.saving = false
			if err != nil {
				f.err = client.Explain(err)
				return
			}
			f.open = false
		})
	}()
}
