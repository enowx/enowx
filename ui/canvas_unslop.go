package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/unslopview"
)

// Unslop on the Canvas page: one click reads the design shown for what
// makes it look AI-made, copies it, shows the copy, and has the canvas's
// agent redesign it pass by pass, with nothing more to ask.

func (a *App) unslopCanvas(frame string) { a.unslopCanvasOnly(frame, nil) }

// unslopCanvasOnly unslops design frame with only these passes (nil for
// all of them).
func (a *App) unslopCanvasOnly(frame string, only []string) {
	cv, cl := a.canvases.Canvas(), a.client
	if cv == nil || cl == nil || a.unslopping {
		return
	}
	a.unslopping, a.unslopErr = true, ""
	id := cv.ID
	go func() {
		u, err := cl.CanvasUnslop(proto.CanvasUnslopParams{ID: id, Frame: frame, Start: true, Only: only})
		a.onMain(func() {
			a.unslopping = false
			if err != nil {
				a.unslopErr = strings.TrimPrefix(client.Explain(err), "canvas: ")
				return
			}
			o := a.canvasOptions()
			a.canvases.Goto(o, id, u.Frame)
			a.canvases.OpenAssistant()
			if cur := a.canvases.Canvas(); cur != nil && cur.ID == id {
				a.canvasTell(*cur, u.Ask)
			}
			a.notify(fmt.Sprintf("Unslop: %d/100, %d findings; the agent redesigns the copy", u.Score, len(u.Findings)))
		})
	}()
}

// unslopEvent keeps a redesign's run as the daemon tells it.
func (a *App) unslopEvent(data json.RawMessage) func() {
	var run proto.CanvasUnslopRun
	if json.Unmarshal(data, &run) != nil {
		return nil
	}
	return func() {
		if a.unslopRuns == nil {
			a.unslopRuns = map[string]proto.CanvasUnslopRun{}
		}
		a.unslopRuns[run.ID] = run
	}
}

// unslopRunOf is the run of the canvas shown, nil when none.
func (a *App) unslopRunOf(_ *hostCanvases) *proto.CanvasUnslopRun {
	cv := a.canvases.Canvas()
	if cv == nil {
		return nil
	}
	if run, ok := a.unslopRuns[cv.ID]; ok {
		return &run
	}
	return nil
}

// agentState is what session's agent is at, for the Unslop panel.
func (a *App) agentState(id string) string {
	s, ok := a.sessionByID(id)
	switch {
	case !ok || s.Exited:
		return unslopview.AgentStopped
	case s.State == proto.StateWaiting:
		return unslopview.AgentWaiting
	case s.State == proto.StateWorking || s.Active || len(s.Background) > 0:
		return unslopview.AgentWorking
	}
	return unslopview.AgentIdle
}
