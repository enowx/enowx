package ui

import (
	"slices"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/chatview"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/unslopview"
	"github.com/enowx/enowx/ui/widget"
)

// A fanned-out unslop's region agents, followed in the canvas's agent
// panel: a click on one in the Unslop panel shows its conversation there,
// live, the Canvas page staying open; a bar on top goes back to the main
// agent or on to another region's.

// viewRegionAgent shows region agent session id in the canvas's agent
// panel.
func (a *App) viewRegionAgent(id string) {
	cv := a.canvases.Canvas()
	if cv == nil {
		return
	}
	a.canvasChatOf(cv.ID).viewing = id
	a.canvases.OpenAssistant()
}

// regionAgent is the region agent shown in the panel of canvas cv, if any.
func (a *App) regionAgent(cv proto.Canvas, hc *hostCanvasChat) (proto.SessionInfo, bool) {
	if hc.viewing == "" {
		return proto.SessionInfo{}, false
	}
	s, ok := a.sessionByID(hc.viewing)
	if !ok || s.CanvasFor != cv.ID {
		hc.viewing = ""
		return proto.SessionInfo{}, false
	}
	return s, true
}

// canvasRegionView is region agent s's conversation in the panel.
func (a *App) canvasRegionView(c *ui.Context, cv proto.Canvas, hc *hostCanvasChat, s proto.SessionInfo) {
	a.regionBar(c, cv, hc, s)
	shown := func() bool {
		return a.page == pageCanvas && a.canvases.AssistantOpen() && a.canvases.Selected() == cv.ID && hc.viewing == s.ID
	}
	switch p := a.panes[s.ID]; {
	case chatview.Supported(s):
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			a.chatViewSending(c.Key("chat:"+s.ID), s, shown, chatTweaks{showUser: canvasRegionPrompt})
		})
	case p != nil:
		terminal.View(c.Key("term:"+s.ID), p.term).Grow(1).FillWidth()
	default:
		ui.Text(c, "Connecting…").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S4)
	}
}

// canvasRegionPrompt shows a region agent's first prompt, long and for
// the agent, as the line saying what it does.
func canvasRegionPrompt(text string) string {
	if len(text) > 160 {
		return text[:160] + "…"
	}
	return text
}

// regionBar is the panel's bar over a region agent: back to the main
// agent, the regions to go to, and this one's state.
func (a *App) regionBar(c *ui.Context, cv proto.Canvas, hc *hostCanvasChat, s proto.SessionInfo) {
	run, hasRun := a.unslopRuns[cv.ID]
	ui.Column(c.Key("region-bar")).FillWidth().Shrink(0).Padding(theme.S2, theme.S3).Gap(theme.S2).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			back := ui.ButtonBase(c.Key("back")).Height(theme.ControlHeight).Padding(0, theme.S3, 0, theme.S2).Gap(theme.S1).Radius(theme.RadiusControl).AlignItems(ui.Center).
				Label("Back to the main agent").Cursor(ui.CursorPointer)
			if back.Hovered() {
				back.Background(theme.Hover)
			}
			back.Children(func() {
				ui.Icon(c, icon.ArrowLeft).FontSize(14).TextColor(theme.TextSoft)
				ui.Text(c, "Main agent").FontSize(theme.TextUI).FontWeight(500).TextColor(theme.TextSoft).SingleLine()
			})
			if back.Clicked() {
				hc.viewing = ""
			}
			ui.Spacer(c)
			label, col := regionStateLabel(a.agentState(s.ID))
			if hasRun {
				if i := slices.IndexFunc(run.Regions, func(r proto.UnslopRegion) bool { return r.Session == s.ID }); i >= 0 {
					switch r := run.Regions[i]; {
					case r.Done:
						label, col = "done", theme.Done
					case r.Pass != "" && label == "working":
						label = r.Pass
					}
				}
			}
			widget.Tag(c, label, col)
		})
		if !hasRun || len(run.Regions) == 0 {
			widget.Running(c.Key("name"), s.Name, widget.RunStyle{Weight: 500})
			return
		}
		// Every region's agent, the one shown lit: one click to the next.
		ui.Row(c.Key("regions")).FillWidth().Gap(theme.S1).Wrap().Children(func() {
			for i, r := range run.Regions {
				on := r.Session == s.ID
				chip := ui.ButtonBase(c.Key("r:" + r.ID)).Height(24).MaxWidth(200).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusSmall).AlignItems(ui.Center).
					Label("Show the agent of #" + r.ID).Cursor(ui.CursorPointer).Background(theme.Sunken)
				if on {
					chip.Background(theme.Selected)
				} else if chip.Hovered() {
					chip.Background(theme.Hover)
				}
				chip.Children(func() {
					ui.Box(c).Size(8, 8).Shrink(0).Radius(theme.RadiusFull).Background(ui.Hex(unslopview.RegionColors[i%len(unslopview.RegionColors)]))
					ui.Row(c).Shrink(1).MinWidth(0).Children(func() {
						widget.Running(c, "#"+r.ID, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
					})
					if r.Done {
						ui.Icon(c, icon.Check).FontSize(12).TextColor(theme.Done)
					}
				})
				if chip.Clicked() {
					hc.viewing = r.Session
				}
			}
		})
	})
}

// regionStateLabel is what an agent state reads as, and its color.
func regionStateLabel(state string) (string, ui.Color) {
	switch state {
	case unslopview.AgentWaiting:
		return "needs you", theme.Warning
	case unslopview.AgentIdle:
		return "idle", theme.Muted
	case unslopview.AgentStopped:
		return "stopped", theme.Error
	}
	return "working", theme.Accent
}

// regionViewing is the region agent shown for the canvas open, "" none.
func (a *App) regionViewing() string {
	if cv := a.canvases.Canvas(); cv != nil && a.canvasChat != nil && a.canvasChat.canvas == cv.ID {
		return a.canvasChat.viewing
	}
	return ""
}
