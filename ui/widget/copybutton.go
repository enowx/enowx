package widget

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
)

// CopyButton is CopyIcon with its words: for a dialog whose main point is what
// to copy. It keeps its label's width by saying Copied in the same place.
func CopyButton(c *ui.Context, state *CopyFeedback, label, value string) {
	svg, text := icon.Copy, label
	if remaining := state.until.Sub(c.Now()); remaining > 0 {
		svg, text = icon.Check, strings.Replace(label, "Copy", "Copied", 1)
		c.After(remaining)
	}
	if Button(c, Secondary, svg, text, "").Label(label).Clicked() {
		c.WriteClipboard(value)
		state.until = c.Now().Add(1500 * time.Millisecond)
		c.After(0)
	}
}
