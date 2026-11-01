package diffview

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
)

// conflictBanner says which files another running session changes too,
// before their branches meet; clicking one shows that session.
func (v *View) conflictBanner(c *ui.Context, o Options) {
	if len(o.Conflicts) == 0 {
		return
	}
	lines := 0
	for _, n := range o.Conflicts {
		if n.Lines {
			lines++
		}
	}
	col := theme.Muted
	if lines > 0 {
		col = theme.Warning
	}
	ui.Column(c).Shrink(0).FillWidth().Padding(8, 12, 8, 14).Gap(4).Background(theme.Fade(col, 0.08)).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Icon(c, icon.Alert).FontSize(12).TextColor(col)
			title := fmt.Sprintf("%d %s also changed in another session", len(o.Conflicts), textfmt.Plural(len(o.Conflicts), "file", "files"))
			ui.Text(c, title).FontSize(theme.TextLabel).TextColor(theme.Text).SingleLine().Shrink(1)
		})
		for i, n := range o.Conflicts {
			b := ui.ButtonBase(c.Key(i)).FillWidth().Height(22).Gap(8).Padding(0, 0, 0, 20).Justify(ui.Start).
				Cursor(ui.CursorPointer).Label(n.File + " in " + n.With)
			b.Children(func() {
				ui.Text(c, n.File).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(1)
				ui.Text(c, "· "+n.With).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
				ui.Spacer(c)
				if n.Lines {
					ui.Text(c, "same lines").FontSize(theme.TextLabel).TextColor(theme.Warning).SingleLine().Shrink(0)
				}
			})
			if b.Clicked() && o.ShowSession != nil {
				o.ShowSession(n.Session)
			}
		}
	})
}
