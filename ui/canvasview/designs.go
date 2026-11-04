package canvasview

import (
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A canvas's frames are its designs: variations of a page, or a flow's
// screens. The page shows one at a time with a strip of them all beside
// it, or all of them as a grid, or a few side by side.

// designDrag is what dragging a design in the strip carries.
type designDrag struct{ id string }

// Goto shows design ref (a number, name or ID) of canvas id, as an agent
// or a link asks.
func (v *View) Goto(o Options, id, ref string) {
	v.Open(o, id)
	v.view = viewDesign
	if v.canvas != nil && v.canvas.ID == id {
		if i, err := canvas.FrameIndex(*v.canvas, ref); err == nil {
			v.frame, v.sel = v.canvas.Frames[i].ID, ""
			return
		}
	}
	v.pendingFrame, v.pendingAt = ref, time.Now()
}

// CompareDesigns shows designs (frame IDs) of canvas id side by side.
func (v *View) CompareDesigns(o Options, id string, frames []string) {
	v.Open(o, id)
	v.cmpSel = map[string]bool{}
	for _, f := range frames {
		v.cmpSel[f] = true
	}
	v.view = viewCompare
}

// designIndex is the index of the design shown.
func (v *View) designIndex() int {
	if v.canvas == nil {
		return 0
	}
	return max(slices.IndexFunc(v.canvas.Frames, func(f proto.CanvasFrame) bool { return f.ID == v.frame }), 0)
}

// step shows the design by from the one shown.
func (v *View) step(by int) {
	if v.canvas == nil || len(v.canvas.Frames) == 0 {
		return
	}
	i := min(max(v.designIndex()+by, 0), len(v.canvas.Frames)-1)
	v.frame, v.sel, v.preview = v.canvas.Frames[i].ID, "", 0
}

// designNav is the design shown, with the one before and after.
func (v *View) designNav(c *ui.Context, o Options, cv proto.Canvas) {
	i, n := v.designIndex(), len(cv.Frames)
	prev := widget.IconButton(c.Key("prev"), icon.ChevronLeft, "Design before (←)", false)
	if i == 0 {
		prev.Disabled(true)
	} else if prev.Clicked() {
		v.step(-1)
	}
	f := v.frameOf()
	label := fmt.Sprintf("Design %d of %d", i+1, n)
	if v.stageRoom() < 620 {
		label = fmt.Sprintf("%d / %d", i+1, n)
	}
	ui.Text(c.Key("at"), label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0).Tooltip(f.Name)
	next := widget.IconButton(c.Key("next"), icon.ChevronRight, "Design after (→)", false)
	if i >= n-1 {
		next.Disabled(true)
	} else if next.Clicked() {
		v.step(1)
	}
	add := widget.IconButton(c.Key("add"), icon.Plus, "Add a design", false)
	v.menu.OpenBelow(add, "add-frame", theme.ControlHeight)
	v.menu.Show(c, add, "add-frame", func(done func()) {
		if widget.MenuItem(c.Key("blank"), icon.Plus, "New design", false) {
			v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameAdd, FrameInfo: &proto.CanvasFrame{Name: fmt.Sprintf("Design %d", n+1), Width: f.Width, Height: f.Height}})
			done()
		}
		if widget.MenuItem(c.Key("copy"), icon.Copy, "Duplicate as a variant", false) {
			v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameCopy, Frame: f.ID})
			done()
		}
		widget.MenuSeparator(c)
		for _, d := range canvas.Devices {
			if widget.MenuItem(c.Key(d.Name), icon.Square, fmt.Sprintf("%s · %d×%d", d.Name, d.Width, d.Height), false) {
				v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameAdd, FrameInfo: &proto.CanvasFrame{Name: d.Name}})
				done()
			}
		}
	})
}

// designLine is under the stage: the shown design's name and size and what it
// tries, with its star and the winner's mark.
func (v *View) designLine(c *ui.Context, o Options) {
	f, cv := v.frameOf(), v.canvas
	if len(cv.Frames) < 2 && f.Description == "" {
		return
	}
	ui.Row(c.Key("design-line")).FillWidth().MinHeight(theme.ControlHeight).Shrink(0).Padding(theme.S1, 0, 0, theme.S1).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		ui.Row(c.Key("name")).MinWidth(0).Shrink(1).Children(func() {
			widget.Running(c, fmt.Sprintf("%d %s · %s", v.designIndex()+1, f.Name, sizeOf(f)), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		if g := groupOf(f); g != "" {
			widget.Tag(c.Key("group"), g, theme.Muted)
		}
		if cv.Winner == f.ID {
			widget.Tag(c.Key("winner"), "Winner", theme.Done)
		}
		ui.Row(c.Key("about")).Grow(1).Basis(0).MinWidth(0).Children(func() {
			if f.Description != "" {
				widget.Running(c, f.Description, widget.RunStyle{Size: theme.TextLabel, Color: theme.Muted})
			}
		})
		v.starButton(c, o, f)
		more := widget.IconButton(c.Key("more"), icon.More, "Design "+f.Name, false)
		v.menu.OpenBelow(more, "design-more", theme.ControlHeight)
		v.menu.Show(c, more, "design-more", func(done func()) { v.designItems(c, o, f, done) })
	})
}

// groupOf is a design's page × variant, as the user reads it.
func groupOf(f proto.CanvasFrame) string {
	switch {
	case f.Page != "" && f.Variant != "":
		return f.Page + " · " + f.Variant
	case f.Page != "":
		return f.Page
	case f.Variant != "":
		return "Variant " + f.Variant
	}
	return ""
}

func (v *View) starButton(c *ui.Context, o Options, f proto.CanvasFrame) {
	label := "Star " + f.Name
	if f.Favorite {
		label = "Unstar " + f.Name
	}
	b := widget.IconButton(c.Key("star:"+f.ID), icon.Star, label, f.Favorite)
	if f.Favorite {
		b.TextColor(theme.Warning)
	}
	if b.Clicked() {
		on := !f.Favorite
		v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameMeta, Frame: f.ID, Favorite: &on})
	}
}

// designItems are a design's actions, for its menus.
func (v *View) designItems(c *ui.Context, o Options, f proto.CanvasFrame, done func()) {
	cv := v.canvas
	star := "Star"
	if f.Favorite {
		star = "Unstar"
	}
	if widget.MenuItem(c.Key("star"), icon.Star, star, false) {
		on := !f.Favorite
		v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameMeta, Frame: f.ID, Favorite: &on})
		done()
	}
	if cv.Winner == f.ID {
		if widget.MenuItem(c.Key("unpick"), icon.Award, "No longer the winner", false) {
			v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpPick})
			done()
		}
	} else if widget.MenuItem(c.Key("pick"), icon.Award, "Pick as the winner", false) {
		v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpPick, Frame: f.ID})
		done()
	}
	if widget.MenuItem(c.Key("copy"), icon.Copy, "Duplicate as a variant", false) {
		v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameCopy, Frame: f.ID})
		done()
	}
	if widget.MenuItem(c.Key("describe"), icon.Pencil, "Name and describe…", false) {
		v.descr = &describeForm{open: true, frame: f.ID, name: f.Name, description: f.Description, page: f.Page, variant: f.Variant}
		done()
	}
	if widget.MenuItem(c.Key("link"), icon.Copy, "Copy link", false) {
		i := slices.IndexFunc(cv.Frames, func(x proto.CanvasFrame) bool { return x.ID == f.ID })
		c.WriteClipboard(canvas.Link(cv.ID, i+1))
		done()
	}
	v.libraryItem(c, o, cv.ID, f, done)
	if len(cv.Frames) > 1 {
		widget.MenuSeparator(c)
		if widget.MenuItem(c.Key("rm"), icon.Trash, "Delete design", true) {
			confirmed(o, "Delete “"+f.Name+"”?", "The design leaves this canvas. An earlier version in History still holds it.", "Delete", func() {
				v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameRemove, Frame: f.ID})
			})
			done()
		}
	}
}

// Sizes of the strip's items: a picture of any shape fits a fixed box, and
// the box never grows with it.
const (
	thumbW = 128
	thumbH = 80
)

// strip is every design, small, by page, under the stage: a click shows one, a
// drag puts it before another; the one an agent works on is marked.
func (v *View) strip(c *ui.Context, o Options, cv proto.Canvas) {
	ui.ScrollHorizontal(c.Key("strip")).FillWidth().Shrink(0).Padding(theme.S1, 0, 0, 0).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		page := "\x00"
		for i, f := range cv.Frames {
			if f.Page != page && (f.Page != "" || page != "\x00") {
				name := f.Page
				if name == "" {
					name = "Other"
				}
				ui.Row(c.Key("page:" + f.ID)).MaxWidth(96).Shrink(0).Children(func() {
					widget.Running(c, name, widget.RunStyle{Size: theme.TextLabel, Weight: 500, Color: theme.TextSoft})
				})
			}
			page = f.Page
			v.stripItem(c.Key(f.ID), o, cv, i, f)
		}
	})
}

// working says an agent changes design f now.
func (v *View) working(f proto.CanvasFrame) bool {
	return v.last.Frame == f.ID && time.Since(v.lastAt) < designingFor
}

func (v *View) stripItem(c *ui.Context, o Options, cv proto.Canvas, i int, f proto.CanvasFrame) {
	label := fmt.Sprintf("Design %d", i+1)
	item := ui.ButtonBase(c).Width(thumbW + 2*theme.S1).Shrink(0).Padding(theme.S1).Gap(theme.S1).Radius(theme.RadiusSmall).Column().Cursor(ui.CursorPointer).Label(label)
	switch {
	case f.ID == v.frame:
		item.Background(theme.Selected)
	case item.Hovered():
		item.Background(theme.Hover)
	}
	item.Drag(designDrag{id: f.ID})
	if d, ok := ui.Drop[designDrag](item); ok && d.id != f.ID {
		v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameMove, Frame: d.id, Before: f.ID})
	}
	if d, ok := ui.DragOver[designDrag](item); ok && d.id != f.ID {
		item.BorderWidth(0, 0, 0, 2).BorderColor(theme.Accent) // it goes before this one
	}
	if item.Clicked() {
		v.frame, v.sel, v.preview = f.ID, "", 0
	}
	v.menu.Open(item, "design:"+f.ID)
	v.menu.Show(c, item, "design:"+f.ID, func(done func()) { v.designItems(c, o, f, done) })
	item.Children(func() {
		ui.Row(c.Key("box")).Size(thumbW, thumbH).Shrink(0).Radius(theme.RadiusTail).Clip().Background(theme.Sunken).Center().Children(func() {
			w := float32(thumbW)
			if f.Width > 0 && f.Height > 0 {
				w = min(w, thumbH*float32(f.Width)/float32(f.Height))
			}
			v.designPreview(c.Key("pv"), o, f, w)
		})
		ui.Row(c.Key("cap")).FillWidth().Gap(theme.S1).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, fmt.Sprintf("%d %s", i+1, f.Name), widget.RunStyle{Size: theme.TextLabel})
			})
			if v.working(f) {
				widget.Dot(c.Key("working"), theme.Accent, false, 6)
				c.After(designingFor)
			}
			if f.Favorite {
				ui.Icon(c.Key("fav"), icon.Star).FontSize(12).TextColor(theme.Warning)
			}
			if cv.Winner == f.ID {
				ui.Icon(c.Key("won"), icon.Award).FontSize(12).TextColor(theme.Done)
			}
		})
	})
}

// designKeys are the designs' shortcuts: ← and → step, 1 to 9 jump.
func (v *View) designKeys(c *ui.Context) {
	if v.canvas == nil || v.editing != nil || v.view != viewDesign {
		return
	}
	if c.Shortcut(0, ui.KeyLeft) {
		v.step(-1)
	}
	if c.Shortcut(0, ui.KeyRight) {
		v.step(1)
	}
	for n := range 9 {
		if c.Shortcut(0, ui.Key1+ui.Key(n)) && n < len(v.canvas.Frames) {
			v.frame, v.sel, v.preview = v.canvas.Frames[n].ID, "", 0
		}
	}
}

// describeForm names and describes a design.
type describeForm struct {
	open                             bool
	frame                            string
	name, description, page, variant string
}

func (v *View) describeDialog(c *ui.Context, o Options) {
	f := v.descr
	if f == nil {
		return
	}
	widget.Dialog(c, &f.open, 440, theme.S3, func() {
		widget.DialogHeader(c, "Name and describe the design", "", false)
		widget.Field(c, "Name", func() { widget.Input(c, &f.name, "Hero B").AutoFocus().Label("Design name") })
		widget.Field(c, "What it tries", func() { widget.Input(c, &f.description, "Big type, one call to action").Label("Design description") })
		ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Field(c, "Page", func() { widget.Input(c, &f.page, "Home").Label("Design page") })
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Field(c, "Variant", func() { widget.Input(c, &f.variant, "B").Label("Design variant") })
			})
		})
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			if widget.Button(c, widget.Primary, nil, "Save", "").Clicked() {
				f.open = false
				v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameMeta, Frame: f.frame, Name: &f.name, Description: &f.description, Page: &f.page, Variant: &f.variant})
			}
		})
	})
	if !f.open {
		v.descr = nil
	}
}

// confirmed does do once the user agrees, through the app's dialog.
func confirmed(o Options, title, msg, action string, do func()) {
	if o.Confirm == nil {
		do()
		return
	}
	o.Confirm(title, msg, action, do)
}
