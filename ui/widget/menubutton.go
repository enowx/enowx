package widget

import (
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// MenuButton is a capsule that opens a menu: its label, then a chevron saying
// so.
func MenuButton(c *ui.Context, kind ButtonKind, label string) ui.Element {
	b := ui.ButtonBase(c).Gap(theme.S1).Padding(0, theme.S2, 0, theme.S3).Height(theme.ControlHeight).Radius(theme.RadiusFull).
		Cursor(ui.CursorPointer).Label(label).Shrink(0)
	hover := b.Hovered() && !b.IsDisabled()
	weight := 400
	if kind == Primary {
		bg := theme.Accent
		if hover {
			bg = theme.AccentHigh
		}
		b.Background(QuickTint(b, "bg", bg)).TextColor(theme.OnAccent)
		weight = 500
	} else {
		bg := theme.Fade(theme.TextStrong, 0.1)
		if hover {
			bg = theme.Fade(theme.TextStrong, 0.18)
		}
		b.Background(QuickTint(b, "bg", bg)).TextColor(theme.Text)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(theme.TextUI).FontWeight(weight).SingleLine()
		ui.Icon(c, icon.ChevronDown).FontSize(12)
	})
	return Pressable(b)
}

// SegmentedFit is Segmented filling the width it is given, for a narrow panel:
// the options share it, their words cut rather than leaving the capsule. It
// returns the option clicked, or -1.
func SegmentedFit(c *ui.Context, options []string, current int) int {
	clicked := -1
	ui.Row(c).FillWidth().Shrink(0).Padding(theme.S1).Radius(theme.RadiusFull).Background(theme.Sunken).Children(func() {
		for i, label := range options {
			// Shares by length, not evenly: a long word gets the room a short one
			// leaves, so that narrow cards don't cut labels that would all fit.
			b := ui.ButtonBase(c.Key(i)).Height(28).Grow(float32(utf8.RuneCountInString(label) + 3)).Basis(0).Shrink(1).MinWidth(0).PaddingX(theme.S1).Radius(theme.RadiusFull).Label(label).Center()
			bg, fg, weight := theme.Fade(theme.Selected, 0), theme.TextSoft, 400
			switch {
			case i == current:
				bg, fg, weight = theme.Selected, theme.TextStrong, 500
			case b.Hovered():
				bg, fg = theme.Hover, theme.Text
			}
			if i != current {
				b.Cursor(ui.CursorPointer)
			}
			b.Background(QuickTint(b, "bg", bg)).TextColor(QuickTint(b, "fg", fg))
			b.Children(func() { ui.Text(c, label).FontSize(theme.TextUI).FontWeight(weight).SingleLine().Shrink(1).MinWidth(0) })
			if i != current && b.Clicked() {
				clicked = i
			}
		}
	})
	return clicked
}
