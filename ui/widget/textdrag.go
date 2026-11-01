package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// TextDrag is text dragged within the window, as a pointer at a session,
// for a chat's composer or a terminal to take as if typed.
type TextDrag struct{ Text string }

// TakeTextDrag makes e take TextDrag, outlined while one is over it, and
// says the text dropped on it since the last frame.
func TakeTextDrag(e ui.Element) (string, bool) {
	if d, ok := ui.Drop[TextDrag](e); ok {
		return d.Text, true
	}
	if _, ok := ui.DragOver[TextDrag](e); ok {
		e.Border(1, theme.Accent)
	}
	return "", false
}
