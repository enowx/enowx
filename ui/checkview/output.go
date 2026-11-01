package checkview

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// DrawOutput shows a check's output, for a center tab: a bar with its command,
// how it went and running it, the judge's word, then the end of what it printed
// on the code's ground.
func (v *View) DrawOutput(c *ui.Context, o Options, name string) {
	v.follow(o)
	r, ran := v.run(name)
	command := name
	for _, ch := range v.info.Checks {
		if ch.Name == name {
			command = commandOf(ch)
		}
	}
	out := ""
	switch {
	case !ran:
	case r.Output == "" && r.State == proto.CheckRunning:
	default:
		if out = lastLines(r.Output, outputLines); out == "" {
			out = "(no output)"
		}
	}
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		widget.ViewerBar(c, icon.ListChecks, command, func() {
			ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
				widget.Running(c.Key("facts"), facts(r, ran), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if !v.running() {
				label := "Run again"
				if !ran {
					label = "Run"
				}
				if widget.Button(c.Key("run"), widget.Secondary, icon.Play, label, "").Height(28).Clicked() {
					v.do(o, func(cl *client.Client) error { _, err := cl.RunChecks(o.Session.ID, name); return err })
				}
			}
			if out != "" && widget.IconButton(c.Key("copy"), icon.Copy, "Copy the output", false).Clicked() {
				c.WriteClipboard(out)
			}
		})
		if v.err != "" {
			ui.Row(c).FillWidth().Shrink(0).Padding(theme.S3, theme.S4, 0, theme.S4).Children(func() {
				widget.Notice(c, nil, theme.Del, v.err, "", nil)
			})
		}
		if ran && r.State == proto.CheckFailed && (r.Judging || r.Judged != "") {
			ui.Row(c).FillWidth().Shrink(0).Padding(theme.S2, 0, 0, 0).Children(func() { v.judgedLine(c, o, r, theme.S4) })
		}
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Background(widget.CodeGround).Children(func() {
			ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Padding(theme.S3, theme.S4).Gap(0).Children(func() {
					code := func(text string, col ui.Color) {
						ui.Text(c, text).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(col).FillWidth().Selectable()
					}
					dim := ui.Hex("#8A8FA3")
					code("$ "+command, dim)
					switch {
					case !ran:
						code("Not run yet.", dim)
					case r.State == proto.CheckQueued:
						code("Queued.", dim)
					default:
						if out != "" {
							code(out, widget.CodeText)
						}
					}
					if ran && r.State == proto.CheckFailed && strings.TrimSpace(out) != "" {
						code("exit status "+strconv.Itoa(r.ExitCode), ui.Hex("#FF9D97"))
					}
				})
			})
		})
	})
}
