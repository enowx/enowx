package canvasview

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// toolbar is over the stage: the designs' navigation and components to insert;
// undo and redo, the guides, the preview's width and the mode; the side panel.
func (v *View) toolbar(c *ui.Context, o Options, cv proto.Canvas) {
	f := v.frameOf()
	// In a narrow stage, buttons show as icons alone.
	narrow := v.stageRoom() < 620
	// As tall as the Edit and Interact segments it holds, its tallest control.
	var tools ui.Element
	if v.stageRoom() < toolsRoom {
		tools = ui.ScrollHorizontal(c.Key("canvas-tools")) // too narrow for them all, they scroll
	} else {
		tools = ui.Row(c.Key("canvas-tools"))
	}
	tools.FillWidth().Height(36).Shrink(0).Gap(theme.S1).AlignItems(ui.Center).Children(func() {
		v.designNav(c, o, cv)
		if cv.Kind == proto.CanvasMockup || strings.TrimSpace(f.HTML) != "" {
			var ins ui.Element
			if narrow {
				ins = widget.IconButton(c.Key("insert"), icon.Square, "Insert a component", false)
			} else {
				ins = widget.Button(c.Key("insert"), widget.Secondary, icon.Square, "Insert", "").Tooltip("Insert a component into the mockup")
			}
			v.menu.OpenBelow(ins, "insert", theme.ControlHeight)
			v.menu.Show(c, ins, "insert", func(done func()) {
				for _, comp := range canvas.Components {
					if widget.MenuItem(c.Key(comp.Name), nil, comp.Name, false) {
						id, html := canvas.ComponentHTML(comp, f.HTML)
						v.sel = id
						v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpHTMLPatch, Target: id, HTML: html})
						done()
					}
				}
			})
		}
		ui.Spacer(c)
		// Undo and redo keep their shortcuts where the room is too short.
		if v.stageRoom() >= 520 {
			if widget.IconButton(c.Key("undo"), icon.Undo, "Undo ("+modKey+"Z)", false).Clicked() {
				v.undo(o, false)
			}
			if widget.IconButton(c.Key("redo"), icon.Redo, "Redo (Shift+"+modKey+"Z)", false).Clicked() {
				v.undo(o, true)
			}
		}
		if v.view == viewDesign {
			v.gridToggle(c, o, f)
		}
		if v.view == viewDesign && strings.TrimSpace(f.HTML) != "" {
			v.modeTools(c, o, f, narrow)
		}
		v.panelToggle(c, o)
	})
}

// panelToggle shows or hides the panel beside the stage; the agent's panel
// takes its place, so showing it closes the agent's.
func (v *View) panelToggle(c *ui.Context, o Options) {
	label := "Hide the panel"
	if v.panel == "" {
		label = "Show the panel"
	}
	if widget.IconButton(c.Key("panel"), icon.Layers, label, v.panel != "").Clicked() {
		switch {
		case v.panel != "":
			v.panelBack, v.panel = v.panel, ""
		default:
			v.assistant = false
			v.panel = cmpOr(v.panelBack, "layers")
			v.togglePanelLoad(o)
		}
	}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// modeTools are the preview's width, Edit or Interact, and starting the
// prototype over.
func (v *View) modeTools(c *ui.Context, o Options, f proto.CanvasFrame, narrow bool) {
	widths := []int{0, 1440, 768, 390}
	names := []string{"Its own width", "Desktop · 1440", "Tablet · 768", "Phone · 390"}
	cur := 0
	for i, w := range widths {
		if w == v.preview {
			cur = i
		}
	}
	// The preview's width, a menu: four segments don't fit a narrow page.
	pw := widget.IconButton(c.Key("preview"), icon.Laptop, "Preview width: "+names[cur], cur != 0)
	v.menu.OpenBelow(pw, "preview", theme.ControlHeight)
	v.menu.Show(c, pw, "preview", func(done func()) {
		for i, name := range names {
			if widget.MenuItem(c.Key(name), nil, name, false) {
				v.preview = widths[i]
				done()
			}
		}
	})
	if narrow {
		if widget.IconButton(c.Key("interact"), icon.Play, "Interact: use the mockup as the real thing", v.mode == modeInteract).Clicked() {
			v.mode = 1 - v.mode
		}
	} else if i := widget.Segmented(c.Key("mode"), []string{"Edit", "Interact"}, v.mode, nil); i >= 0 {
		v.mode = i
	}
	if v.mode == modeInteract && o.Web != nil && widget.IconButton(c.Key("reset"), icon.Undo, "Reset the prototype: its state, and back to the first design", false).Clicked() {
		v.resetPrototype(o)
	}
}

// viewSwitch shows the design, all of them, or those picked side by side.
func (v *View) viewSwitch(c *ui.Context, cv proto.Canvas) {
	canCompare := len(v.comparing(cv)) >= 2
	if v.width > 0 && v.pageRoom() < viewRoom {
		for i, b := range []struct {
			svg   *ui.SVG
			label string
		}{{icon.Square, "One design at a time"}, {icon.List, "All designs"}, {icon.Split, "Compare the designs picked"}} {
			btn := widget.IconButton(c.Key("view:"+b.label), b.svg, b.label, v.view == i)
			if i == viewCompare && !canCompare {
				btn.Disabled(true)
			} else if btn.Clicked() {
				v.view = i
			}
		}
		return
	}
	if i := widget.Segmented(c.Key("view"), []string{"Design", "All designs", "Compare"}, v.view, func(i int) bool {
		return i == viewCompare && !canCompare
	}); i >= 0 {
		v.view = i
	}
}

// palette is the shapes to add, and Connect, floating at the stage's left edge
// in Edit mode.
func (v *View) palette(c *ui.Context, o Options) {
	if v.mode != modeDesign {
		return
	}
	raised(ui.Column(c.Key("palette")).Absolute().Left(theme.S2).Top(theme.S2).Padding(theme.S1).Gap(theme.S1).AlignItems(ui.Center).Radius(theme.RadiusCard)).Children(func() {
		for _, t := range []struct {
			typ   string
			svg   *ui.SVG
			label string
		}{{proto.ShapeRect, icon.Square, "Rectangle"}, {proto.ShapeEllipse, icon.Circle, "Ellipse"}, {proto.ShapeDiamond, icon.Diamond, "Decision"},
			{proto.ShapeText, icon.Type, "Text"}, {proto.ShapeSticky, icon.Sticky, "Sticky note"}} {
			if widget.IconButton(c.Key(t.typ), t.svg, "Add "+strings.ToLower(t.label), false).Clicked() {
				v.addShape(o, t.typ)
			}
		}
		ui.Box(c.Key("rule")).Size(20, 0.5).Shrink(0).Background(theme.Border)
		label := "Connect: select a shape, then the one to point to"
		on := v.connect != ""
		if on {
			label = "Click the shape to point to"
		}
		if b := widget.IconButton(c.Key("connect"), icon.ArrowLink, label, on); b.Clicked() {
			switch {
			case on:
				v.connect = ""
			case v.sel != "":
				v.connect = v.sel
			}
		}
	})
}

// addShape adds a shape of typ in the frame's middle, selected.
func (v *View) addShape(o Options, typ string) {
	f := v.frameOf()
	el := proto.CanvasElement{Type: typ, W: 180, H: 90, X: float64(f.Width)/2 - 90, Y: float64(f.Height)/2 - 45}
	switch typ {
	case proto.ShapeText:
		el.Text, el.H = "Text", 40
	case proto.ShapeSticky:
		el.Text, el.W, el.H = "Note", 160, 160
	case proto.ShapeRect:
		el.Radius = 8
	}
	for n := len(f.Elements) + 1; ; n++ {
		id := fmt.Sprintf("%s-%d", typ, n)
		if _, taken := shapeOf(f, id); !taken {
			el.ID = id
			break
		}
	}
	v.sel = el.ID
	v.showLayers()
	v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpElementSet, Element: &el})
}

func shapeOf(f proto.CanvasFrame, id string) (proto.CanvasElement, bool) {
	for _, el := range f.Elements {
		if el.ID == id {
			return el, true
		}
	}
	return proto.CanvasElement{}, false
}

// keys are the page's shortcuts: undo, redo, delete the selection.
func (v *View) keys(c *ui.Context, o Options) {
	if c.Shortcut(ui.Cmd, ui.KeyZ) {
		v.undo(o, false)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyZ) {
		v.undo(o, true)
	}
	if v.editing == nil && v.sel != "" && v.mode == modeDesign && (c.Shortcut(0, ui.KeyDelete) || c.Shortcut(0, ui.KeyBackspace)) {
		v.remove(o, v.sel)
	}
	if v.connect != "" && c.Shortcut(0, ui.KeyEscape) {
		v.connect = ""
	}
}

// remove removes element id: a shape, or an element of the mockup.
func (v *View) remove(o Options, id string) {
	if _, ok := shapeOf(v.frameOf(), id); ok {
		v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpElementDel, Target: id})
	} else {
		v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpHTMLRemove, Target: id})
	}
	v.sel = ""
}
