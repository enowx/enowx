package canvasview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// createForm is the New canvas dialog.
type createForm struct {
	open  bool
	title string
	kind  int
	group string
	// project is the project it belongs to, "" none.
	project string
}

var kinds = []struct{ id, name, about string }{
	{proto.CanvasMockup, "Mockup", "A UI in HTML and CSS, interactive"},
	{proto.CanvasWireframe, "Wireframe", "A UI in boxes and text"},
	{proto.CanvasDiagram, "Diagram", "A flow or an architecture"},
	{proto.CanvasBoard, "Board", "Notes, shapes and images"},
}

func (v *View) createDialog(c *ui.Context, o Options) {
	f := v.creating
	if f == nil {
		return
	}
	widget.Dialog(c, &f.open, 440, theme.S3, func() {
		widget.DialogHeader(c, "New canvas", "You and agents design it together; they see it on this host.", false)
		widget.Field(c, "Title", func() {
			widget.Input(c, &f.title, "Checkout").AutoFocus().Label("Title")
		})
		widget.Field(c, "Kind", func() { v.kindSelect(c, f) })
		if len(o.Projects) > 0 {
			widget.Field(c, "Project", func() { v.projectSelect(c, o, f) })
		}
		widget.Field(c, "Group", func() { widget.Input(c, &f.group, "Ideas").Label("Group") })
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			if widget.Button(c, widget.Primary, nil, "Create", "").Label("Create canvas").Clicked() {
				f.open = false
				p := proto.CanvasCreateParams{Title: f.title, Kind: kinds[f.kind].id, Group: strings.TrimSpace(f.group)}
				if f.project != "" {
					p.Projects = []string{f.project}
				}
				o.Create(p, func(cv proto.Canvas, err error) {
					if err != nil {
						v.err = err.Error()
						return
					}
					v.selected = cv.ID
					v.show(o, cv)
					v.comments = nil
				})
			}
		})
	})
	if !f.open {
		v.creating = nil
	}
}

// kindSelect picks the new canvas's kind.
func (v *View) kindSelect(c *ui.Context, f *createForm) {
	t := widget.Trigger(ui.ButtonBase(c.Key("kind")).Label("Kind"), false)
	t.Children(func() {
		ui.Text(c, kinds[f.kind].name).FontSize(theme.TextUI).TextColor(theme.Text).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	v.menu.OpenBelow(t, "new-kind", theme.ControlHeight)
	v.menu.Show(c, t, "new-kind", func(done func()) {
		for i, k := range kinds {
			if widget.MenuItemSub(c.Key(k.id), nil, k.name, k.about, false) {
				f.kind = i
				done()
			}
		}
	})
}

// projectSelect picks the project the new canvas belongs to, if any.
func (v *View) projectSelect(c *ui.Context, o Options, f *createForm) {
	name := "No project"
	for _, p := range o.Projects {
		if p.Path == f.project {
			name = p.Name
		}
	}
	t := widget.Trigger(ui.ButtonBase(c.Key("project")).Label("Project"), false)
	t.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, name, widget.RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	v.menu.OpenBelow(t, "new-project", theme.ControlHeight)
	v.menu.Show(c, t, "new-project", func(done func()) {
		if widget.MenuItem(c.Key("none"), nil, "No project", false) {
			f.project = ""
			done()
		}
		for _, p := range o.Projects {
			if widget.MenuItem(c.Key(p.Path), icon.Folder, p.Name, false) {
				f.project = p.Path
				done()
			}
		}
	})
}

func (v *View) deleteDialog(c *ui.Context, o Options) {
	d := v.deleting
	if d == nil {
		return
	}
	open := true
	widget.Dialog(c, &open, 420, theme.S3, func() {
		widget.DialogHeader(c, "Delete “"+d.Title+"”?", "Its frames, versions, comments and images go; this can't be undone.", false)
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				open = false
			}
			if widget.DangerButton(c, "Delete canvas").Clicked() {
				open = false
				id := d.ID
				o.Delete(id, func(err error) {
					if err != nil {
						v.err = err.Error()
						return
					}
					if v.selected == id {
						v.selected, v.canvas = "", nil
						if o.Web != nil {
							o.Web.Close()
						}
					}
				})
			}
		})
	})
	if !open {
		v.deleting = nil
	}
}

func (v *View) renameDialog(c *ui.Context, o Options) {
	title := v.renaming
	if title == nil || v.canvas == nil {
		v.renaming = nil
		return
	}
	open := true
	widget.Dialog(c, &open, 400, theme.S3, func() {
		widget.DialogHeader(c, "Rename canvas", "", false)
		in := widget.Input(c, title, "Title").AutoFocus().Label("New title")
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				open = false
			}
			if widget.Button(c, widget.Primary, nil, "Rename", "").Clicked() || in.Submitted() {
				open = false
				t := *title
				v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpMeta, Title: &t})
			}
		})
	})
	if !open {
		v.renaming = nil
	}
}
