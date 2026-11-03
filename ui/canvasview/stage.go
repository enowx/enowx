package canvasview

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The stage shows the frame open, scaled to fit: its mockup in a web view,
// its shapes drawn over it, each one an element the user selects, moves,
// resizes and edits in Design mode.

// dragState is a shape being moved or resized: where it shows meanwhile.
type dragState struct {
	id         string
	resize     bool
	x, y, w, h float64
	moved      bool
}

// textEdit is a shape's text being edited in place.
type textEdit struct {
	id   string
	text string
}

// highlightFor is how long an agent's change stays outlined.
const highlightFor = 1600 * time.Millisecond

func color(s string, fallback ui.Color) ui.Color {
	if c, ok := canvas.ParseColor(s); ok {
		return ui.RGB(c.R, c.G, c.B)
	}
	return fallback
}

// stage draws the frame open: on the endless board in Design mode, as a
// page to use in Interact mode.
func (v *View) stage(c *ui.Context, o Options) {
	f := v.frameOf()
	width, height := float64(f.Width), float64(f.Height)
	hasHTML := strings.TrimSpace(f.HTML) != ""
	if v.preview > 0 && hasHTML {
		height = math.Max(height*float64(v.preview)/width, height)
		width = float64(v.preview)
	}
	if o.Web != nil {
		o.Web.setDesign(v.mode == modeDesign)
		if v.canvas != nil {
			o.Web.show(*v.canvas, f, nil) // the design picked, when it isn't the one loaded
		}
	}
	if v.mode == modeInteract && hasHTML && o.Web != nil {
		v.interactStage(c, o, float32(width), float32(height))
		return
	}
	// The zoom's controls sit beside the board, over it, so its pointer
	// handling never takes their clicks.
	wrap := ui.Box(c.Key("stage-wrap")).Grow(1).FillWidth().MinHeight(0)
	var area ui.Element
	wrap.Children(func() {
		area = ui.Box(c.Key("stage")).FillWidth().FillHeight().Background(theme.Sunken).Radius(theme.RadiusCard).Clip()
	})
	b := area.Bounds()
	if key := fmt.Sprintf("%s/%v", f.ID, width); v.cam.key != key {
		if b.W <= 0 {
			// Placed once the stage has a size: the next frame.
			c.After(time.Millisecond)
			return
		}
		v.cam.place(v.selected, f.ID, key, b.W, float32(width))
	}
	v.scale = v.cam.z
	s := float64(v.cam.z)
	design := ui.Rect{X: v.cam.x, Y: v.cam.y, W: float32(width * s), H: float32(height * s)}
	// A press on the page, or on a shape, is theirs; elsewhere the board's.
	onDesign := func(x, y float32) bool {
		if hasHTML || v.panel == "grid" && v.gridF != nil && v.gridF.drawing {
			return x >= design.X && y >= design.Y && x < design.X+design.W && y < design.Y+design.H
		}
		px, py := float64((x-design.X)/v.cam.z), float64((y-design.Y)/v.cam.z)
		for _, el := range f.Elements {
			el = v.placed(el)
			if el.Type == proto.ShapeLine || el.Type == proto.ShapeArrow {
				continue
			}
			if px >= el.X-6 && py >= el.Y-6 && px < el.X+el.W+6 && py < el.Y+el.H+6 {
				return true
			}
		}
		return false
	}
	area.HandleInput(v.cameraInput(0, 0, onDesign, func(float32, float32) {
		if v.mode == modeDesign {
			v.deselect(o)
		}
	}))
	if v.cam.panning {
		area.Cursor(ui.CursorGrabbing)
	}
	now := time.Now()
	area.Children(func() {
		v.dots(c)
		// A mockup's page shows as a sheet; shapes alone lie on the board
		// itself, free form, with nothing drawn under them.
		board := ui.Box(c.Key("board:"+f.ID)).Absolute().Left(design.X).Top(design.Y).Size(design.W, design.H).Label("Frame " + f.Name)
		if hasHTML {
			board.Background(color(f.Background, ui.RGB(255, 255, 255))).Clip()
			widget.DropShadow(board, 2, 12, theme.Fade(theme.TextStrong, 0.12))
		}
		if v.mode == modeDesign && board.Clicked() {
			v.deselect(o)
		}
		board.Children(func() {
			if hasHTML {
				v.mockup(c, o, f, design, s)
			}
			drawing := v.panel == "grid" && v.gridF != nil && v.gridF.drawing
			if !drawing {
				v.gridGuides(c, o, f, s)
			}
			v.arrows(c, f, s)
			for _, el := range f.Elements {
				if el.Type == proto.ShapeLine || el.Type == proto.ShapeArrow {
					v.arrowHandle(c.Key(el.ID), o, f, el, s)
					continue
				}
				v.shape(c.Key(el.ID), o, el, s, now)
			}
			if drawing {
				v.gridGuides(c, o, f, s)
			}
		})
	})
	wrap.Children(func() {
		v.palette(c, o)
		v.zoomBar(c, b.W, b.H, float32(width), float32(height))
		v.unslopHUD(c, o, f)
		if hasHTML && o.Web != nil {
			o.Web.measureWidth(c)
			if n := o.Web.Wider(); n > 0 {
				// Past the palette, which keeps the stage's corner in Edit mode.
				raised(ui.Row(c.Key("wider")).Absolute().Left(theme.S6+theme.S5).Top(theme.S2).Right(theme.S2).MaxWidth(420).Height(theme.ControlHeight).Padding(0, theme.S3).Gap(theme.S2).AlignItems(ui.Center).
					Radius(theme.RadiusFull)).Tooltip("Something sticks out past the design's width, so its page scrolls sideways: fix it, or ask the agent to").
					Children(func() {
						ui.Icon(c, icon.Alert).FontSize(14).TextColor(theme.Warning)
						ui.Row(c).Grow(1).MinWidth(0).Children(func() {
							widget.Running(c, fmt.Sprintf("Wider than its frame by %d px: it scrolls sideways", n), widget.RunStyle{Size: theme.TextLabel, Color: theme.Warning})
						})
					})
			}
		}
	})
	if v.lastAt.Add(highlightFor).After(now) {
		c.After(highlightFor - now.Sub(v.lastAt) + 10*time.Millisecond)
	}
}

// deselect selects nothing.
func (v *View) deselect(o Options) {
	v.sel, v.connect = "", ""
	if o.Web != nil {
		o.Web.outline("")
	}
}

// interactStage shows the mockup as a page to use: as wide as the stage
// lets it (its own width at most), scrolled as a browser scrolls.
func (v *View) interactStage(c *ui.Context, o Options, width, height float32) {
	area := ui.Box(c.Key("stage")).Grow(1).FillWidth().MinHeight(0).Background(theme.Sunken).Radius(theme.RadiusCard).Clip()
	b := area.Bounds()
	// A moving design keeps room for its controls from the start, so it doesn't
	// shift once its page says it has motion.
	room := float32(0)
	if v.usesMotion(v.frameOf().HTML) {
		room = motionBarRoom
	}
	box := interactBox(b.W, b.H-room, width, height)
	v.scale = box.Zoom
	area.Children(func() {
		if b.W > 0 {
			o.Web.view(c.Key("web"), box.W, box.H, float64(box.Zoom)).Absolute().Left(box.X).Top(box.Y)
			o.Web.motionBar(c, b.W, b.H)
		}
	})
	o.Web.pollGoto(c, v.followLink)
}

// interactMargin is what Interact mode leaves around a design.
const interactMargin = 24

// interactPlace is where Interact mode shows a design, in stage pixels.
type interactPlace struct {
	X, Y, W, H, Zoom float32
}

// interactBox fits a design width×height in a stage aw×ah: whole, centered,
// its margin all round, never above its own size, so its page's 100vh is its
// frame's height. A web view zoomed out too far lays its page out anew, so it
// stops at liveZoom: what doesn't fit then is cut by the box, and the page
// scrolls to it.
func interactBox(aw, ah, width, height float32) interactPlace {
	room := max(aw-2*interactMargin, 1)
	tall := max(ah-2*interactMargin, 1)
	z := max(min(room/width, tall/height, 1), liveZoom)
	w, h := min(width*z, room), min(height*z, tall)
	return interactPlace{X: (aw - w) / 2, Y: (ah - h) / 2, W: w, H: h, Zoom: z}
}

// mockup is the frame's HTML: in a web view, or, where there is none,
// its elements listed as they come. In Design mode a cover over the page
// takes its pointer, so the board moves and zooms over it too, and a click
// on it selects nothing.
func (v *View) mockup(c *ui.Context, o Options, f proto.CanvasFrame, design ui.Rect, s float64) {
	w, h := design.W, design.H
	if o.Web != nil {
		// Small, the design shows as its picture, rendered at its own
		// width: a web view zoomed out that far lays its page out anew,
		// narrower, rather than shrinking it. The live page shows from
		// liveZoom up, and until the picture comes.
		var img ui.ImageSource
		if s < liveZoom && v.preview == 0 {
			img = v.picture(c, o, f)
		}
		if img != nil {
			ui.Image(c.Key("design-picture"), img).Absolute().Left(0).Top(0).Size(w, h).Label("Picture of " + f.Name)
		} else {
			o.Web.view(c.Key("web"), w, h, s).Absolute().Left(0).Top(0)
		}
		// A click picks the component under it, for the user to ask about,
		// change or note: on the live page.
		cover := ui.Box(c.Key("web-cover")).Absolute().Left(0).Top(0).Size(w, h).Label("Design " + f.Name).Cursor(ui.CursorPointer).
			HandleInput(v.cameraInput(design.X, design.Y, nil, func(x, y float32) {
				if img != nil {
					// Too small to pick from: zoom in there, to the page.
					v.cam.zoomAt(design.X+x, design.Y+y, liveZoom/float32(s))
					return
				}
				v.pickAt(o, x/float32(s), y/float32(s))
			}))
		cover.Hovered() // takes the pointer, over the page
		if img != nil {
			cover.Tooltip("Click to zoom in and pick its components")
		}
		v.componentBar(c, o, s, w, h)
		return
	}
	ui.Column(c.Key("outline")).Absolute().Left(0).Top(0).Size(w, h).Padding(theme.S4).Gap(theme.S1).Children(func() {
		ui.Text(c, "This device shows mockups in a web view: here, their elements.").FontSize(theme.TextLabel).TextColor(ui.RGB(100, 116, 139))
		for _, el := range v.outlineOf(o, f.HTML) {
			row := ui.Row(c.Key(el.ID)).Padding(2, 6, 2, 6+float32(el.Depth)*12).Radius(theme.RadiusTail).Gap(theme.S2)
			if v.recent(el.ID) {
				row.Background(ui.RGBA(99, 102, 241, 0.18))
			}
			row.Children(func() {
				ui.Text(c, "#"+el.ID).FontSize(theme.TextLabel).TextColor(ui.RGB(79, 70, 229))
				ui.Text(c, widget.LongLine(el.Text)).FontSize(theme.TextLabel).TextColor(ui.RGB(15, 23, 42)).SingleLine().Shrink(1)
			})
		}
	})
}

// recent says element id is the one an agent changed moments ago.
func (v *View) recent(id string) bool {
	return id != "" && v.last.Element == id && time.Since(v.lastAt) < highlightFor
}

// placed is where shape el shows: as it is, or as being dragged.
func (v *View) placed(el proto.CanvasElement) proto.CanvasElement {
	if d := v.drag; d != nil && d.id == el.ID {
		el.X, el.Y, el.W, el.H = d.x, d.y, d.w, d.h
		el = snapped(v.frameOf(), el) // where it goes, on the grid
	}
	return el
}

// shape draws a shape and handles the pointer on it.
func (v *View) shape(c *ui.Context, o Options, el proto.CanvasElement, s float64, now time.Time) {
	shown := v.placed(el)
	st := canvas.StyleOf(shown)
	box := ui.Box(c).Absolute().Left(float32(shown.X*s)).Top(float32(shown.Y*s)).Size(float32(shown.W*s), float32(shown.H*s)).
		Label(el.ID)
	fill, stroke := color(st.Fill, ui.Color{}), color(st.Stroke, ui.Color{})
	switch el.Type {
	case proto.ShapeEllipse, proto.ShapeDiamond:
		typ, width := el.Type, float32(st.StrokeWidth*s)
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			var path ui.Path
			if typ == proto.ShapeEllipse {
				ellipsePath(&path, r)
			} else {
				path.MoveTo(r.X+r.W/2, r.Y).LineTo(r.X+r.W, r.Y+r.H/2).LineTo(r.X+r.W/2, r.Y+r.H).LineTo(r.X, r.Y+r.H/2).Close()
			}
			if st.Fill != "" {
				p.FillPath(&path, fill)
			}
			if st.Stroke != "" {
				p.StrokePath(&path, width, stroke)
			}
		})
	case proto.ShapeImage:
		box.Background(ui.RGB(226, 232, 240)).Radius(float32(el.Radius * s))
	default:
		if st.Fill != "" {
			box.Background(fill)
		}
		if st.Stroke != "" {
			box.Border(float32(math.Max(st.StrokeWidth*s, 1)), stroke)
		}
		box.Radius(float32(el.Radius * s))
		if el.Type == proto.ShapeSticky {
			box.Shadow(0, 1, 4, 0, ui.RGBA(15, 23, 42, 0.15))
		}
	}
	selected := v.sel == el.ID
	if v.recent(el.ID) {
		box.Shadow(0, 0, 0, 3, theme.Accent)
	} else if selected {
		box.Shadow(0, 0, 0, 2, ui.RGB(99, 102, 241))
	} else if v.connect != "" && box.Hovered() {
		box.Shadow(0, 0, 0, 2, ui.RGBA(99, 102, 241, 0.5))
	}
	if v.mode == modeDesign {
		box.Cursor(ui.CursorPointer)
		v.handlePointer(c, o, box, el, s)
	}
	box.Children(func() {
		switch {
		case v.editing != nil && v.editing.id == el.ID:
			in := ui.TextInput(c.Key("edit"), &v.editing.text).FillWidth().FillHeight().FontSize(float32(st.FontSize * s)).
				TextColor(color(st.Color, ui.RGB(15, 23, 42))).Padding(theme.S1).AutoFocus().Label("Text of " + el.ID)
			if in.Submitted() || in.Shortcut(0, ui.KeyEscape) || !in.Focused() && v.editing.text != el.Text {
				el.Text = v.editing.text
				v.editing = nil
				v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpElementSet, Element: &el})
			}
		case el.Type == proto.ShapeImage:
			ui.Column(c).FillWidth().FillHeight().Center().Gap(theme.S1).Children(func() {
				ui.Icon(c, icon.File).FontSize(float32(math.Max(14*s, 10))).TextColor(ui.RGB(100, 116, 139))
				ui.Text(c, el.Src).FontSize(float32(math.Max(11*s, 9))).TextColor(ui.RGB(100, 116, 139)).SingleLine()
			})
		case el.Text != "":
			shapeText(c, shown, st, s)
		}
		if selected && v.mode == modeDesign && v.editing == nil {
			v.resizeHandle(c, o, el, s)
		}
		if v.recent(el.ID) && v.last.Session != "" {
			who := "Agent"
			if o.AgentName != nil {
				who = o.AgentName(v.last.Session)
			}
			ui.Row(c.Key("who")).Absolute().Left(0).Top(-22).Height(18).PaddingX(theme.S2).Radius(theme.RadiusTail).Background(theme.Accent).Children(func() {
				ui.Text(c, who).FontSize(theme.TextLabel).TextColor(ui.RGB(255, 255, 255)).SingleLine()
			})
		}
	})
	_ = now
}

func shapeText(c *ui.Context, el proto.CanvasElement, st canvas.Style, s float64) {
	align := ui.Center
	switch el.Align {
	case "left":
		align = ui.Start
	case "right":
		align = ui.End
	}
	ui.Column(c).FillWidth().FillHeight().Padding(float32(8 * s)).Justify(ui.Center).AlignItems(align).Children(func() {
		t := ui.Text(c, el.Text).FontSize(float32(st.FontSize * s)).TextColor(color(st.Color, ui.RGB(15, 23, 42))).TextAlign(align)
		if el.Bold {
			t.FontWeight(700)
		}
	})
}

func ellipsePath(p *ui.Path, r ui.Rect) {
	const k = 0.5523
	cx, cy, rx, ry := r.X+r.W/2, r.Y+r.H/2, r.W/2, r.H/2
	p.MoveTo(cx+rx, cy).
		CubeTo(cx+rx, cy+ry*k, cx+rx*k, cy+ry, cx, cy+ry).
		CubeTo(cx-rx*k, cy+ry, cx-rx, cy+ry*k, cx-rx, cy).
		CubeTo(cx-rx, cy-ry*k, cx-rx*k, cy-ry, cx, cy-ry).
		CubeTo(cx+rx*k, cy-ry, cx+rx, cy-ry*k, cx+rx, cy).Close()
}

// handlePointer selects shape el on a click, connects an arrow to it, edits
// its text on a double click, and moves it as it is dragged.
func (v *View) handlePointer(c *ui.Context, o Options, box ui.Element, el proto.CanvasElement, s float64) {
	if dx, dy, ok := box.Dragged(); ok {
		if v.drag == nil || v.drag.id != el.ID {
			v.drag = &dragState{id: el.ID, x: el.X, y: el.Y, w: el.W, h: el.H}
		}
		if (dx != 0 || dy != 0) && !v.drag.resize {
			v.drag.x += float64(dx) / s
			v.drag.y += float64(dy) / s
			v.drag.moved = true
		}
	} else if d := v.drag; d != nil && d.id == el.ID && !d.resize {
		v.drag = nil
		if d.moved {
			el.X, el.Y = math.Round(d.x), math.Round(d.y)
			v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpElementSet, Element: &el})
		}
	}
	if box.DoubleClicked() && el.Type != proto.ShapeImage {
		v.sel, v.editing = el.ID, &textEdit{id: el.ID, text: el.Text}
		return
	}
	if box.Clicked() {
		if v.connect != "" && v.connect != el.ID {
			arrow := proto.CanvasElement{Type: proto.ShapeArrow, From: v.connect, To: el.ID}
			v.connect = ""
			v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpElementSet, Element: &arrow})
			return
		}
		v.sel = el.ID
		v.showLayers()
	}
}

// resizeHandle is the selected shape's corner, which drags its size.
func (v *View) resizeHandle(c *ui.Context, o Options, el proto.CanvasElement, s float64) {
	h := ui.Box(c.Key("resize")).Absolute().Right(-5).Bottom(-5).Size(10, 10).Radius(theme.RadiusTail).
		Background(ui.RGB(255, 255, 255)).Border(1.5, ui.RGB(99, 102, 241)).Cursor(ui.CursorResizeNWSE).Label("Resize " + el.ID)
	if dx, dy, ok := h.Dragged(); ok {
		if v.drag == nil || v.drag.id != el.ID || !v.drag.resize {
			v.drag = &dragState{id: el.ID, resize: true, x: el.X, y: el.Y, w: el.W, h: el.H}
		}
		v.drag.w = math.Max(v.drag.w+float64(dx)/s, 8)
		v.drag.h = math.Max(v.drag.h+float64(dy)/s, 8)
		v.drag.moved = v.drag.moved || dx != 0 || dy != 0
	} else if d := v.drag; d != nil && d.id == el.ID && d.resize {
		v.drag = nil
		if d.moved {
			el.W, el.H = math.Round(d.w), math.Round(d.h)
			v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpElementSet, Element: &el})
		}
	}
}

// arrows draws the frame's lines and arrows, following the shapes they
// join as those move.
func (v *View) arrows(c *ui.Context, f proto.CanvasFrame, s float64) {
	shown := f
	shown.Elements = make([]proto.CanvasElement, len(f.Elements))
	for i, el := range f.Elements {
		shown.Elements[i] = v.placed(el)
	}
	ui.Box(c.Key("arrows")).Absolute().Left(0).Top(0).FillWidth().FillHeight().PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		for _, el := range shown.Elements {
			if el.Type != proto.ShapeLine && el.Type != proto.ShapeArrow {
				continue
			}
			st := canvas.StyleOf(el)
			col := color(st.Stroke, ui.RGB(71, 85, 105))
			if v.sel == el.ID || v.recent(el.ID) {
				col = ui.RGB(99, 102, 241)
			}
			x1, y1, x2, y2 := canvas.Ends(shown, el)
			w := float32(math.Max(st.StrokeWidth*s, 1))
			p.Line(r.X+float32(x1*s), r.Y+float32(y1*s), r.X+float32(x2*s), r.Y+float32(y2*s), w, col)
			if el.Type == proto.ShapeArrow {
				ax, ay, bx, by := canvas.ArrowHead(x1, y1, x2, y2, 8+st.StrokeWidth*2)
				var head ui.Path
				head.MoveTo(r.X+float32(x2*s), r.Y+float32(y2*s)).LineTo(r.X+float32(ax*s), r.Y+float32(ay*s)).
					LineTo(r.X+float32(bx*s), r.Y+float32(by*s)).Close()
				p.FillPath(&head, col)
			}
		}
	})
}

// arrowHandle is an arrow's label at its middle, which selects it.
func (v *View) arrowHandle(c *ui.Context, o Options, f proto.CanvasFrame, el proto.CanvasElement, s float64) {
	shown := f
	shown.Elements = make([]proto.CanvasElement, len(f.Elements))
	for i, x := range f.Elements {
		shown.Elements[i] = v.placed(x)
	}
	x1, y1, x2, y2 := canvas.Ends(shown, el)
	mx, my := (x1+x2)/2*s, (y1+y2)/2*s
	st := canvas.StyleOf(el)
	label := ui.Row(c).Absolute().Left(float32(mx) - 60).Top(float32(my) - 11).Width(120).Height(22).Justify(ui.Center).AlignItems(ui.Center).Label(el.ID)
	if v.mode == modeDesign {
		label.Cursor(ui.CursorPointer)
		if label.Clicked() {
			v.sel = el.ID
			v.showLayers()
		}
	}
	label.Children(func() {
		if el.Text == "" {
			return
		}
		ui.Text(c, el.Text).FontSize(float32(st.FontSize * s * 0.85)).TextColor(color(st.Color, ui.RGB(15, 23, 42))).
			Background(ui.RGBA(255, 255, 255, 0.9)).PaddingX(theme.S1).Radius(theme.RadiusTail).SingleLine()
	})
	_ = o
}

// resetPrototype takes a prototype back to its start: nothing remembered, on
// its first design.
func (v *View) resetPrototype(o Options) {
	o.Web.ResetState()
	if v.canvas != nil && len(v.canvas.Frames) > 0 {
		v.frame, v.sel = v.canvas.Frames[0].ID, ""
	}
}

// followLink shows the design a mockup's link goes to: its number
// ("design-2"), name or ID.
func (v *View) followLink(target string) {
	if v.canvas == nil {
		return
	}
	if i, err := canvas.FrameIndex(*v.canvas, strings.TrimPrefix(target, "#")); err == nil {
		v.frame, v.sel = v.canvas.Frames[i].ID, ""
	}
}
