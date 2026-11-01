package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// TextBox is a text input of several lines: a well of a fixed height, whose
// text scrolls inside it.
func TextBox(c *ui.Context, value *string, placeholder string, height float32) ui.Element {
	in := ui.TextArea(c, value).Height(height).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).FontSize(theme.TextUI).
		Placeholder(placeholder).Label(placeholder)
	return Well(in, in.Focused())
}

// SwitchRow is ToggleRow for what has no on or off of its own to hold: it
// shows on and reports a flip.
func SwitchRow(c *ui.Context, title, about string, on bool) bool {
	v := on
	return ToggleRow(c, title, about, &v)
}
