// Package platformview is the page of Claude Platform's settings, a sheet: its API keys,
// which live in the credential vault, and its system prompt on a host. It also has the
// words a chat and the new session dialog say of them. It gets what it needs through
// Options and never sees a key's value.
package platformview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The sheet's tabs.
const (
	TabKeys = iota
	TabPrompt
)

// ConsoleURL is where an Anthropic account shows its credit: an ordinary API key can't
// read it.
const ConsoleURL = "https://console.anthropic.com/settings/billing"

// sheetWidth is how wide the sheet is, in a window that leaves it.
const sheetWidth = 640

// View is the sheet's state, owned by the main thread.
type View struct {
	open   bool
	tab    int
	prompt promptState
	// tested says how the last Test of a key went, by ID.
	tested map[string]string
}

// Open opens the sheet on tab.
func (v *View) Open(tab int) { v.open, v.tab = true, tab }

// IsOpen says the sheet shows.
func (v *View) IsOpen() bool { return v.open }

// Sheet draws the sheet while open: a tab each for the keys and the prompt.
func (v *View) Sheet(c *ui.Context, o Options) {
	if !v.open {
		return
	}
	widget.Sheet(c, &v.open, sheetWidth, func() {
		widget.SheetHead(c, &v.open, "Claude Platform", "Its API keys and its system prompt on "+o.Host+".")
	}, func() {
		if i := widget.Segmented(c.Key("tab"), []string{"API keys", "System prompt"}, v.tab, nil); i >= 0 {
			v.tab = i
		}
		if v.tab == TabPrompt {
			v.promptBody(c, o)
		} else {
			v.keysBody(c, o)
		}
	}, func() {
		if v.tab == TabPrompt {
			v.promptButtons(c, o)
		} else {
			v.keysButtons(c, o)
		}
	})
}

// quiet is a line of the sheet's plain words, in the room it has.
func quiet(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
}
