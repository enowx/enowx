package cli

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// The command and its MCP tool change GOA's agent, saying what a change needs; GOA itself
// is neither offered the tool nor let through the command.
func TestGOASwitchCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvSession, "")
	t.Setenv(proto.EnvGOA, "")
	bin := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexec sleep 300\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	c, err := client.New(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var out bytes.Buffer
	run := func(args ...string) (string, error) {
		out.Reset()
		err := runner{c: c, out: &out}.goaCmd(append([]string{"switch"}, args...))
		return out.String(), err
	}
	if _, err := run("--agent", "codex"); err == nil || !strings.Contains(err.Error(), "no GOD of Agents") {
		t.Fatalf("with no GOA: %v", err)
	}
	var g proto.SessionInfo
	if err := c.Call(proto.MethodSessionCreate, proto.SessionCreateParams{Agent: "claude", GOA: true, Cols: 80, Rows: 20}, &g); err != nil {
		t.Fatal(err)
	}
	if text, err := run(); err != nil || !strings.Contains(text, "claude on the official config (enforced)") {
		t.Fatalf("shown: %q %v", text, err)
	}
	if text, err := run("--agent", "codex", "--check"); err != nil || !strings.Contains(text, "a new conversation") {
		t.Fatalf("checked: %q %v", text, err)
	}
	if _, err := run("--agent", "codex"); err == nil || !strings.Contains(err.Error(), "--new-conversation") {
		t.Fatalf("with no yes: %v", err)
	}
	if _, err := run("--agent", "gemini", "--new-conversation"); err == nil || !strings.Contains(err.Error(), "not tested yet") {
		t.Fatalf("not proven: %v", err)
	}

	t.Run("GOA itself", func(t *testing.T) {
		t.Setenv(proto.EnvGOA, "1")
		if _, err := run("--agent", "codex", "--new-conversation"); err == nil || !strings.Contains(err.Error(), "tell the user") {
			t.Fatalf("got %v", err)
		}
		if text, err := run(); err != nil || !strings.Contains(text, "claude") {
			t.Fatalf("reading: %q %v", text, err)
		}
	})
	if slices.Contains(goaOfferedTools, "goa_switch") {
		t.Fatal("GOA is offered goa_switch")
	}

	i := slices.IndexFunc(goaTools, func(tool mcpTool) bool { return tool.name == "goa_switch" })
	if i < 0 {
		t.Fatal("no goa_switch tool")
	}
	runner{c: c, out: &out}.callTool(goaTools[i], runner.goaCmd, map[string]any{"agent": "codex", "new_conversation": true})
	var board proto.GOABoard
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.Call(proto.MethodGOABoard, nil, &board); err != nil {
			t.Fatal(err)
		}
		if board.Agent == "codex" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if board.Agent != "codex" || board.Session != g.ID {
		t.Fatalf("through the tool: %+v", board)
	}
}
