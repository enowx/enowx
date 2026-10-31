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
func (v *View) judgedLine(c *ui.Context, o Options, r proto.CheckRun) {
	if !r.Judging && r.Judged == "" {
		return
	}
	ui.Row(c.Key("judged")).FillWidth().Padding(0, 10, 8, 36).Gap(8).AlignItems(ui.Center).Children(func() {
		switch {
		case r.Judging:
			widget.Spinner(c, 10)
			ui.Text(c, "The judge reads the failure…").FontSize(11.5).TextColor(theme.Muted)
		case r.Judged == proto.VerdictFlaky:
			widget.Tag(c, "flaky", theme.Warning)
			ui.Text(c, r.JudgedWhy).FontSize(11.5).TextColor(theme.TextSoft).Shrink(1).MaxLines(2)
			ui.Spacer(c)
			if !v.running() && widget.Button(c, widget.Secondary, icon.Play, "Run again", "").Height(26).Clicked() {
				v.do(o, func(cl *client.Client) error { _, err := cl.RunChecks(o.Session.ID, r.Name); return err })
			}
		default:
			widget.Tag(c, "real failure", theme.Error)
			ui.Text(c, r.JudgedWhy).FontSize(11.5).TextColor(theme.TextSoft).Shrink(1).MaxLines(2)
		}
	})
}
