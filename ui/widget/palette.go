package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

const (
	paletteWidth  = 600
	paletteTop    = 96
	paletteHeight = 560
)

// Palette is a search that lists what it finds, hung from the window's top so
// it stays put as the list shortens: the field, a line, then the list, which
// scrolls when the window is short. onKey sees the keys before the field does;
// it returns whether Enter was pressed.
func Palette(c *ui.Context, open *bool, query *string, placeholder string, onKey func(ui.TextKey) bool, list func()) (submitted bool) {
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		dialogMotion(backdrop, panel)
		w, h := c.Size()
		width := min(paletteWidth, max(280, w-32))
		Float(panel.Absolute().Left((w - width) / 2).Top(paletteTop).Width(width).
			MaxHeight(max(0, min(paletteHeight, h-paletteTop-16))).Radius(theme.RadiusPanel).Clip().Background(theme.Dialog))
		ui.Row(c).FillWidth().Shrink(0).Height(52).PaddingX(theme.S4).Gap(theme.S2).Children(func() {
			ui.Icon(c, icon.Search).FontSize(16).TextColor(theme.TextSoft)
			in := ui.TextInputBase(c, query).Grow(1).MinWidth(0).FontSize(theme.TextRead).Placeholder(placeholder).Label(placeholder).AutoFocus()
			in.OnTextKey(onKey)
			submitted = in.Submitted()
		})
		ui.Box(c).FillWidth().Shrink(0).Height(theme.Rule).Background(theme.Border)
		ui.Scroll(c).FillWidth().Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(theme.S2, ScrollGutter, theme.S2, theme.S2).Gap(2).Children(list)
		})
	})
	return submitted
}

// PaletteRow is a result: its icon, its name and, far right, a word about it.
// Both run when they don't fit, sharing the row with what is picked.
func PaletteRow(c *ui.Context, row ui.Element, picked bool, svg *ui.SVG, label, detail string, mono bool) ui.Element {
	row.FillWidth().Shrink(0).MinHeight(theme.ControlHeight).Padding(theme.S1, theme.S3).Gap(theme.S2).Radius(theme.RadiusFull).Justify(ui.Start).Cursor(ui.CursorPointer)
	bg := theme.Fade(theme.Selected, 0)
	if picked || row.Hovered() {
		bg = theme.Selected
	}
	row.Background(QuickTint(row, "bg", bg)).Children(func() {
		ui.Icon(c, svg).FontSize(14).TextColor(theme.SoftOn(picked || row.Hovered()))
		ui.Row(c).MinWidth(0).Shrink(1).Children(func() { Running(c, label, RunStyle{Always: picked}) })
		ui.Spacer(c)
		if detail != "" {
			ui.Row(c).MinWidth(0).Shrink(100).Children(func() {
				Running(c.Key("detail"), detail, RunStyle{Size: theme.TextLabel, Color: theme.SoftOn(picked || row.Hovered()), Mono: mono})
			})
		}
	})
	return row
}
