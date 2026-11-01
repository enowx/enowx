//go:build !windows

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// The Canvas page's agent: asked about the element selected, it starts on
// the canvas told where the user is; the three login designs it makes
// show in the strip as it makes them; the user undoes them from the
// review card, and the conversation keeps the message with its version.
func TestCanvasAgentDesignsAndTheUserUndoes(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h := newCanvasHarness(t)
	h.app.agents = []proto.AgentInfo{{ID: "claude", Name: "Claude Code", Command: "claude", Installed: true}}
	cv, _ := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Login", Kind: proto.CanvasWireframe, Frames: []proto.CanvasFrame{{Name: "Login A"}}})
	hero := proto.CanvasElement{ID: "hero", Type: proto.ShapeRect, X: 200, Y: 200, W: 400, H: 160, Text: "Welcome back"}
	if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpElementSet, Element: &hero}); err != nil {
		t.Fatal(err)
	}
	h.settle("the canvas open", func() bool { _, ok := h.tt.Find("hero"); return ok })
	if err := h.tt.Click("hero"); err != nil {
		t.Fatal(err)
	}
	h.tt.Advance(time.Second)
	h.settle("the inspector", func() bool { return h.tt.HasText("Ask agent") })
	if err := h.tt.Click("Ask agent about hero"); err != nil {
		t.Fatal(err)
	}
	h.settle("the agent's panel", func() bool { return h.tt.HasText("Design with Claude Code") })
	h.tt.Advance(time.Second)
	h.tt.Frame()
	shot(t, h.tt, "canvas-agent-panel.png")
	if err := h.tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("make 3 login designs")
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	var agent proto.SessionInfo
	h.settle("the agent started (status "+h.app.status+")", func() bool {
		list, _ := h.cl.Sessions()
		for _, s := range list {
			if s.CanvasFor == cv.ID {
				agent = s
				return true
			}
		}
		return false
	})
	if !strings.HasPrefix(agent.Prompt, "[canvas "+cv.ID+" “Login” · design 1 “Login A”") || !strings.Contains(agent.Prompt, "selected #hero]\n@#hero make 3 login designs") {
		t.Fatalf("prompt %q", agent.Prompt)
	}
	h.settle("the message kept", func() bool {
		log, _ := h.cl.CanvasChatLog(cv.ID)
		return len(log.Messages) == 1 && log.Messages[0].Version == 2 && log.Messages[0].Text == "@#hero make 3 login designs"
	})

	// The agent, through the canvas tools in its session.
	for _, name := range []string{"Login B", "Login C", "Login D"} {
		if _, err := h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpFrameAdd, FrameInfo: &proto.CanvasFrame{Name: name}, Session: agent.ID}); err != nil {
			t.Fatal(err)
		}
		h.settle(name+" in the strip", func() bool { return h.tt.HasText(name) })
	}
	h.settle("the review card", func() bool { return h.tt.HasText("Changes from your message: v2 → v5") })
	shot(t, h.tt, "canvas-agent-review.png")
	if err := h.tt.Click("Undo"); err != nil {
		t.Fatal(err)
	}
	h.settle("undone", func() bool {
		c, _ := h.cl.Canvas(cv.ID)
		log, _ := h.cl.CanvasChatLog(cv.ID)
		return len(c.Frames) == 1 && log.Messages[0].State == proto.CanvasChatUndone && !h.tt.HasText("Login B")
	})
	if err := h.tt.Click("Messages and their versions (1)"); err != nil {
		t.Fatal(err)
	}
	h.settle("the history", func() bool { return h.tt.HasText("#1 · v2") && h.tt.HasText("Back to v2") })
}
