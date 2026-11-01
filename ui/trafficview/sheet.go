package trafficview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// sheet is a form of the tool as a dialog over the window.
func sheet(c *ui.Context, open *bool, width float32, title, description string, body, buttons func()) {
	widget.FormSheet(c, open, width, title, description, body, buttons)
}

// codeArea is a multi-line field of code: headers, a body.
func codeArea(c *ui.Context, label string, value *string, height float32) {
	widget.Field(c, label, func() {
		ui.TextAreaBase(c, value).FillWidth().MinHeight(height).MaxHeight(260).Padding(8, 10).Font(theme.MonoFont).FontSize(theme.TextUI).
			Radius(theme.RadiusSmall).Background(theme.Sunken).InsetShadow(0, 0, 0, 0.5, theme.Border)
	})
}

// errorText shows a form's error, when it has one.
func errorText(c *ui.Context, err string) {
	if err != "" {
		ui.Text(c, err).FontSize(theme.TextUI).TextColor(theme.Error)
	}
}

// check is a checkbox on a row of its own, at its start.
func check(c *ui.Context, value *bool, label string) {
	ui.Row(c).FillWidth().Children(func() { widget.Checkbox(c, value, label) })
}
