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
		ui.Text(c, "Connecting…").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(16)
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
	ui.Column(c).FillWidth().Shrink(0).Padding(8, 10).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			back := ui.ButtonBase(c.Key("back")).Height(28).Padding(0, 10, 0, 6).Gap(6).Radius(theme.RadiusControl).AlignItems(ui.Center).
				Label("Back to the main agent").Cursor(ui.CursorPointer)
			if back.Hovered() {
				back.Background(theme.Hover)
			}
			back.Children(func() {
				ui.Icon(c, icon.ArrowLeft).FontSize(13).TextColor(theme.TextSoft)
				ui.Text(c, "Main agent").FontSize(theme.TextLabel).FontWeight(600).TextColor(theme.TextSoft)
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
			ui.Text(c, s.Name).FontSize(theme.TextUI).FontWeight(600).SingleLine()
			return
		}
		// Every region's agent, the one shown lit: one click to the next.
		ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
			for i, r := range run.Regions {
				on := r.Session == s.ID
				chip := ui.ButtonBase(c.Key("r:"+r.ID)).Height(24).Padding(0, 9, 0, 7).Gap(6).Radius(theme.RadiusSmall).AlignItems(ui.Center).
					Label("Show the agent of #"+r.ID).Cursor(ui.CursorPointer).Border(1, theme.Border)
				if on {
					chip.Background(theme.Fade(theme.Accent, 0.16)).Border(1, theme.AccentLine)
				} else if chip.Hovered() {
					chip.Background(theme.Hover)
				}
				chip.Children(func() {
					ui.Box(c).Size(7, 7).Radius(theme.RadiusTail).Background(ui.Hex(unslopview.RegionColors[i%len(unslopview.RegionColors)]))
					ui.Text(c, "#"+r.ID).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
					if r.Done {
						ui.Icon(c, icon.Check).FontSize(10).TextColor(theme.Done)
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
