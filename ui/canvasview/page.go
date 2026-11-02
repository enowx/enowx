package canvasview

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// page is the canvas open: its header; under it the agent's bar while one
// designs it, the tools, the stage and the designs' strip; the panel beside.
func (v *View) page(c *ui.Context, o Options) {
	cv := v.canvas
	v.header(c, o, *cv)
	// Keyed: the agent's bar and an error come and go above what's under them,
	// and without a key what it holds (the strip's and grid's scroll) would
	// start over each time.
	ui.Row(c.Key("canvas-body")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c.Key("canvas-main")).Grow(1).MinWidth(0).FillHeight().Padding(theme.S3).Gap(theme.S2).Children(func() {
			v.agentBar(c, o, *cv)
			v.errorBar(c)
			v.toolbar(c, o, *cv)
			ui.Row(c.Key("canvas-views")).Grow(1).FillWidth().MinHeight(0).Children(func() {
				switch v.view {
				case viewGrid:
					v.grid(c, o, *cv)
				case viewCompare:
					v.compareView(c, o, *cv)
				default:
					ui.Column(c.Key("stage-column")).Grow(1).MinWidth(0).FillHeight().Children(func() {
						v.stage(c, o)
						v.designLine(c, o)
						if len(cv.Frames) > 1 && v.stageRoom() >= stripRoom {
							v.strip(c, o, *cv)
						}
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

// sizeOf is a design's size, as its caption and the header say it.
func sizeOf(f proto.CanvasFrame) string { return fmt.Sprintf("%d×%d", f.Width, f.Height) }

// raised is what floats over the stage or the page's views: a pane of the
// menus' material with an overlay's edge.
func raised(e ui.Element) ui.Element { return widget.Rim(e.Background(theme.Popover)) }

// errorBar says what failed last, until dismissed.
func (v *View) errorBar(c *ui.Context) {
	if v.err == "" {
		return
	}
	widget.Banner(c.Key("canvas-error"), theme.Error, v.err, func() {
		if widget.IconButton(c, icon.Close, "Dismiss", false).Clicked() {
			v.err = ""
		}
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
	bar := ui.Row(c.Key("agent-bar")).FillWidth().Shrink(0).MinHeight(40).Padding(0, theme.S1, 0, theme.S4).Gap(theme.S2).Radius(theme.RadiusControl).
		Background(theme.Fade(theme.Accent, 0.14))
	if cv.Paused {
		bar.Background(theme.Fade(theme.Warning, 0.14))
	}
	bar.Children(func() {
		if cv.Paused {
			ui.Icon(c, icon.Pause).FontSize(14).TextColor(theme.Warning)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, "Agents are paused on this canvas: their changes wait until you resume", widget.RunStyle{})
			})
			if widget.Button(c, widget.Primary, icon.Play, "Resume", "").Clicked() {
				o.Control(cv.ID, proto.CanvasResume, v.reportErr)
			}
			return
		}
		widget.Spinner(c, 14)
		what := ""
		if v.last.Element != "" && now.Sub(v.lastAt) < designingFor {
			what = " · " + v.last.Element
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, who+" is designing"+what, widget.RunStyle{}) })
		if widget.Button(c, widget.Secondary, icon.Pause, "Pause", "").Clicked() {
			o.Control(cv.ID, proto.CanvasPause, v.reportErr)
		}
		if widget.Button(c, widget.Secondary, icon.Stop, "Stop", "").Tooltip("Interrupt the agent, and pause").Clicked() {
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
		v.panelBack, v.panel = id, ""
		return
	}
	v.panel = id
	v.togglePanelLoad(o)
}

// togglePanelLoad reads what the panel now shown lists.
func (v *View) togglePanelLoad(o Options) {
	switch v.panel {
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
