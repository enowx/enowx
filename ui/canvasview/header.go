package canvasview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// header is the frame's top: the canvas's title and the design's size, the
// view of its designs, and what is done with it.
func (v *View) header(c *ui.Context, o Options, cv proto.Canvas) {
	f := v.frameOf()
	ui.Row(c.Key("canvas-header")).FillWidth().MinHeight(56).Shrink(0).Padding(theme.S2, theme.S4, theme.S2, theme.S5).Gap(theme.S2).AlignItems(ui.Center).
		BorderWidth(0, 0, theme.Rule, 0).BorderColor(theme.Border).Children(func() {
		if !v.listOutside && (!v.listShown() || v.listOpen) {
			// The canvases' list, hidden in a narrow page, a click away.
			if widget.IconButton(c.Key("list"), icon.Sidebar, "Canvases", v.listOpen).Clicked() {
				v.listOpen = !v.listOpen
			}
		}
		// Its title keeps some room from the actions, which scroll when short of it.
		ui.Row(c.Key("heading")).Grow(1).MinWidth(headingRoom).Gap(theme.S3).AlignItems(ui.Center).Children(func() {
			t := ui.ButtonBase(c).Label("Title").Cursor(ui.CursorText).Shrink(1).MinWidth(0)
			t.Children(func() {
				widget.Running(c, cv.Title, widget.RunStyle{Size: theme.TextTitle, Weight: 600, Color: theme.TextStrong, Always: true})
			})
			if t.DoubleClicked() {
				title := cv.Title
				v.renaming = &title
			}
			if f.Width > 0 && !v.compact() {
				ui.Row(c.Key("size")).MinWidth(0).Shrink(100).Children(func() {
					widget.Running(c, sizeOf(f), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
			}
		})
		// In a page too narrow for them all, they scroll in what room there is.
		var actions ui.Element
		if v.width > 0 && v.pageRoom() < actionsRoom {
			actions = ui.ScrollHorizontal(c.Key("actions")).Shrink(1).MinWidth(0)
		} else {
			actions = ui.Row(c.Key("actions")).Shrink(0)
		}
		actions.Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			v.viewSwitch(c, cv)
			if cv.Kind == proto.CanvasMockup || cv.Kind == proto.CanvasWireframe {
				v.briefButton(c, cv)
			}
			if cv.Kind == proto.CanvasMockup && o.Build != nil && o.CanAsk {
				v.buildButton(c, o)
			}
			v.unslopButton(c, o, f)
			v.exportMenu(c, o, cv, f)
			v.makeCodeMenu(c, o, cv)
			if o.Share != nil {
				var b ui.Element
				if v.compact() {
					b = widget.IconButton(c.Key("share-link"), icon.Link, "Share", false)
				} else {
					b = widget.GhostButton(c.Key("share-link"), "Share")
				}
				if b.Clicked() {
					v.openShare(o, cv.ID)
				}
			}
			if o.CanAsk && widget.IconButton(c.Key("agent"), icon.Chat, "Ask an agent", v.assistant).Clicked() {
				if v.assistant {
					v.CloseAssistant()
				} else {
					v.OpenAssistant()
				}
			}
			v.moreMenu(c, o, cv)
		})
	})
}

// unslopButton starts an unslop of design f, or first lets the user pick its
// passes.
func (v *View) unslopButton(c *ui.Context, o Options, f proto.CanvasFrame) {
	if o.Unslop == nil || !o.CanAsk || f.HTML == "" {
		return
	}
	tip := "Unslop: find what makes this design look AI-made, and have the agent redesign a copy of it, pass by pass"
	var b ui.Element
	if v.compact() {
		b = widget.IconButton(c.Key("unslop"), icon.Wand, tip, false)
	} else {
		b = widget.GhostButton(c.Key("unslop"), "Unslop").Tooltip(tip)
	}
	if o.Unslopping {
		b.Disabled(true)
	} else if b.Clicked() {
		o.Unslop(f.ID)
		v.OpenAssistant()
	}
	if o.UnslopChoose != nil && !o.Unslopping && widget.IconButton(c.Key("unslop-passes"), icon.ChevronDown, "Unslop with the passes you pick…", false).Clicked() {
		o.UnslopChoose(f.ID)
	}
}

// exportMenu saves the design shown as a file, or the canvas as a page.
func (v *View) exportMenu(c *ui.Context, o Options, cv proto.Canvas, f proto.CanvasFrame) {
	var exp ui.Element
	if v.compact() {
		exp = widget.IconButton(c.Key("export"), icon.Download, "Export", false)
	} else {
		exp = widget.MenuButton(c.Key("export"), widget.Secondary, "Export")
	}
	v.menu.OpenBelow(exp, "export", theme.ControlHeight)
	v.menu.Show(c, exp, "export", func(done func()) {
		// A still of a moving design is one moment of it; only HTML moves.
		still, moving := "", ""
		if v.usesMotion(f.HTML) {
			still, moving = "One frame", "Keeps the motion"
		}
		for _, e := range []struct{ label, format, sub string }{
			{"PNG", proto.RenderPNG, still}, {"PDF", proto.RenderPDF, still}, {"SVG", proto.RenderSVG, still}, {"HTML", proto.RenderHTML, moving},
		} {
			picked := false
			if e.sub != "" {
				picked = widget.MenuItemSub(c.Key(e.label), nil, e.label, e.sub, false)
			} else {
				picked = widget.MenuItem(c.Key(e.label), nil, e.label, false)
			}
			if picked {
				v.export(o, cv, f.ID, e.format)
				done()
			}
		}
		v.videoMenuItem(c, o, f, done)
		if widget.MenuItem(c.Key("page"), nil, "Share as a page (read-only HTML)…", false) {
			v.export(o, cv, "", proto.RenderHTML)
			done()
		}
	})
	v.videoDialog(c, o, cv)
}

// makeCodeMenu has an agent build the canvas in a project.
func (v *View) makeCodeMenu(c *ui.Context, o Options, cv proto.Canvas) {
	if o.MakeCode == nil {
		return
	}
	var mk ui.Element
	if v.compact() {
		mk = widget.IconButton(c.Key("make-code"), icon.Code, "Make code", false)
	} else {
		mk = widget.MenuButton(c.Key("make-code"), widget.Secondary, "Make code").Tooltip("Have an agent build it in a project, as components")
	}
	v.menu.OpenBelow(mk, "make-code", theme.ControlHeight)
	v.menu.Show(c, mk, "make-code", func(done func()) {
		if len(o.Projects) == 0 {
			ui.Text(c, "Open a project on this host first").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S1, theme.S3)
		}
		for _, p := range o.Projects {
			label := p.Name
			if containsPath(cv.Projects, p.Path) {
				label += " · attached"
			}
			if widget.MenuItem(c.Key(p.Path), nil, label, false) {
				o.MakeCode(cv, p.Path)
				if !containsPath(cv.Projects, p.Path) {
					projects := append(append([]string{}, cv.Projects...), p.Path)
					v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpMeta, Projects: &projects})
				}
				done()
			}
		}
	})
}

// moreMenu is what is done with the canvas itself.
func (v *View) moreMenu(c *ui.Context, o Options, cv proto.Canvas) {
	more := widget.IconButton(c.Key("more"), icon.More, "More for this canvas", false)
	v.menu.OpenBelow(more, "more", theme.ControlHeight)
	v.menu.Show(c, more, "more", func(done func()) {
		if widget.MenuItem(c.Key("rename"), icon.Pencil, "Rename…", false) {
			title := cv.Title
			v.renaming = &title
			done()
		}
		for _, p := range o.Projects {
			on := containsPath(cv.Projects, p.Path)
			label := "Attach to " + p.Name
			if on {
				label = "Detach from " + p.Name
			}
			if widget.MenuItem(c.Key("attach:"+p.Path), icon.Folder, label, false) {
				var projects []string
				for _, x := range cv.Projects {
					if x != p.Path {
						projects = append(projects, x)
					}
				}
				if !on {
					projects = append(projects, p.Path)
				}
				v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpMeta, Projects: &projects})
				done()
			}
		}
		if o.MoveSpace != nil {
			widget.MenuSeparator(c)
			if widget.MenuItem(c.Key("space"), icon.ArrowRight, "Move or copy to a space…", false) {
				o.MoveSpace(cv.CanvasInfo)
				done()
			}
		}
		widget.MenuSeparator(c)
		if widget.MenuItem(c.Key("delete"), icon.Trash, "Delete…", true) {
			info := cv.CanvasInfo
			v.deleting = &info
			done()
		}
	})
}
