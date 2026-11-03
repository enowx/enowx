package diffview

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
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
	// As many files as it holds: past a few, its frame stays and they scroll.
	ui.Column(c.Key("conflicts")).Shrink(0).FillWidth().MaxHeight(136).Padding(theme.S2, theme.S1, theme.S2, theme.S3).Gap(theme.S1).Radius(theme.RadiusCard).Clip().
		Background(theme.Fade(col, 0.14)).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).PaddingX(theme.S1).Children(func() {
			ui.Icon(c, icon.Alert).FontSize(14).TextColor(col)
			title := fmt.Sprintf("%d %s also changed in another session", len(o.Conflicts), textfmt.Plural(len(o.Conflicts), "file", "files"))
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, title, widget.RunStyle{Size: theme.TextLabel}) })
		})
		widget.Scroll(c).FillWidth().Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for i, n := range o.Conflicts {
					v.conflictRow(c.Key(i), o, n)
				}
			})
		})
	})
}

// conflictRow is a file another session changes; clicking it shows that session.
func (v *View) conflictRow(c *ui.Context, o Options, n ConflictNote) {
	b := ui.ButtonBase(c).FillWidth().Height(24).Gap(theme.S2).PaddingX(theme.S1).Radius(theme.RadiusSmall).Justify(ui.Start).
		Cursor(ui.CursorPointer).Label(n.File + " in " + n.With)
	if b.Hovered() {
		b.Background(theme.Hover)
	}
	b.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(theme.S1).Children(func() {
			ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
				widget.Running(c.Key("file"), n.File, widget.RunStyle{Size: theme.TextLabel, Mono: true})
			})
			ui.Row(c).MinWidth(0).Shrink(100).Children(func() {
				widget.Running(c.Key("with"), "· "+n.With, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		})
		if n.Lines {
			ui.Text(c, "same lines").FontSize(theme.TextLabel).TextColor(theme.Warning).SingleLine().Shrink(0)
		}
	})
	if b.Clicked() && o.ShowSession != nil {
		o.ShowSession(n.Session)
	}
}
