package widget

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/iconlib"
	"github.com/enowx/enowx/ui/theme"
)

// HasInlineIcons says line s draws icons in it (:icon[lib:name]).
func HasInlineIcons(s string) bool { return iconlib.InlinePattern.MatchString(s) }

// InlineIcons draws a line of Markdown as InlineLabel does, with the
// icons in it: a text can't hold an icon, so its words and icons flow in
// lines of their own, size high, in ink, weight heavy, in italics or not.
func InlineIcons(c *ui.Context, s string, size float32, weight int, ink ui.Color, italic bool) ui.Element {
	return ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Children(func() {
		k := 0
		word := func(text string, sp span) {
			// A text ends at its last letter: its space is a gap after it.
			trimmed := strings.TrimRight(text, " ")
			defer func() {
				if trimmed != text {
					ui.Box(c.Key(k)).Width(size * 0.28).Height(1)
					k++
				}
			}()
			if trimmed == "" {
				return
			}
			t := ui.Text(c.Key(k), trimmed).FontSize(size).FontWeight(weight).TextColor(ink)
			k++
			switch {
			case sp.code:
				t.Font(theme.MonoFont).FontSize(size - 2).Background(theme.Raised).TextColor(theme.Now)
			case sp.url != "":
				t.TextColor(theme.Now).Underline()
			}
			if sp.bold {
				t.Bold().TextColor(theme.TextStrong)
			}
			if sp.italic || italic {
				t.Italic()
			}
			if sp.struck {
				t.Strikethrough().TextColor(theme.Muted)
			}
		}
		at := 0
		for _, m := range iconlib.InlinePattern.FindAllStringSubmatchIndex(s, -1) {
			for _, sp := range inlineSpans(s[at:m[0]]) {
				for _, w := range splitWords(sp.text) {
					word(w, sp)
				}
			}
			ref := s[m[2]:m[3]]
			ui.Row(c.Key(k)).Height(size * 1.4).Center().PaddingX(1).Children(func() {
				NoteIcon(c, ref, size*1.05, ink, nil).Tooltip(ref)
			})
			k++
			at = m[1]
		}
		for _, sp := range inlineSpans(s[at:]) {
			for _, w := range splitWords(sp.text) {
				word(w, sp)
			}
		}
	})
}

// splitWords splits s after each space, so its words wrap apart and keep
// the spaces between them.
func splitWords(s string) []string {
	var out []string
	for s != "" {
		i := strings.IndexByte(s, ' ')
		if i < 0 {
			return append(out, s)
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}
