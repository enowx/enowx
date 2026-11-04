package projectdocsview

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// templatesNote names the top-level note whose children are the user's own
// templates.
const templatesNote = "Project Docs templates"

// The frames of the form: its pack cards keep their size whatever they list.
const (
	packHeight       = 220
	packNarrowHeight = 150
	packHeader       = 52
	formStatusHeight = 34
)

// newForm is the dialog making a project. Its fields belong to the main thread,
// the goroutine making the notes changing them through Options.Main.
type newForm struct {
	open     bool
	title    string
	size     string
	approval bool
	picked   map[string]bool
	busy     bool
	done     int
	total    int
	current  string
	err      string
}

// OpenNew opens making a project.
func (v *View) OpenNew() {
	v.form = &newForm{open: true, size: projectdocs.SizeCore, approval: true, picked: map[string]bool{}}
}

// userTemplates are the user's own templates: children of the templates note.
func userTemplates(list []proto.NoteInfo) []proto.NoteInfo {
	parent := ""
	for _, n := range list {
		if n.Parent == "" && strings.EqualFold(strings.TrimSpace(n.Title), templatesNote) {
			parent = n.ID
			break
		}
	}
	if parent == "" {
		return nil
	}
	var out []proto.NoteInfo
	for _, n := range list {
		if n.Parent == parent {
			out = append(out, n)
		}
	}
	return out
}

func (v *View) newDialog(c *ui.Context, o Options) {
	f := v.form
	if f == nil {
		return
	}
	if !f.open && !f.busy {
		v.form = nil
		return
	}
	f.open = true // what is being made finishes, or is taken back, before it closes
	w, _ := c.Size()
	stacked := w < 560
	widget.FormSheet(c.Key("new-docs"), &f.open, 640, "New project docs",
		"A project note, with its documents as ordinary notes under it.", func() {
			widget.Field(c, "Project title", func() {
				widget.Input(c, &f.title, "Project title").FillWidth()
			})
			packs := ui.Grid(c).FillWidth().Gap(10)
			if !stacked {
				packs.Columns(2)
			}
			packs.Children(func() {
				height := float32(packHeight)
				if stacked {
					height = packNarrowHeight
				}
				packCard(c.Key("core"), f, projectdocs.SizeCore, "Core pack", height)
				packCard(c.Key("full"), f, projectdocs.SizeFull, "Full pack", height)
			})
			approvalRow(c, f)
			if own := userTemplates(o.Notes); len(own) > 0 {
				widget.Field(c, "Your templates", func() {
					ui.Column(c).FillWidth().Gap(2).Children(func() {
						for _, n := range own {
							templateTick(c.Key(n.ID), f, n)
						}
					})
				})
			}
			formStatus(c, f)
		}, func() {
			b := widget.Button(c, widget.Primary, icon.Plus, "Create", "")
			if f.busy || strings.TrimSpace(f.title) == "" {
				b.Disabled(true).Opacity(0.45)
			} else if b.Clicked() {
				v.create(o, f)
			}
		})
	if !f.open && f.busy {
		f.open = true
	}
}

// packCard is a pack to pick: how many documents, and which, in a scroll of
// its fixed frame.
func packCard(c *ui.Context, f *newForm, size, name string, height float32) {
	entries := projectdocs.Pack(size)
	picked := f.size == size
	border := theme.Border
	if picked {
		border = theme.AccentLine
	}
	ui.Column(c).Height(height).MinWidth(0).Radius(theme.RadiusCard).Border(1, border).Clip().Label(name + " frame").Children(func() {
		b := ui.ButtonBase(c).FillWidth().Height(packHeader).Shrink(0).PaddingX(12).Gap(9).Cursor(ui.CursorPointer).AlignItems(ui.Center).MinWidth(0).Label(name)
		if picked {
			b.Background(theme.AccentSoft)
		} else if b.Hovered() {
			b.Background(theme.Hover)
		}
		b.Children(func() {
			widget.Dot(c, map[bool]ui.Color{true: theme.Accent, false: theme.Idle}[picked], !picked, 9)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).FillWidth().MinWidth(0).Children(func() {
					running(c, name, runStyle{Size: 12.5, Weight: 600, Color: theme.Text}).Grow(1)
				})
				ui.Row(c).FillWidth().MinWidth(0).Children(func() {
					running(c, plural(len(entries), "document", "documents"), runStyle{Size: 11, Color: theme.Muted}).Grow(1)
				})
			})
		})
		if b.Clicked() && !f.busy {
			f.size = size
		}
		ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(6, 12, 8, 12).Children(func() {
				for _, e := range entries {
					ui.Row(c.Key(e.Key)).FillWidth().Height(22).Gap(7).AlignItems(ui.Center).MinWidth(0).Children(func() {
						widget.NoteIcon(c, e.Icon, 12, theme.Muted, nil).Shrink(0)
						running(c, e.Name, runStyle{Size: 11.5, Color: theme.TextSoft}).Grow(1)
					})
				}
			})
		})
	})
}

func approvalRow(c *ui.Context, f *newForm) {
	const label = "Require approval of PRD and Technical Spec before building"
	ui.Row(c).FillWidth().Height(lineHeight).Gap(9).AlignItems(ui.Center).MinWidth(0).Children(func() {
		widget.Switch(c, &f.approval, label)
		running(c, label, runStyle{Size: 12.5, Color: theme.TextSoft}).Grow(1)
	})
}

// templateTick is a template to add. A row of its own rather than
// widget.Checkbox, whose label doesn't give way to a long title.
func templateTick(c *ui.Context, f *newForm, n proto.NoteInfo) {
	title := titleOf(n)
	b := ui.ButtonBase(c).FillWidth().Height(28).Gap(8).Cursor(ui.CursorPointer).MinWidth(0).AlignItems(ui.Center).Label(title)
	on := f.picked[n.ID]
	b.Children(func() {
		box := ui.Box(c).Size(15, 15).Radius(4).Center().Shrink(0)
		if on {
			box.Background(theme.Accent).TextColor(theme.OnAccent)
		} else {
			box.Border(1, theme.Idle)
		}
		box.Children(func() {
			if on {
				ui.Icon(c, icon.Check).FontSize(10)
			}
		})
		running(c, title, runStyle{Size: 12.5, Color: theme.TextSoft}).Grow(1)
	})
	if b.Clicked() && !f.busy {
		f.picked[n.ID] = !on
	}
}

// formStatus says how making goes, in room always kept for it.
func formStatus(c *ui.Context, f *newForm) {
	ui.Column(c).FillWidth().Height(formStatusHeight).Gap(4).MinWidth(0).Justify(ui.Center).Label("Making progress").Children(func() {
		switch {
		case f.err != "":
			line(c, icon.Alert, f.err, theme.Error)
		case f.busy:
			line(c, nil, fmt.Sprintf("Making %s (%d of %d)", f.current, f.done, f.total), theme.TextSoft)
			progress(c, f.done, f.total)
		}
	})
}

// create makes the project off the main thread, a note at a time; what it made
// is removed if it can't finish.
func (v *View) create(o Options, f *newForm) {
	picked := slices.Collect(func(yield func(string) bool) {
		for id, on := range f.picked {
			if on && !yield(id) {
				return
			}
		}
	})
	makeProject := v.makeProject
	if makeProject == nil {
		makeProject = projectdocs.New
	}
	opts := projectdocs.Options{Title: strings.TrimSpace(f.title), Size: f.size, Team: o.Team}
	opts.Approval = projectdocs.ApprovalOff
	if f.approval {
		opts.Approval = projectdocs.ApprovalRequired
	}
	f.busy, f.err, f.done, f.current = true, "", 0, opts.Title
	f.total = packNotes(f.size) + len(picked)
	go func() {
		save := func(n proto.Note) (proto.NoteInfo, error) { return saveOn(o, f, n) }
		for _, id := range picked {
			n, err := o.Read(id)
			if err != nil {
				o.Main(func() { f.busy, f.err = false, "Couldn't read a template: "+err.Error() })
				return
			}
			opts.Own = append(opts.Own, n)
		}
		root, made, err := makeProject(opts, save)
		if err != nil {
			left := removeMade(o, made)
			o.Main(func() { f.busy, f.err = false, failureText(err, left) })
			return
		}
		o.Main(func() {
			f.busy, f.open = false, false
			v.form = nil
			o.Open(root.ID)
		})
	}()
}

// packNotes is how many notes a pack makes: the project's, its documents' and,
// for the full pack, its areas'.
func packNotes(size string) int {
	n := 1 + len(projectdocs.Pack(size))
	if size == projectdocs.SizeFull {
		n += len(projectdocs.Areas)
	}
	return n
}

// saveOn saves a note through Options.Save, on the main thread as it expects,
// for the goroutine that waits here. The channel is buffered: done may run
// before the wait starts.
func saveOn(o Options, f *newForm, n proto.Note) (proto.NoteInfo, error) {
	type saved struct {
		info proto.NoteInfo
		err  error
	}
	res := make(chan saved, 1)
	o.Main(func() {
		f.done++
		if t := strings.TrimSpace(n.Title); t != "" {
			f.current = t
		}
		o.Save(n, func(info proto.NoteInfo, err error) { res <- saved{info, err} })
	})
	s := <-res
	return s.info, s.err
}

// removeMade deletes the notes made, the last first since those under a note
// are made after it, and says how many stayed.
func removeMade(o Options, made []string) int {
	left := 0
	for _, id := range slices.Backward(made) {
		res := make(chan error, 1)
		o.Main(func() { o.Delete(id, func(err error) { res <- err }) })
		if <-res != nil {
			left++
		}
	}
	return left
}

// failureText says why making failed, and what it left behind.
func failureText(err error, left int) string {
	why := strings.TrimPrefix(err.Error(), "projectdocs: ")
	if left > 0 {
		return plural(left, "note", "notes") + " couldn't be removed: " + why
	}
	return "Nothing was made: " + why
}
