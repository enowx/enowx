package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A message's changes show live; in review mode, once the agent is done
// with it, the panel asks to keep them or undo them (back to the canvas's
// version when it was sent). Every message keeps that version, which the
// history goes back to.

// pendingMessage is the last message whose changes wait for the user: in
// review mode, the agent done, the canvas changed since it was sent.
func pendingMessage(cv proto.Canvas, log proto.CanvasChatLog, s proto.SessionInfo, ok bool) (proto.CanvasChatEntry, bool) {
	if log.Mode == proto.CanvasChatDirect || len(log.Messages) == 0 {
		return proto.CanvasChatEntry{}, false
	}
	m := log.Messages[len(log.Messages)-1]
	if m.State != "" || cv.Version <= m.Version || ok && isWorking(s) {
		return proto.CanvasChatEntry{}, false
	}
	return m, true
}

// canvasReview is the card keeping or undoing the last message's changes.
func (a *App) canvasReview(c *ui.Context, cv proto.Canvas, hc *hostCanvasChat, s proto.SessionInfo, ok bool) {
	m, pending := pendingMessage(cv, hc.log, s, ok)
	if !pending {
		return
	}
	ui.Column(c.Key("review")).FillWidth().Shrink(0).Padding(0, theme.S3, theme.S2, theme.S3).Children(func() {
		ui.Column(c.Key("plate")).FillWidth().Padding(theme.S3, theme.S4).Gap(theme.S1).Radius(theme.RadiusCard).Background(theme.Sunken).Children(func() {
			widget.Running(c.Key("what"), fmt.Sprintf("Changes from your message: v%d → v%d", m.Version, cv.Version), widget.RunStyle{Weight: 500})
			widget.Running(c.Key("said"), "“"+firstLine(m.Text)+"”", widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			ui.Row(c.Key("actions")).FillWidth().Gap(theme.S1).Padding(theme.S1, 0, 0, 0).Wrap().Children(func() {
				if widget.Button(c.Key("keep"), widget.Primary, nil, "Keep", "").Clicked() {
					a.markCanvasMessage(cv.ID, m.N, proto.CanvasChatKept)
				}
				if widget.GhostButton(c.Key("undo"), "Undo").Tooltip(fmt.Sprintf("Back to version %d", m.Version)).Clicked() {
					a.undoCanvasMessage(cv.ID, m)
				}
				if widget.GhostButton(c.Key("compare"), "Compare").Clicked() {
					a.canvases.ShowVersions(a.canvasOptions(), m.Version)
				}
			})
		})
	})
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

func (a *App) markCanvasMessage(id string, n int, state string) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		if _, err := cl.MarkCanvasChat(id, n, state); err != nil {
			a.onMain(func() { a.status = err.Error() })
		}
	}()
}

// undoCanvasMessage puts the canvas back as it was when m was sent, as a
// new version, and says m's changes were undone.
func (a *App) undoCanvasMessage(id string, m proto.CanvasChatEntry) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		_, err := cl.RestoreCanvas(id, m.Version)
		if err == nil {
			_, err = cl.MarkCanvasChat(id, m.N, proto.CanvasChatUndone)
		}
		if err != nil {
			a.onMain(func() { a.status = err.Error() })
		}
	}()
}

// canvasChatHistory lists the messages sent, each with the canvas's
// version then: to compare with, or go back to.
func (a *App) canvasChatHistory(c *ui.Context, cv proto.Canvas, hc *hostCanvasChat) {
	ui.Scroll(c.Key("chat-history")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(0, theme.S3, theme.S3, theme.S3).Gap(theme.S2).Children(func() {
			if len(hc.log.Messages) == 0 {
				ui.Text(c, "No messages yet.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			}
			for i := len(hc.log.Messages) - 1; i >= 0; i-- {
				m := hc.log.Messages[i]
				widget.Raise(ui.Column(c.Key(m.N)).FillWidth().Shrink(0).Padding(theme.S2, theme.S3).Gap(theme.S1).Radius(theme.RadiusCard).Background(theme.Raised)).Children(func() {
					ui.Row(c.Key("head")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
						ui.Text(c, fmt.Sprintf("#%d · v%d", m.N, m.Version)).FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.Now).SingleLine().Shrink(0)
						if m.State != "" {
							widget.Tag(c.Key("state"), m.State, theme.Muted)
						}
						ui.Spacer(c)
						ui.Text(c.Key("when"), m.Time.Local().Format("Jan 2 15:04")).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
					})
					ui.Text(c.Key("text"), m.Text).FontSize(theme.TextUI).TextColor(theme.Text)
					ui.Row(c.Key("actions")).FillWidth().Gap(theme.S1).Wrap().Children(func() {
						if widget.GhostButton(c.Key("cmp"), fmt.Sprintf("Compare v%d", m.Version)).Clicked() {
							a.canvases.ShowVersions(a.canvasOptions(), m.Version)
						}
						if widget.GhostButton(c.Key("back"), fmt.Sprintf("Back to v%d", m.Version)).Clicked() {
							a.undoCanvasMessage(cv.ID, m)
						}
					})
				})
			}
		})
	})
}
