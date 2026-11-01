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
