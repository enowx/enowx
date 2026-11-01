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

// mcpTestPanel is the Test button, and how the test went: its tools, or why
// not. Lists have a height of their own, leaving the rest to the server's
// entry under them.
func (v *View) mcpTestPanel(c *ui.Context, o Options, it proto.EnvItem) {
	t := v.test
	if t.forID != it.ID {
		t = mcpTesting{}
	}
	ui.Row(c.Key("mcp-test")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		b := widget.Button(c, widget.Secondary, icon.Play, "Test on "+o.Host, "").Disabled(t.running)
		if b.Clicked() && !t.running {
			v.testMCP(o, it.ID)
		}
		switch {
		case t.running:
			widget.Spinner(c, 14)
			testLine(c, "Starting it on "+o.Host+"…", theme.TextSoft)
		case t.err != "":
			ui.Icon(c, icon.Alert).FontSize(14).TextColor(theme.Error)
			testLine(c, t.err, theme.Error)
		case t.result != nil && t.result.OK:
			widget.Dot(c, theme.Done, false, 8)
			testLine(c, fmt.Sprintf("%s lists %d tools in %.1f s", serverName(*t.result, it.Name), len(t.result.Tools), float64(t.result.Took)/1000), theme.Text)
		case t.result != nil:
			ui.Icon(c, icon.Alert).FontSize(14).TextColor(theme.Error)
			testLine(c, t.result.Error, theme.Error)
		default:
			testLine(c, "Runs it where its agent would and lists its tools.", theme.TextSoft)
		}
	})
	r := t.result
	if r == nil {
		return
	}
	if r.OK && len(r.Tools) > 0 {
		ui.Scroll(c.Key("mcp-tools")).FillWidth().Shrink(0).MaxHeight(88).Children(func() {
			ui.Row(c).FillWidth().Wrap().Gap(theme.S1).Children(func() {
				for i, tool := range r.Tools {
					ui.Row(c.Key(strconv.Itoa(i))).MaxWidth(360).MinWidth(0).Clip().Tooltip(tool.Description).Children(func() {
						widget.Chip(c, tool.Name, ui.Color{})
					})
				}
			})
		})
	}
	if !r.OK && r.Stderr != "" {
		ui.Scroll(c.Key("mcp-stderr")).FillWidth().Shrink(0).MaxHeight(120).Radius(theme.RadiusCard).Clip().Background(widget.CodeGround).Children(func() {
			ui.Column(c).FillWidth().Padding(theme.S3, theme.S4).Children(func() { codeLine(c, r.Stderr, widget.CodeText) })
		})
	}
}

// testLine says how the test goes, in the room beside its button.
func testLine(c *ui.Context, text string, col ui.Color) {
	ui.Row(c.Key("test-line")).Grow(1).Basis(0).MinWidth(0).Children(func() {
		widget.Running(c, text, widget.RunStyle{Color: col})
	})
}

func serverName(r proto.MCPTest, fallback string) string {
	if r.Server != "" {
		return r.Server
	}
	return fallback
}
