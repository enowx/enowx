package canvasview

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// sidePanel is the panel beside the stage: the frame's layers and the
// selection's properties, the comments, or the versions.
func (v *View) sidePanel(c *ui.Context, o Options) {
	ui.Column(c.Key("canvas-panel")).Width(300).Shrink(0).FillHeight().Background(theme.Panel).BorderWidth(0, 0, 0, 1).BorderColor(theme.Border).
		Transition(widget.PanelMotion(16)).Children(func() {
		titles := map[string]string{"layers": "Layers", "comments": "Comments", "versions": "Versions", "grid": "Layout grid"}
		ui.Row(c).FillWidth().Height(40).Shrink(0).PaddingX(12).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
			ui.Text(c, titles[v.panel]).FontSize(theme.TextUI).FontWeight(600).Grow(1)
			if widget.IconButton(c, icon.Close, "Close panel", false).Clicked() {
				v.panel = ""
			}
		})
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(10).Gap(8).Children(func() {
				switch v.panel {
				case "layers":
					v.layers(c, o)
				case "comments":
					v.commentsPanel(c, o)
				case "versions":
					v.versions(c, o)
				case "grid":
					v.gridPanel(c, o)
				}
			})
		})
	})
}

// layers lists the frame's elements, the mockup's by id and the shapes;
// the one selected shows its properties.
func (v *View) layers(c *ui.Context, o Options) {
	f := v.frameOf()
	if v.sel != "" {
		v.inspector(c, o, f)
		widget.Divider(c)
	}
	row := func(key, id, kind, text string, depth int) {
		r := ui.ButtonBase(c.Key(key)).FillWidth().Height(26).Padding(0, 6, 0, 6+float32(depth)*12).Gap(6).Radius(theme.RadiusSmall).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Label("Layer " + id)
		switch {
		case v.sel == id:
			r.Background(theme.Selected)
		case v.recent(id):
			r.Background(theme.AccentSoft)
		case r.Hovered():
			r.Background(theme.Hover)
		}
		if r.Clicked() {
			v.sel = id
			if o.Web != nil {
				o.Web.outline(id)
			}
		}
		r.Children(func() {
			ui.Text(c, kind).FontSize(theme.TextLabel).TextColor(theme.Muted).Width(52).SingleLine()
			ui.Text(c, id).FontSize(theme.TextLabel).TextColor(theme.Text).SingleLine().Shrink(1)
			ui.Text(c, text).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1).Grow(1)
		})
	}
	out := canvas.Outline(f.HTML)
	if len(out) > 0 {
		widget.SectionLabel(c, "MOCKUP", theme.Muted)
		for _, el := range out {
			row("h:"+el.ID, el.ID, "<"+el.Tag+">", el.Text, el.Depth)
		}
	}
	if len(f.Elements) > 0 {
		widget.SectionLabel(c, "SHAPES", theme.Muted)
		for _, el := range f.Elements {
			row("s:"+el.ID, el.ID, el.Type, el.Text, 0)
		}
	}
	if len(out) == 0 && len(f.Elements) == 0 {
		ui.Text(c, "Nothing in this frame yet: add shapes from the toolbar, or ask an agent to design it.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
}

// inspector is the selected element's properties: a shape's text, place
// and colors; a mockup element's HTML.
func (v *View) inspector(c *ui.Context, o Options, f proto.CanvasFrame) {
	if el, ok := shapeOf(f, v.sel); ok {
		v.shapeInspector(c.Key("shape:"+el.ID), o, el)
		return
	}
	src, err := canvas.ElementHTML(f.HTML, v.sel)
	if err != nil {
		v.sel = ""
		return
	}
	insp := v.htmlInspector(v.sel, src)
	ui.Text(c, "#"+v.sel).FontSize(theme.TextLabel).FontWeight(600)
	ui.TextArea(c.Key("html:"+v.sel), &insp.text).FillWidth().Height(160).Font(theme.MonoFont).FontSize(theme.TextLabel).
		Padding(8).Radius(theme.RadiusSmall).Background(theme.Sunken).Border(1, theme.Border).Label("HTML of " + v.sel)
	ui.Row(c).FillWidth().Gap(6).Children(func() {
		save := widget.Button(c, widget.Primary, nil, "Apply", "")
		if insp.text == src {
			save.Disabled(true).Opacity(0.5)
		} else if save.Clicked() {
			v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpHTMLPatch, Target: v.sel, HTML: insp.text})
		}
		if o.AskAgent != nil && widget.Button(c, widget.Secondary, icon.Chat, "Ask agent", "").Label("Ask agent about "+v.sel).Clicked() {
			v.assistant, v.panel = true, "" // room for the agent
			o.AskAgent("@#" + v.sel + " ")
		}
		if widget.Button(c, widget.Secondary, icon.Comment, "Comment", "").Clicked() {
			v.panel = "comments"
			v.loadComments(o)
		}
		ui.Spacer(c)
		if widget.IconButton(c, icon.Trash, "Remove "+v.sel, false).Clicked() {
			v.remove(o, v.sel)
		}
	})
}

// htmlEdit is a mockup element's HTML being edited, from src.
type htmlEdit struct{ id, src, text string }

// htmlInspector is element id's HTML being edited: src anew when another
// element is selected, or it changed while not being edited.
func (v *View) htmlInspector(id, src string) *htmlEdit {
	e := v.htmlEdit
	if e == nil || e.id != id || e.src != src && e.text == e.src {
		e = &htmlEdit{id: id, src: src, text: src}
		v.htmlEdit = e
	}
	return e
}

// palette is the colors offered for shapes.
var palette = []string{"#ffffff", "#f1f5f9", "#dbeafe", "#dcfce7", "#fef9c3", "#fee2e2", "#ede9fe", "#0f172a"}

func (v *View) shapeInspector(c *ui.Context, o Options, el proto.CanvasElement) {
	set := func(change func(*proto.CanvasElement)) {
		x := el
		change(&x)
		v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpElementSet, Element: &x})
	}
	ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, el.ID).FontSize(theme.TextLabel).FontWeight(600).Grow(1)
		ui.Text(c, el.Type).FontSize(theme.TextLabel).TextColor(theme.Muted)
	})
	if el.Type != proto.ShapeLine && el.Type != proto.ShapeArrow {
		ui.Text(c, fmt.Sprintf("%g, %g · %g×%g", el.X, el.Y, el.W, el.H)).FontSize(theme.TextLabel).TextColor(theme.Muted)
	} else {
		ui.Text(c, el.From+" → "+el.To).FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	if el.Type != proto.ShapeImage {
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			if widget.Button(c, widget.Secondary, icon.Pencil, "Edit text", "").Clicked() {
				v.editing = &textEdit{id: el.ID, text: el.Text}
			}
			bold := widget.IconButton(c, icon.Type, "Bold", el.Bold)
			if bold.Clicked() {
				set(func(x *proto.CanvasElement) { x.Bold = !x.Bold })
			}
		})
	}
	if el.Type != proto.ShapeLine && el.Type != proto.ShapeArrow && el.Type != proto.ShapeImage {
		ui.Text(c, "Fill").FontSize(theme.TextLabel).TextColor(theme.Muted)
		swatches(c.Key("fill"), el.Fill, func(col string) { set(func(x *proto.CanvasElement) { x.Fill = col }) })
	}
	ui.Text(c, "Line").FontSize(theme.TextLabel).TextColor(theme.Muted)
	swatches(c.Key("stroke"), el.Stroke, func(col string) { set(func(x *proto.CanvasElement) { x.Stroke = col }) })
	ui.Row(c).FillWidth().Gap(6).Children(func() {
		if o.AskAgent != nil && widget.Button(c, widget.Secondary, icon.Chat, "Ask agent", "").Label("Ask agent about "+v.sel).Clicked() {
			v.assistant, v.panel = true, "" // room for the agent
			o.AskAgent("@#" + v.sel + " ")
		}
		if widget.Button(c, widget.Secondary, icon.Comment, "Comment", "").Clicked() {
			v.panel = "comments"
			v.loadComments(o)
		}
		ui.Spacer(c)
		if widget.IconButton(c, icon.Trash, "Remove "+el.ID, false).Clicked() {
			v.remove(o, el.ID)
		}
	})
}

func swatches(c *ui.Context, cur string, pick func(string)) {
	ui.Row(c).FillWidth().Gap(5).Children(func() {
		for _, col := range append([]string{"none"}, palette...) {
			s := ui.ButtonBase(c.Key(col)).Size(20, 20).Radius(theme.RadiusSmall).Border(1, theme.BorderStrong).Cursor(ui.CursorPointer).Label("Color " + col)
			if col == "none" {
				s.Children(func() { ui.Icon(c, icon.Slash).FontSize(12).TextColor(theme.Muted) }).Center()
			} else {
				s.Background(color(col, ui.Color{}))
			}
			if strings.EqualFold(col, cur) {
				s.Shadow(0, 0, 0, 2, theme.Accent)
			}
			if s.Clicked() {
				pick(col)
			}
		}
	})
}

// commentsPanel is the comments, the open ones first, and one to write on
// the selection.
func (v *View) commentsPanel(c *ui.Context, o Options) {
	on := "the frame"
	if v.sel != "" {
		on = "#" + v.sel
	}
	ui.TextArea(c.Key("draft"), &v.draft).FillWidth().Height(64).FontSize(theme.TextLabel).Padding(8).Radius(theme.RadiusSmall).
		Background(theme.Sunken).Border(1, theme.Border).Placeholder("Comment on " + on + ": agents read it").Label("New comment")
	add := widget.Button(c, widget.Primary, icon.Comment, "Comment", "").Label("Add comment")
	if strings.TrimSpace(v.draft) == "" {
		add.Disabled(true).Opacity(0.5)
	} else if add.Clicked() && v.canvas != nil {
		cm := proto.CanvasComment{Text: v.draft, Element: v.sel, Frame: v.frameOf().ID}
		v.draft = ""
		o.Comment(proto.CanvasCommentParams{ID: v.canvas.ID, Op: proto.CanvasCommentAdd, Comment: cm}, v.tookComments)
	}
	if len(v.comments) == 0 {
		ui.Text(c, "No comments yet. Select an element and say what to change: an agent asked to work on the comments reads them.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	for _, resolved := range []bool{false, true} {
		for _, cm := range v.comments {
			if cm.Resolved == resolved && v.onDesign(cm) {
				v.comment(c.Key(cm.ID), o, cm)
			}
		}
	}
}

func (v *View) tookComments(list []proto.CanvasComment, err error) {
	if err != nil {
		v.err = err.Error()
		return
	}
	v.comments = list
}

func (v *View) comment(c *ui.Context, o Options, cm proto.CanvasComment) {
	card := ui.Column(c).FillWidth().Padding(10).Gap(6).Radius(theme.RadiusSmall).Background(theme.Raised).Border(1, theme.Border)
	if cm.Resolved {
		card.Opacity(0.55)
	}
	card.Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			who := "You"
			if cm.Author != "" {
				who = "Agent"
				if o.AgentName != nil {
					who = o.AgentName(cm.Author)
				}
			}
			ui.Text(c, who).FontSize(theme.TextLabel).FontWeight(600)
			if cm.Element != "" {
				on := ui.ButtonBase(c).Cursor(ui.CursorPointer).Label("Select " + cm.Element)
				on.Children(func() { ui.Text(c, "#"+cm.Element).FontSize(theme.TextLabel).TextColor(theme.Accent) })
				if on.Clicked() {
					v.sel = cm.Element
					if o.Web != nil {
						o.Web.outline(cm.Element)
					}
				}
			}
			ui.Spacer(c)
			ui.Text(c, ago(cm.Created)).FontSize(theme.TextLabel).TextColor(theme.Muted)
		})
		ui.Text(c, cm.Text).FontSize(theme.TextLabel).TextColor(theme.Text)
		ui.Row(c).FillWidth().Gap(4).Children(func() {
			op, label := proto.CanvasCommentResolve, "Resolve"
			if cm.Resolved {
				op, label = proto.CanvasCommentReopen, "Reopen"
			}
			if widget.GhostButton(c, label).Clicked() {
				o.Comment(proto.CanvasCommentParams{ID: v.canvas.ID, Op: op, Comment: proto.CanvasComment{ID: cm.ID}}, v.tookComments)
			}
			if o.SendAgent != nil && !cm.Resolved && widget.GhostButton(c, "Send to agent").Clicked() {
				v.assistant, v.panel = true, "" // room for the agent
				on := "the design"
				if cm.Element != "" {
					on = "#" + cm.Element
				}
				o.SendAgent("Do comment " + cm.ID + " on " + on + ": " + cm.Text + "\nThen resolve it (canvas_resolve).")
			}
			if widget.GhostButton(c, "Delete").Clicked() {
				o.Comment(proto.CanvasCommentParams{ID: v.canvas.ID, Op: proto.CanvasCommentDelete, Comment: proto.CanvasComment{ID: cm.ID}}, v.tookComments)
			}
		})
	})
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	}
	return t.Local().Format("Jan 2")
}

// onDesign says comment cm is on the design shown, or on the canvas as a
// whole; with several designs, the others' go with theirs.
func (v *View) onDesign(cm proto.CanvasComment) bool {
	f := v.frameOf()
	return cm.Frame == "" || cm.Frame == f.ID || strings.EqualFold(cm.Frame, f.Name)
}
