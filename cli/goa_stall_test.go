package cli

import (
	"bytes"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// The limits shown are the host's; only those named change, and 0 puts one back.
func TestGOAStallCommand(t *testing.T) {
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
		err := runner{c: c, out: &out}.goaCmd(append([]string{"stall"}, args...))
		return out.String(), err
	}
	if text, err := run(); err != nil || !strings.Contains(text, "blank 1m") || !strings.Contains(text, "stuck 10m") {
		t.Fatalf("the defaults: %q %v", text, err)
	}
	if text, err := run("--idle", "90s", "--stuck", "1h"); err != nil || !strings.Contains(text, "idle  1m30s") || !strings.Contains(text, "stuck 1h") {
		t.Fatalf("set: %q %v", text, err)
	}
	if _, err := run("--blank", "soon"); err == nil {
		t.Fatal("soon taken")
	}
	if _, err := run("--stuck", "0"); err != nil {
		t.Fatal(err)
	}
	var l proto.GOAStall
	if err := c.Call(proto.MethodGOAStall, nil, &l); err != nil || l != (proto.GOAStall{Blank: goa.StallBlank, Idle: 90, Stuck: goa.StallStuck}) {
		t.Fatalf("kept: %+v %v", l, err)
	}
	// The MCP tool is the same command.
	i := slices.IndexFunc(goaTools, func(tool mcpTool) bool { return tool.name == "goa_stall" })
	if i < 0 {
		t.Fatal("no goa_stall tool")
	}
	out.Reset()
	runner{c: c, out: &out}.callTool(goaTools[i], runner.goaCmd, map[string]any{"blank": "45s"})
	if err := c.Call(proto.MethodGOAStall, nil, &l); err != nil || l.Blank != 45 || l.Idle != 90 {
		t.Fatalf("through the tool: %+v %v", l, err)
	}
}
