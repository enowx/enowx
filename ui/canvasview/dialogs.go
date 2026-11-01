package canvasview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/widget"
)

// createForm is the New canvas dialog.
type createForm struct {
	open  bool
	title string
	kind  int
	group string
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
	widget.Dialog(c, &f.open, 480, 14, func() {
		widget.DialogHeader(c, "New canvas", "You and agents design it together; they see it on this host.", false)
		widget.Field(c, "Title", func() {
			widget.Input(c, &f.title, "Checkout flow").AutoFocus().Label("Title")
		})
		// Two cards a row, one in a narrow window: their text never squeezes.
		widget.FitGrid(c, 190, 2).Gap(8).Children(func() {
			for i, k := range kinds {
				if widget.OptionCard(c.Key(k.id), f.kind == i, k.name, k.about) {
					f.kind = i
				}
			}
		})
		widget.Field(c, "Group (optional)", func() { widget.Input(c, &f.group, "Ideas").Label("Group") })
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			if widget.Button(c, widget.Primary, nil, "Make canvas", "").Clicked() {
				f.open = false
				o.Create(proto.CanvasCreateParams{Title: f.title, Kind: kinds[f.kind].id, Group: strings.TrimSpace(f.group)}, func(cv proto.Canvas, err error) {
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

func (v *View) deleteDialog(c *ui.Context, o Options) {
	d := v.deleting
	if d == nil {
		return
	}
	open := true
	widget.Dialog(c, &open, 420, 14, func() {
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
	widget.Dialog(c, &open, 400, 14, func() {
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
