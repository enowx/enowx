package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// Choice is a drop-down of options, named label to assistive tools, width wide
// (0 fills its parent). It returns the index picked this frame, or -1.
func Choice(c *ui.Context, label string, options []string, current int, width float32) int {
	if current < 0 || current >= len(options) {
		current = 0
	}
	cur := options[current]
	sel := ui.SelectBase(c.Key("choice:"+label), &cur)
	t := Trigger(sel.Trigger, sel.Open()).Label(label)
	if width > 0 {
		t.Width(width).Shrink(0)
	}
	t.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { Running(c, options[current], RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	sel.Popup(func(panel ui.Element) {
		Popup(panel.Margin(theme.S1, 0, 0, 0).MinWidth(max(width, 160)))
		ui.Scroll(c).FillWidth().MaxHeight(320).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, o := range options {
					item := sel.Item(o)
					PopupItem(item, item.Highlighted()).Children(func() { ui.Text(c, o).FontSize(theme.TextUI).SingleLine() })
				}
			})
		})
	})
	if cur != options[current] {
		for i, o := range options {
			if o == cur {
				return i
			}
		}
	}
	return -1
}

// SegmentedFill is Segmented taking the width it is given, its options sharing
// it evenly: their words run when too long, so it never leaves its frame.
func SegmentedFill(c *ui.Context, options []string, current int) int {
	clicked := -1
	ui.Row(c).FillWidth().MinWidth(0).Padding(theme.S1).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
		for i, label := range options {
			b := ui.ButtonBase(c.Key(label)).Grow(1).Basis(0).MinWidth(0).Height(28).PaddingX(theme.S1).Radius(theme.RadiusSmall).Label(label)
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
			b.Background(QuickTint(b, "bg", bg)).Children(func() {
				Running(c, label, RunStyle{Color: fg, Weight: weight})
			})
			if i != current && b.Clicked() {
				clicked = i
			}
		}
	})
	return clicked
}
