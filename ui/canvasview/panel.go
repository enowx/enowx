package canvasview

import (
	"fmt"
	"slices"
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

// Tabs of the panel beside the stage.
var panelTabs = []string{"layers", "comments", "versions", "grid"}

// sidePanel is the panel beside the stage: the frame's layers and the
// selection's properties, the comments, the versions, or the grid. The toolbar
// shows it again once hidden.
func (v *View) sidePanel(c *ui.Context, o Options) {
	ui.Column(c.Key("canvas-panel")).Width(panelWidth).Shrink(0).FillHeight().BorderWidth(0, 0, 0, theme.Rule).BorderColor(theme.Border).
		Transition(widget.PanelMotion(16)).Children(func() {
		ui.Row(c.Key("panel-tabs")).FillWidth().Shrink(0).Padding(theme.S3, theme.S3, theme.S2, theme.S3).Gap(theme.S1).AlignItems(ui.Center).Children(func() {
			comments := "Comments"
			if n := v.openComments(); n > 0 {
				comments = fmt.Sprintf("Comments %d", n)
			}
			if i := panelSegments(c.Key("tabs"), []string{"Layers", comments, "Versions", "Grid"}, slices.Index(panelTabs, v.panel)); i >= 0 {
				v.panel = panelTabs[i]
				v.togglePanelLoad(o)
			}
		})
		ui.Scroll(c.Key("panel-body")).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(0, theme.S3, theme.S3, theme.S3).Gap(theme.S2).Children(func() {
				switch v.panel {
				case "layers":
					v.layers(c, o)
				case "comments":
					v.commentsList(c, o)
				case "versions":
					v.versions(c, o)
				case "grid":
					v.gridPanel(c, o)
				}
			})
		})
		if v.panel == "comments" {
			v.commentComposer(c, o)
		}
	})
}

// panelSegments are the panel's tabs, each as wide as its word needs: at equal
// widths "Comments" wouldn't fit the panel, though all four do.
func panelSegments(c *ui.Context, labels []string, current int) int {
	clicked := -1
	ui.Row(c).FillWidth().MinWidth(0).Padding(theme.S1).Radius(theme.RadiusFull).Background(theme.Sunken).Children(func() {
		for i, label := range labels {
			b := ui.ButtonBase(c.Key(i)).Grow(float32(len(label) + 1)).Basis(0).MinWidth(0).Height(28).PaddingX(2).Radius(theme.RadiusFull).Label(label)
			bg, fg, weight := theme.Fade(theme.Selected, 0), theme.TextSoft, 400
			switch {
			case i == current:
				bg, fg, weight = theme.Selected, theme.TextStrong, 500
			case b.Hovered():
				bg, fg = theme.Hover, theme.Text
			}
			if i != current {
				b.Cursor(ui.CursorPointer)
			}
			b.Background(widget.QuickTint(b, "bg", bg)).Children(func() {
				widget.Running(c, label, widget.RunStyle{Color: fg, Weight: weight})
			})
			if i != current && b.Clicked() {
				clicked = i
			}
		}
	})
	return clicked
}

// showLayers shows the Layers panel, or has it show once the agent's closed.
func (v *View) showLayers() {
	if v.assistant {
		v.panelBack = "layers"
		return
	}
	v.panel = "layers"
}

// layerRow is a layer of the frame: a card in the panel, indented by its depth.
func (v *View) layerRow(c *ui.Context, o Options, key, id, kind, text string, depth int) {
	r := ui.ButtonBase(c.Key(key)).FillWidth().MinHeight(44).Shrink(0).Padding(theme.S1, theme.S3, theme.S1, theme.S3+float32(min(depth, 4))*theme.S2).Gap(theme.S2).Radius(theme.RadiusCard).
		Justify(ui.Start).Cursor(ui.CursorPointer).Label("Layer " + id)
	switch {
	case v.sel == id:
		r.Background(theme.Selected)
	case v.recent(id):
		r.Background(theme.AccentSoft)
	case r.Hovered():
		r.Background(theme.Hover)
	default:
		widget.Raise(r.Background(theme.Raised))
	}
	if r.Clicked() {
		v.sel = id
		if o.Web != nil {
			o.Web.outline(id)
		}
	}
	r.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, "#"+id, widget.RunStyle{Always: v.sel == id})
			sub := kind
			if text != "" {
				sub += " · " + text
			}
			widget.Running(c.Key("sub"), sub, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
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
	out := canvas.Outline(f.HTML)
	if len(out) > 0 {
		widget.SectionLabel(c.Key("mockup"), "Mockup", theme.TextSoft)
		for _, el := range out {
			v.layerRow(c, o, "h:"+el.ID, el.ID, "<"+el.Tag+">", el.Text, el.Depth)
		}
	}
	if len(f.Elements) > 0 {
		widget.SectionLabel(c.Key("shapes"), "Shapes", theme.TextSoft)
		for _, el := range f.Elements {
			v.layerRow(c, o, "s:"+el.ID, el.ID, el.Type, el.Text, 0)
		}
	}
	if len(out) == 0 && len(f.Elements) == 0 {
		ui.Text(c, "Nothing in this frame yet: add shapes from the toolbar, or ask an agent to design it.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
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
	widget.Running(c.Key("html-id"), "#"+v.sel, widget.RunStyle{Weight: 500})
	ui.Column(c.Key("html-code")).FillWidth().Height(160).Shrink(0).Radius(theme.RadiusCard).Clip().Background(theme.CodeGround).Children(func() {
		ui.TextArea(c.Key("html:"+v.sel), &insp.text).FillWidth().FillHeight().Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(widget.CodeText).
			Padding(theme.S2, theme.S3).Label("HTML of " + v.sel)
	})
	ui.Row(c.Key("html-actions")).FillWidth().Gap(theme.S1).Wrap().AlignItems(ui.Center).Children(func() {
		save := widget.Button(c.Key("apply"), widget.Primary, nil, "Apply", "")
		if insp.text == src {
			save.Disabled(true)
		} else if save.Clicked() {
			v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpHTMLPatch, Target: v.sel, HTML: insp.text})
		}
		v.selectionActions(c, o, v.sel)
	})
}

// selectionActions are what an element selected can go on to: the agent, a
// comment, or away.
func (v *View) selectionActions(c *ui.Context, o Options, id string) {
	if o.AskAgent != nil && widget.Button(c.Key("ask"), widget.Secondary, nil, "Ask agent", "").Label("Ask agent about "+id).Clicked() {
		v.OpenAssistant()
		o.AskAgent("@#" + id + " ")
	}
	if widget.IconButton(c.Key("comment"), icon.Comment, "Comment", false).Clicked() {
		v.panel = "comments"
		v.loadComments(o)
	}
	if widget.IconButton(c.Key("remove"), icon.Trash, "Remove "+id, false).Clicked() {
		v.remove(o, id)
	}
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
	ui.Row(c.Key("shape-head")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, el.ID, widget.RunStyle{Weight: 500}) })
		ui.Text(c, el.Type).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
	})
	where := el.From + " → " + el.To
	if el.Type != proto.ShapeLine && el.Type != proto.ShapeArrow {
		where = fmt.Sprintf("%g, %g · %g×%g", el.X, el.Y, el.W, el.H)
	}
	widget.Running(c.Key("where"), where, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
	if el.Type != proto.ShapeImage {
		ui.Row(c.Key("shape-text")).FillWidth().Gap(theme.S1).AlignItems(ui.Center).Children(func() {
			if widget.Button(c.Key("edit"), widget.Secondary, nil, "Edit text", "").Clicked() {
				v.editing = &textEdit{id: el.ID, text: el.Text}
			}
			if widget.IconButton(c.Key("bold"), icon.Type, "Bold", el.Bold).Clicked() {
				set(func(x *proto.CanvasElement) { x.Bold = !x.Bold })
			}
		})
	}
	if el.Type != proto.ShapeLine && el.Type != proto.ShapeArrow && el.Type != proto.ShapeImage {
		ui.Text(c.Key("fill-l"), "Fill").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		swatches(c.Key("fill"), el.Fill, func(col string) { set(func(x *proto.CanvasElement) { x.Fill = col }) })
	}
	ui.Text(c.Key("line-l"), "Line").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	swatches(c.Key("stroke"), el.Stroke, func(col string) { set(func(x *proto.CanvasElement) { x.Stroke = col }) })
	ui.Row(c.Key("shape-actions")).FillWidth().Gap(theme.S1).Wrap().AlignItems(ui.Center).Children(func() {
		v.selectionActions(c, o, el.ID)
	})
}

func swatches(c *ui.Context, cur string, pick func(string)) {
	ui.Row(c).FillWidth().Gap(theme.S1).Wrap().Children(func() {
		for _, col := range append([]string{"none"}, palette...) {
			s := ui.ButtonBase(c.Key(col)).Size(20, 20).Radius(theme.RadiusFull).Border(0.5, theme.BorderStrong).Cursor(ui.CursorPointer).Label("Color " + col)
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

// commentsList is the comments, the open ones first.
func (v *View) commentsList(c *ui.Context, o Options) {
	if len(v.comments) == 0 {
		ui.Text(c, "No comments yet. Select an element and say what to change: an agent asked to work on the comments reads them.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
	}
	for _, resolved := range []bool{false, true} {
		for _, cm := range v.comments {
			if cm.Resolved == resolved && v.onDesign(cm) {
				v.comment(c.Key(cm.ID), o, cm)
			}
		}
	}
}

// commentComposer is one to write on the selection, at the panel's foot.
func (v *View) commentComposer(c *ui.Context, o Options) {
	on := "the frame"
	if v.sel != "" {
		on = "#" + v.sel
	}
	ui.Column(c.Key("comment-new")).FillWidth().Shrink(0).Padding(theme.S2, theme.S3, theme.S3, theme.S3).Gap(theme.S2).Children(func() {
		ta := ui.TextArea(c.Key("draft"), &v.draft).FillWidth().Height(60).FontSize(theme.TextUI).TextColor(theme.Text).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).
			Placeholder("Comment on " + on + ": agents read it").Label("New comment")
		widget.Well(ta, false)
		ui.Row(c.Key("add")).FillWidth().Justify(ui.End).Children(func() {
			add := widget.Button(c, widget.Primary, nil, "Comment", "").Label("Add comment")
			if strings.TrimSpace(v.draft) == "" {
				add.Disabled(true)
			} else if add.Clicked() && v.canvas != nil {
				cm := proto.CanvasComment{Text: v.draft, Element: v.sel, Frame: v.frameOf().ID}
				v.draft = ""
				o.Comment(proto.CanvasCommentParams{ID: v.canvas.ID, Op: proto.CanvasCommentAdd, Comment: cm}, v.tookComments)
			}
		})
	})
}

func (v *View) tookComments(list []proto.CanvasComment, err error) {
	if err != nil {
		v.err = err.Error()
		return
	}
	v.comments = list
}

func (v *View) comment(c *ui.Context, o Options, cm proto.CanvasComment) {
	card := widget.Raise(ui.Column(c).FillWidth().Shrink(0).Padding(theme.S2, theme.S3).Gap(theme.S1).Radius(theme.RadiusCard).Background(theme.Raised))
	if cm.Resolved {
		card.Opacity(0.55)
	}
	card.Children(func() {
		ui.Row(c.Key("head")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			who := "You"
			if cm.Author != "" {
				who = "Agent"
				if o.AgentName != nil {
					who = o.AgentName(cm.Author)
				}
			}
			ui.Text(c.Key("who"), who).FontSize(theme.TextUI).FontWeight(500).SingleLine().Shrink(0)
			if cm.Element != "" {
				on := ui.ButtonBase(c.Key("on")).Cursor(ui.CursorPointer).Label("Select " + cm.Element).MinWidth(0).Shrink(1)
				on.Children(func() { widget.Running(c, "#"+cm.Element, widget.RunStyle{Size: theme.TextLabel, Color: theme.Accent}) })
				if on.Clicked() {
					v.sel = cm.Element
					if o.Web != nil {
						o.Web.outline(cm.Element)
					}
				}
			}
			ui.Spacer(c)
			ui.Text(c.Key("ago"), ago(cm.Created)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		})
		ui.Text(c.Key("text"), cm.Text).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		ui.Row(c.Key("actions")).FillWidth().Gap(theme.S1).AlignItems(ui.Center).Children(func() {
			op, label := proto.CanvasCommentResolve, "Resolve"
			if cm.Resolved {
				op, label = proto.CanvasCommentReopen, "Reopen"
			}
			if widget.GhostButton(c.Key("state"), label).Clicked() {
				o.Comment(proto.CanvasCommentParams{ID: v.canvas.ID, Op: op, Comment: proto.CanvasComment{ID: cm.ID}}, v.tookComments)
			}
			ui.Spacer(c)
			if o.SendAgent != nil && !cm.Resolved && widget.IconButton(c.Key("send"), icon.Chat, "Send to agent", false).Clicked() {
				v.OpenAssistant()
				on := "the design"
				if cm.Element != "" {
					on = "#" + cm.Element
				}
				o.SendAgent("Do comment " + cm.ID + " on " + on + ": " + cm.Text + "\nThen resolve it (canvas_resolve).")
			}
			if widget.IconButton(c.Key("delete"), icon.Trash, "Delete", false).Clicked() {
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
