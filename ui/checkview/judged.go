package checkview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// judgedLine is the judge's word on failed run r, when the host asks it
// (Settings → Decisions → Flaky or real test failures): flaky, with Run
// again, or a real failure, each with why.
func (v *View) judgedLine(c *ui.Context, o Options, r proto.CheckRun, left float32) {
	if !r.Judging && r.Judged == "" {
		return
	}
	ui.Column(c.Key("judged")).FillWidth().Padding(0, theme.S3, theme.S2, left).Gap(theme.S1).Children(func() {
		if r.Judging {
			ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
				widget.Spinner(c, 12)
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, "The judge reads the failure…", widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
			})
			return
		}
		ui.Row(c).FillWidth().MinHeight(24).Gap(theme.S2).Children(func() {
			if r.Judged == proto.VerdictFlaky {
				widget.Tag(c, "flaky", theme.Warning)
				ui.Spacer(c)
				if !v.running() && widget.Button(c, widget.Secondary, icon.Play, "Run again", "").Height(24).Clicked() {
					v.do(o, func(cl *client.Client) error { _, err := cl.RunChecks(o.Session.ID, r.Name); return err })
				}
				return
			}
			widget.Tag(c, "real failure", theme.Error)
		})
		ui.Text(c, widget.LongWords(r.JudgedWhy)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).MaxLines(3)
	})
}
