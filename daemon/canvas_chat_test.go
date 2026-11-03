package daemon

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// A Canvas page agent starts in a folder of its own with the canvas skill
// and enowX's canvas tools, reading the canvas's folder and its projects,
// unable to write files or run commands; what it is asked is kept with the
// canvas's version then, and marked kept or undone.
func TestCanvasAgentStartsOnTheCanvas(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	project := t.TempDir()
	cv, err := c.CreateCanvas(proto.CanvasCreateParams{Title: "Login", Projects: []string{project}})
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", CanvasFor: cv.ID, Prompt: "make 3 login designs", Project: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if info.CanvasFor != cv.ID || info.Project != "" || !strings.HasSuffix(info.Dir, filepath.Join("canvas-chat", cv.ID)) {
		t.Fatalf("session %+v", info)
	}
	cmd := strings.Join(info.Command, "\x00") + sessionGuide(t, srv, info.ID)
	for _, want := range []string{agent.CanvasChatGuide, "Many designs in one canvas", "--add-dir\x00", project, "mcp__enowx", "--disallowedTools", "--strict-mcp-config"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command lacks %q", want)
		}
	}
	if strings.Contains(cmd, agent.Guide) {
		t.Error("the canvas agent got the sessions' guide")
	}
	l, err := launchFor(proto.SessionCreateParams{Agent: "claude", CanvasFor: cv.ID}, srv.chatOf("", cv.ID, ""))
	if err != nil || !slices.Contains(l.env, proto.EnvCanvasChat+"=1") {
		t.Fatalf("env %q, %v", l.env, err)
	}

	if _, err := c.CanvasChat(proto.CanvasChatParams{ID: cv.ID, Agent: "codex", Mode: proto.CanvasChatDirect}); err != nil {
		t.Fatal(err)
	}
	log, err := c.CanvasChat(proto.CanvasChatParams{ID: cv.ID, Text: "make 3 login designs", Session: info.ID})
	if err != nil || len(log.Messages) != 1 || log.Messages[0].Version != 1 || log.Agent != "codex" || log.Mode != proto.CanvasChatDirect {
		t.Fatalf("log %+v %v", log, err)
	}
	log, err = c.MarkCanvasChat(cv.ID, 1, proto.CanvasChatUndone)
	if err != nil || log.Messages[0].State != proto.CanvasChatUndone {
		t.Fatalf("mark %+v %v", log, err)
	}
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", CanvasFor: "000000000000"}); err == nil {
		t.Fatal("an agent for a canvas that isn't")
	}
}
