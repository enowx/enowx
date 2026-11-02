package widget

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// In every style a page's parts stay inside a fixed frame, whatever their words.
func TestStylesKeepPartsInTheirFrame(t *testing.T) {
	old, palette := theme.CurrentStyle(), theme.Current()
	defer func() {
		theme.UseStyle(old)
		theme.Apply(palette)
	}()
	const w, h = 420, 300
	long := "A very long title that doesn't fit in the frame it is drawn in, however it is styled"
	for _, s := range theme.Styles {
		theme.UseStyle(s)
		theme.Apply(palette)
		value := ""
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Size(w, h).Clip().Children(func() {
				PageHeader(c, long, long, nil)
				Input(c, &value, "Field")
				Button(c, Primary, nil, "Do it", "")
				Card(c).Children(func() { ui.Text(c, long).FontSize(theme.TextUI) })
			})
		}, w, h)
		tt.Frame()
		tt.Frame()
		for _, label := range []string{"Field", "Do it"} {
			if b, ok := tt.Find(label); !ok || b.X < 0 || b.X+b.W > w || b.Y+b.H > h {
				t.Errorf("%s: %q at %+v", s.ID, label, b)
			}
		}
		if in, ok := tt.Find("Field"); ok && in.H != theme.ControlHeight {
			t.Errorf("%s: a field %v high", s.ID, in.H)
		}
	}
}
