package diffview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// DrawFile shows the changes of the file at path, for a center tab: a bar with
// its path, how it changed and how to read it, then its lines.
func (v *View) DrawFile(c *ui.Context, o Options, path string) {
	v.follow(o)
	v.showFile(path, v.splitWanted)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		v.fileBar(c, o, path)
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Background(diffGround).Children(func() {
			switch {
			case !v.loaded:
				centered(c, "Reading the changes…")
			case v.fileIndex(path) < 0:
				centered(c, "This file has no changes any more.")
			default:
				v.lines(c)
			}
		})
		v.commentBar(c, o)
	})
}

// fileBar heads a file's changes.
func (v *View) fileBar(c *ui.Context, o Options, path string) {
	ui.Row(c).Height(44).Shrink(0).FillWidth().Padding(0, theme.S2, 0, theme.S4).Gap(theme.S2).Clip().
		BorderWidth(0, 0, 0.5, 0).BorderColor(theme.Border).Children(func() {
		ui.Icon(c, icon.Git).FontSize(14).TextColor(theme.TextSoft)
		name := path
		fi := v.fileIndex(path)
		if fi >= 0 && v.diff.Files[fi].OldPath != "" {
			name = v.diff.Files[fi].OldPath + " → " + path
		}
		ui.Row(c).MinWidth(64).Shrink(1).Children(func() {
			widget.Running(c, name, widget.RunStyle{Size: theme.TextLabel, Mono: true, Always: true})
		})
		if fi >= 0 {
			letter, col := statusLetter(v.diff.Files[fi].Status)
			ui.Text(c, letter).FontSize(theme.TextLabel).FontWeight(500).TextColor(col).SingleLine().Shrink(0)
		}
		ui.Spacer(c)
		if fi >= 0 {
			counts(c, v.diff.Files[fi])
		}
		current := 0
		if v.splitWanted {
			current = 1
		}
		if i := widget.Segmented(c.Key("how"), []string{"Unified", "Side by side"}, current, nil); i >= 0 {
			v.splitWanted = i == 1
		}
		if o.OpenFile != nil && widget.Button(c.Key("open-file"), widget.Secondary, icon.File, "Open the file", "").Height(28).Clicked() {
			o.OpenFile(path)
		}
		if widget.IconButton(c, icon.Copy, "Copy the path", false).Clicked() {
			c.WriteClipboard(path)
			if o.Notify != nil {
				o.Notify("Copied " + path)
			}
		}
	})
}

// lines is the virtual list of the rows.
func (v *View) lines(c *ui.Context) {
	rows, list := v.rows, &v.fileList
	list.Key = func(i int) any { return rows[i] }
	widget.List(c.Key(v.file), list, len(rows), func(i int) {
		r := rows[i]
		switch r.kind {
		case rowHunk:
			ui.Row(c).FillWidth().Height(rowHeight).Padding(0, theme.S4, 0, 88).Background(theme.Fade(diffHunk, 0.08)).Children(func() {
				ui.Text(c, v.diff.Files[r.file].Hunks[r.hunk].Header).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(diffHunk).SingleLine().Shrink(1).MinWidth(0)
			})
		case rowLine:
			v.diffLine(c, v.diff.Files[r.file].Path, v.diff.Files[r.file].Hunks[r.hunk].Lines[r.line])
		case rowPair:
			lines := v.diff.Files[r.file].Hunks[r.hunk].Lines
			ui.Row(c).FillWidth().Height(rowHeight).Children(func() {
				pairLine(c.Key("l"), lines, r.line, true)
				ui.Box(c).Width(0.5).FillHeight().Background(theme.Fade(theme.TextStrong, 0.12))
				pairLine(c.Key("r"), lines, r.other, false)
			})
		case rowComment:
			v.commentRow(c, r.comment)
		case rowDraft:
			v.draftRow(c)
		case rowBinary:
			note := "Binary file"
			if !v.diff.Files[r.file].Binary {
				note = "No lines to show"
			}
			ui.Row(c).FillWidth().Height(32).PaddingX(theme.S4).Children(func() {
				ui.Text(c, note).FontSize(theme.TextUI).TextColor(diffText).SingleLine()
			})
		case rowCut:
			ui.Row(c).FillWidth().Height(rowHeight).PaddingX(theme.S4).Children(func() {
				ui.Text(c, "…the rest of this file is too long to show here").FontSize(theme.TextLabel).TextColor(diffText).SingleLine()
			})
		}
	}).Grow(1).FillWidth().MinHeight(0)
}
