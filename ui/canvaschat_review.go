package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
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
	ui.Column(c.Key("review")).FillWidth().Shrink(0).Padding(12).Gap(8).Background(theme.AccentSoft).BorderWidth(0, 0, 1, 0).BorderColor(theme.AccentLine).Children(func() {
		ui.Text(c, fmt.Sprintf("Changes from your message: v%d → v%d", m.Version, cv.Version)).FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong)
		ui.Text(c, "“"+firstLine(m.Text)+"”").FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			if widget.Button(c, widget.Primary, icon.Check, "Keep", "").Height(28).PaddingX(10).Clicked() {
				a.markCanvasMessage(cv.ID, m.N, proto.CanvasChatKept)
			}
			if widget.Button(c, widget.Secondary, icon.Undo, "Undo", "").Height(28).PaddingX(10).Tooltip(fmt.Sprintf("Back to version %d", m.Version)).Clicked() {
				a.undoCanvasMessage(cv.ID, m)
			}
			if widget.Button(c, widget.Secondary, icon.Layers, "Compare", "").Height(28).PaddingX(10).Clicked() {
				a.canvases.ShowVersions(a.canvasOptions(), m.Version)
			}
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
		ui.Column(c).FillWidth().Padding(12).Gap(8).Children(func() {
			if len(hc.log.Messages) == 0 {
				ui.Text(c, "No messages yet.").FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
			for i := len(hc.log.Messages) - 1; i >= 0; i-- {
				m := hc.log.Messages[i]
				ui.Column(c.Key(m.N)).FillWidth().Padding(10).Gap(6).Radius(theme.RadiusSmall).Background(theme.Raised).Border(1, theme.Border).Children(func() {
					ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, fmt.Sprintf("#%d · v%d", m.N, m.Version)).FontSize(theme.TextLabel).FontWeight(600).TextColor(theme.Accent)
						if m.State != "" {
							widget.Tag(c, m.State, theme.Muted)
						}
						ui.Spacer(c)
						ui.Text(c, m.Time.Local().Format("Jan 2 15:04")).FontSize(theme.TextLabel).TextColor(theme.Muted)
					})
					ui.Text(c, m.Text).FontSize(theme.TextLabel).TextColor(theme.Text)
					ui.Row(c).FillWidth().Gap(4).Children(func() {
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
