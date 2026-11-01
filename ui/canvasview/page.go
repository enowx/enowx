package canvasview

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// page is the canvas open: its header, the agent's bar while one designs
// it, the tools, the stage, and the panel beside it.
func (v *View) page(c *ui.Context, o Options) {
	cv := v.canvas
	v.header(c, o, *cv)
	v.agentBar(c, o, *cv)
	if v.err != "" {
		ui.Row(c).FillWidth().PaddingX(14).Height(30).AlignItems(ui.Center).Gap(8).Background(theme.Fade(theme.Error, 0.1)).Children(func() {
			ui.Text(c, v.err).FontSize(theme.TextLabel).TextColor(theme.Error).Grow(1).SingleLine()
			if widget.IconButton(c, icon.Close, "Dismiss", false).Clicked() {
				v.err = ""
			}
		})
	}
	// Keyed: the agent's bar and an error come and go above it, and
	// without a key what it holds (the strip's and grid's scroll) would
	// start over each time.
	ui.Row(c.Key("canvas-body")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			v.toolbar(c, o, *cv)
			ui.Row(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				switch v.view {
				case viewGrid:
					v.grid(c, o, *cv)
				case viewCompare:
					v.compareView(c, o, *cv)
				default:
					v.palette(c, o)
					if len(cv.Frames) > 1 && v.stageRoom() >= stripRoom {
						v.strip(c, o, *cv)
					}
					ui.Column(c.Key("stage-column")).Grow(1).MinWidth(0).FillHeight().Children(func() { // the strip comes and goes before it
						v.designLine(c, o)
						v.stage(c, o)
					})
				}
				v.buildHUD(c, o, *cv)
			})
		})
		if v.panel != "" {
			v.sidePanel(c, o)
		}
	})
	v.keys(c, o)
	v.designKeys(c)
}

// header is the canvas's title, its frames and what is done with it.
func (v *View) header(c *ui.Context, o Options, cv proto.Canvas) {
	ui.Row(c).FillWidth().Height(44).Shrink(0).PaddingX(14).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		if !v.listShown() || v.listOpen {
			// The canvases' list, hidden in a narrow page, a click away.
			if widget.IconButton(c.Key("list"), icon.Sidebar, "Canvases", v.listOpen).Clicked() {
				v.listOpen = !v.listOpen
			}
		}
		ui.Icon(c, kindIcon(cv.Kind)).FontSize(15).TextColor(theme.Accent)
		t := ui.ButtonBase(c).Label("Title").Cursor(ui.CursorText).Shrink(1).MinWidth(0)
		t.Children(func() {
			ui.Text(c, cv.Title).FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong).SingleLine()
		})
		if t.DoubleClicked() {
			title := cv.Title
			v.renaming = &title
		}
		if !v.compact() {
			ui.Text(c, fmt.Sprintf("%s · v%d", cv.Kind, cv.Version)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		}
		ui.Spacer(c)
		// Undo and redo keep their shortcuts where the room is too short.
		if v.stageRoom() >= 520 {
			if widget.IconButton(c, icon.Undo, "Undo ("+modKey+"Z)", false).Clicked() {
				v.undo(o, false)
			}
			if widget.IconButton(c, icon.Redo, "Redo (Shift+"+modKey+"Z)", false).Clicked() {
				v.undo(o, true)
			}
		}
		for _, p := range []struct {
			id    string
			svg   *ui.SVG
			label string
		}{{"layers", icon.Layers, "Layers"}, {"comments", icon.Comment, "Comments"}, {"versions", icon.History, "Versions"}} {
			label := p.label
			if p.id == "comments" {
				if n := v.openComments(); n > 0 {
					label = fmt.Sprintf("Comments (%d open)", n)
				}
			}
			if widget.IconButton(c.Key(p.id), p.svg, label, v.panel == p.id).Clicked() {
				v.togglePanel(o, p.id)
			}
		}
		if cv.Kind == proto.CanvasMockup || cv.Kind == proto.CanvasWireframe {
			v.briefButton(c, cv)
		}
		if cv.Kind == proto.CanvasMockup && o.Build != nil && o.CanAsk {
			v.buildButton(c, o)
		}
		if f := v.frameOf(); o.Unslop != nil && o.CanAsk && f.HTML != "" {
			tip := "Unslop: find what makes this design look AI-made, and have the agent redesign a copy of it, pass by pass"
			var b ui.Element
			if v.compact() {
				b = widget.IconButton(c.Key("unslop"), icon.Wand, tip, false)
			} else {
				b = widget.Button(c.Key("unslop"), widget.Secondary, icon.Wand, "Unslop", "").Height(28).PaddingX(10).Tooltip(tip)
			}
			if o.Unslopping {
				b.Disabled(true)
			} else if b.Clicked() {
				o.Unslop(f.ID)
				v.assistant, v.panel = true, ""
			}
			if o.UnslopChoose != nil && !o.Unslopping && widget.IconButton(c.Key("unslop-passes"), icon.ChevronDown, "Unslop with the passes you pick…", false).Clicked() {
				o.UnslopChoose(f.ID)
			}
		}
		if o.CanAsk && widget.IconButton(c, icon.Chat, "Ask an agent", v.assistant).Clicked() {
			v.assistant = !v.assistant
			if v.assistant {
				v.panel = "" // room for the agent
			}
		}
		var exp ui.Element
		if v.compact() {
			exp = widget.IconButton(c.Key("export"), icon.Download, "Export", false)
		} else {
			exp = widget.Button(c.Key("export"), widget.Secondary, icon.Download, "Export", "").Height(28).PaddingX(10)
		}
		v.menu.OpenBelow(exp, "export", 28)
		v.menu.Show(c, exp, "export", func(done func()) {
			f := v.frameOf()
			// A still of a moving design is one moment of it; only HTML moves.
			still, moving := "", ""
			if canvas.UsesMotion(f.HTML) {
				still, moving = " (one frame)", " (keeps the motion)"
			}
			for _, e := range []struct{ label, format, frame string }{
				{"PNG of " + f.Name + still, proto.RenderPNG, f.ID}, {"SVG of " + f.Name + still, proto.RenderSVG, f.ID},
				{"PDF of " + f.Name + still, proto.RenderPDF, f.ID}, {"HTML of " + f.Name + moving, proto.RenderHTML, f.ID},
				{"Share as a page (read-only HTML)…", proto.RenderHTML, ""},
			} {
				if widget.MenuItem(c.Key(e.label), icon.Download, e.label, false) {
					v.export(o, cv, e.frame, e.format)
					done()
				}
			}
			if o.Share != nil && widget.MenuItem(c.Key("share-link"), icon.Link, "Share a link…", false) {
				v.openShare(o, cv.ID)
				done()
			}
		})
		if o.MakeCode != nil {
			var mk ui.Element
			if v.compact() {
				mk = widget.IconButton(c.Key("make-code"), icon.Code, "Make code", false)
			} else {
				mk = widget.Button(c.Key("make-code"), widget.Primary, icon.Code, "Make code", "").Height(28).PaddingX(10).
					Tooltip("Have an agent build it in a project, as components")
			}
			v.menu.OpenBelow(mk, "make-code", 28)
			v.menu.Show(c, mk, "make-code", func(done func()) {
				if len(o.Projects) == 0 {
					ui.Text(c, "Open a project on this host first").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(8)
				}
				for _, p := range o.Projects {
					label := p.Name
					if containsPath(cv.Projects, p.Path) {
						label += " · attached"
					}
					if widget.MenuItem(c.Key(p.Path), icon.Folder, label, false) {
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
		more := widget.IconButton(c, icon.More, "More", false)
		v.menu.OpenBelow(more, "more", 28)
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
	})
}

func containsPath(list []string, p string) bool {
	for _, x := range list {
		if x == p {
			return true
		}
	}
	return false
}

// modKey is the shortcuts' modifier as menus show it.
var modKey = "⌘"

// agentBar says an agent designs the canvas now, with pause and stop; or
// that the user paused agents, with resume.
func (v *View) agentBar(c *ui.Context, o Options, cv proto.Canvas) {
	now := time.Now()
	active := v.designing(now)
	if !active && !cv.Paused {
		return
	}
	if active {
		c.After(designingFor - now.Sub(cv.AgentAt) + 50*time.Millisecond)
	}
	who := "An agent"
	if o.AgentName != nil && cv.Agent != "" {
		who = o.AgentName(cv.Agent)
	}
	ui.Row(c.Key("agent-bar")).FillWidth().Height(36).Shrink(0).PaddingX(14).Gap(8).AlignItems(ui.Center).
		Background(theme.AccentSoft).BorderWidth(0, 0, 1, 0).BorderColor(theme.AccentLine).Children(func() {
		if cv.Paused {
			ui.Icon(c, icon.Pause).FontSize(13).TextColor(theme.Warning)
			ui.Text(c, "Agents are paused on this canvas: their changes wait until you resume").FontSize(theme.TextLabel).TextColor(theme.Text).Grow(1).SingleLine()
			if widget.Button(c, widget.Primary, icon.Play, "Resume", "").Height(26).PaddingX(10).Clicked() {
				o.Control(cv.ID, proto.CanvasResume, v.reportErr)
			}
			return
		}
		widget.Spinner(c, 12)
		what := ""
		if v.last.Element != "" && now.Sub(v.lastAt) < designingFor {
			what = " · " + v.last.Element
		}
		ui.Text(c, who+" is designing"+what).FontSize(theme.TextLabel).TextColor(theme.Text).Grow(1).SingleLine()
		if widget.Button(c, widget.Secondary, icon.Pause, "Pause", "").Height(26).PaddingX(10).Clicked() {
			o.Control(cv.ID, proto.CanvasPause, v.reportErr)
		}
		if widget.Button(c, widget.Secondary, icon.Stop, "Stop", "").Height(26).PaddingX(10).Tooltip("Interrupt the agent, and pause").Clicked() {
			o.Control(cv.ID, proto.CanvasStop, v.reportErr)
		}
	})
}

func (v *View) reportErr(err error) {
	if err != nil {
		v.err = err.Error()
	}
}

func (v *View) undo(o Options, redo bool) {
	if v.canvas == nil {
		return
	}
	fn := o.Undo
	if redo {
		fn = o.Redo
	}
	fn(v.canvas.ID, func(c proto.Canvas, err error) {
		if err != nil {
			v.err = err.Error()
			return
		}
		v.show(o, c)
	})
}

// export renders frame (or, for a page to share, the canvas) as format,
// and saves it where the user says.
func (v *View) export(o Options, cv proto.Canvas, frame, format string) {
	name := cv.Title
	if frame != "" {
		name += " · " + v.frameOf().Name
	}
	o.Render(cv.ID, frame, format, func(data []byte, err error) {
		if err != nil {
			v.err = err.Error()
			return
		}
		o.SaveFile(name+"."+format, data)
	})
}

func (v *View) togglePanel(o Options, id string) {
	if v.panel == id {
		v.panel = ""
		return
	}
	v.panel = id
	switch id {
	case "comments":
		v.loadComments(o)
	case "versions":
		v.loadHistory(o)
	}
}

func (v *View) openComments() int {
	n := 0
	for _, cm := range v.comments {
		if !cm.Resolved {
			n++
		}
	}
	return n
}
