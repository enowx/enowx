package canvasview

import (
	"fmt"
	"slices"
	"strings"
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
	v.pendingFrame = ref
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
	prev := widget.IconButton(c, icon.ChevronLeft, "Design before (←)", false)
	if i == 0 {
		prev.Disabled(true).Opacity(0.4)
	} else if prev.Clicked() {
		v.step(-1)
	}
	f := v.frameOf()
	label := fmt.Sprintf("Design %d of %d · %s  %d×%d", i+1, n, f.Name, f.Width, f.Height)
	if v.compact() {
		label = fmt.Sprintf("%d / %d", i+1, n)
	}
	t := ui.Text(c, label).FontSize(11.5).TextColor(theme.Text).SingleLine().Tooltip(f.Name)
	if v.compact() {
		t.Shrink(0)
	} else {
		t.Shrink(1).MinWidth(0)
	}
	next := widget.IconButton(c, icon.ChevronRight, "Design after (→)", false)
	if i >= n-1 {
		next.Disabled(true).Opacity(0.4)
	} else if next.Clicked() {
		v.step(1)
	}
	add := widget.IconButton(c, icon.Plus, "Add a design", false)
	v.menu.OpenBelow(add, "add-frame", 28)
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

// designLine is the shown design's name and what it tries, with its star
// and the winner's mark.
func (v *View) designLine(c *ui.Context, o Options) {
	f, cv := v.frameOf(), v.canvas
	if len(cv.Frames) < 2 && f.Description == "" {
		return
	}
	ui.Row(c).FillWidth().Height(36).Shrink(0).PaddingX(14).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Text(c, fmt.Sprintf("%d · %s", v.designIndex()+1, f.Name)).FontSize(12.5).FontWeight(600).TextColor(theme.TextStrong).SingleLine()
		if g := groupOf(f); g != "" {
			widget.Tag(c, g, theme.Muted)
		}
		if cv.Winner == f.ID {
			widget.Tag(c, "Winner", theme.Done)
		}
		ui.Text(c, f.Description).FontSize(11.5).TextColor(theme.Muted).SingleLine().Shrink(1).Grow(1).MinWidth(0)
		v.starButton(c, o, f)
		more := widget.IconButton(c, icon.More, "Design "+f.Name, false)
		v.menu.OpenBelow(more, "design-more", 28)
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

// strip is every design, small, by page: a click shows one, a drag puts it
// before another; the one an agent works on is marked.
func (v *View) strip(c *ui.Context, o Options, cv proto.Canvas) {
	ui.Column(c.Key("strip")).Width(176).Shrink(0).FillHeight().Background(theme.Panel).BorderWidth(0, 1, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(10).Gap(10).Children(func() {
				page := "\x00"
				for i, f := range cv.Frames {
					if f.Page != page && (f.Page != "" || page != "\x00") {
						name := f.Page
						if name == "" {
							name = "Other"
						}
						widget.SectionLabel(c.Key("page:"+f.ID), strings.ToUpper(name), theme.Muted)
					}
					page = f.Page
					v.stripItem(c.Key(f.ID), o, cv, i, f)
				}
			})
		})
	})
}

// working says an agent changes design f now.
func (v *View) working(f proto.CanvasFrame) bool {
	return v.last.Frame == f.ID && time.Since(v.lastAt) < designingFor
}

func (v *View) stripItem(c *ui.Context, o Options, cv proto.Canvas, i int, f proto.CanvasFrame) {
	label := fmt.Sprintf("Design %d", i+1)
	item := ui.ButtonBase(c).FillWidth().Padding(6).Gap(5).Radius(8).Column().Cursor(ui.CursorPointer).Label(label)
	switch {
	case f.ID == v.frame:
		item.Background(theme.Selected).Border(1, theme.AccentLine)
	case item.Hovered():
		item.Background(theme.Hover)
	}
	item.Drag(designDrag{id: f.ID})
	if d, ok := ui.Drop[designDrag](item); ok && d.id != f.ID {
		v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpFrameMove, Frame: d.id, Before: f.ID})
	}
	if d, ok := ui.DragOver[designDrag](item); ok && d.id != f.ID {
		item.BorderWidth(2, 0, 0, 0).BorderColor(theme.Accent)
	}
	if item.Clicked() {
		v.frame, v.sel, v.preview = f.ID, "", 0
	}
	v.menu.Open(item, "design:"+f.ID)
	v.menu.Show(c, item, "design:"+f.ID, func(done func()) { v.designItems(c, o, f, done) })
	item.Children(func() {
		v.designPreview(c.Key("pv"), o, f, 152)
		ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%d", i+1)).FontSize(10.5).FontWeight(600).TextColor(theme.Muted)
			ui.Text(c, f.Name).FontSize(11.5).TextColor(theme.Text).SingleLine().Shrink(1).Grow(1).MinWidth(0)
			if v.working(f) {
				widget.Dot(c, theme.Accent, false, 6)
				c.After(designingFor)
			}
			if f.Favorite {
				ui.Icon(c, icon.Star).FontSize(11).TextColor(theme.Warning)
			}
			if cv.Winner == f.ID {
				ui.Icon(c, icon.Award).FontSize(11).TextColor(theme.Done)
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
	widget.Dialog(c, &f.open, 440, 14, func() {
		widget.DialogHeader(c, "Name and describe the design", "", false)
		widget.Field(c, "Name", func() { widget.Input(c, &f.name, "Hero B").AutoFocus().Label("Design name") })
		widget.Field(c, "What it tries", func() { widget.Input(c, &f.description, "Big type, one call to action").Label("Design description") })
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			ui.Column(c).Grow(1).Basis(0).Children(func() {
				widget.Field(c, "Page", func() { widget.Input(c, &f.page, "Home").Label("Design page") })
			})
			ui.Column(c).Grow(1).Basis(0).Children(func() {
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
