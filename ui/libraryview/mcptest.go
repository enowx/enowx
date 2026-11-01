package libraryview

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// An MCP server's detail tests it where its agent runs it (env.mcp.test):
// it starts, answers, and lists its tools, or the test says why not.

// mcpTesting is a test of an MCP server, under way or done.
type mcpTesting struct {
	forID   string
	running bool
	result  *proto.MCPTest
	err     string
}

// testMCP tests MCP server id.
func (v *View) testMCP(o Options, id string) {
	cl := o.Client
	if cl == nil {
		return
	}
	v.test = mcpTesting{forID: id, running: true}
	go func() {
		res, err := cl.EnvMCPTest("", id)
		o.Update(func() {
			if v.test.forID != id {
				return
			}
			v.test.running = false
			if err != nil {
				v.test.err = client.Explain(err)
				return
			}
			v.test.result = &res
		})
	}()
}

// mcpTestPanel is the Test button, and how the test went.
func (v *View) mcpTestPanel(c *ui.Context, o Options, it proto.EnvItem) {
	t := v.test
	if t.forID != it.ID {
		t = mcpTesting{}
	}
	ui.Row(c).FillWidth().Gap(10).Children(func() {
		b := widget.Button(c, widget.Secondary, icon.Play, "Test", "").Tooltip("Run it on " + o.Host + " and list its tools").Disabled(t.running)
		if b.Clicked() && !t.running {
			v.testMCP(o, it.ID)
		}
		switch {
		case t.running:
			widget.Spinner(c, 14)
			ui.Text(c, "Starting it on "+o.Host+"…").FontSize(theme.TextLabel).TextColor(theme.Muted)
		case t.err != "":
			ui.Text(c, t.err).FontSize(theme.TextLabel).TextColor(theme.Error).Shrink(1)
		case t.result != nil && t.result.OK:
			ui.Icon(c, icon.Check).FontSize(13).TextColor(theme.Done)
			ui.Text(c, fmt.Sprintf("%s answers · %d tools · %.1fs", serverName(*t.result, it.Name), len(t.result.Tools), float64(t.result.Took)/1000)).
				FontSize(12).TextColor(theme.Done).Shrink(1)
		case t.result != nil:
			ui.Icon(c, icon.Alert).FontSize(13).TextColor(theme.Error)
			ui.Text(c, t.result.Error).FontSize(theme.TextLabel).TextColor(theme.Error).Shrink(1)
		}
	})
	r := t.result
	if r == nil {
		return
	}
	if r.OK {
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			for i, tool := range r.Tools {
				ui.Row(c.Key(strconv.Itoa(i))).FillWidth().Gap(10).Children(func() {
					ui.Text(c, tool.Name).Font(theme.MonoFont).FontSize(theme.TextLabel).Width(220).Shrink(0)
					ui.Text(c, tool.Description).FontSize(theme.TextLabel).TextColor(theme.Muted).Grow(1).MinWidth(0)
				})
			}
		})
	}
	if !r.OK && r.Stderr != "" {
		ui.Text(c, r.Stderr).Font(theme.MonoFont).FontSize(theme.TextLabel).Padding(8, 10).Radius(theme.RadiusSmall).Background(theme.Sunken).FillWidth()
	}
}

func serverName(r proto.MCPTest, fallback string) string {
	if r.Server != "" {
		return r.Server
	}
	return fallback
}
