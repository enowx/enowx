package widget

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// MarkdownStream draws Markdown still arriving, several times a second, without
// parsing it whole each time: a blank line outside a fence ends every block, so
// what comes before the last one never changes and is parsed once, and only the
// rest is parsed again. Nothing goes in parsedCache or highlightCache, which would
// keep a copy of the text for every change. It belongs to the main thread.
type MarkdownStream struct {
	begun   bool
	last    string  // the text as last drawn
	settled []block // the blocks of last[:at]
	at      int     // where the part still parsed starts
	scanned int     // the lines before this are looked at
	fence   string  // the fence open at scanned, "" for none
	tail    []block // the blocks of last[at:]
}

// liveCodeKeep is how much of a code block still arriving shows, its end: laying
// out all of it for every change would be felt, and the whole shows once it closes.
const liveCodeKeep = 8 << 10

// update brings the stream up to src.
func (s *MarkdownStream) update(src string) {
	if s.begun && src == s.last {
		return
	}
	if !strings.HasPrefix(src, s.last) {
		*s = MarkdownStream{}
	}
	s.last, s.begun = src, true
	for {
		end := strings.IndexByte(src[s.scanned:], '\n')
		if end < 0 {
			break
		}
		line := strings.TrimSpace(src[s.scanned : s.scanned+end])
		next := s.scanned + end + 1
		switch {
		case s.fence != "":
			if strings.HasPrefix(line, s.fence) {
				s.fence = ""
			}
		case strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~"):
			s.fence = line[:3]
		case line == "":
			s.settled = append(s.settled, parseMarkdown(src[s.at:s.scanned])...)
			s.at = next
		}
		s.scanned = next
	}
	s.tail = parseMarkdown(src[s.at:])
}

// parseMarkdown is a text's blocks, not kept.
func parseMarkdown(src string) []block {
	return parseBlocks(strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n"))
}

// Draw draws src as MarkdownLinks does, in the same keys, so that it takes over from
// this with nothing moving once the text is whole.
func (s *MarkdownStream) Draw(c *ui.Context, src string, open LinkOpener) {
	s.update(src)
	ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
		for i, b := range s.settled {
			b.draw(c.Key(i), open)
		}
		for i, b := range s.tail {
			key := c.Key(len(s.settled) + i)
			if s.fence != "" && b.kind == "code" && i == len(s.tail)-1 {
				liveCode(key, b.lang, b.text)
				continue
			}
			b.draw(key, open)
		}
	})
}

// liveCode is a code block still arriving: CodeBlock's frame, its end in plain text.
// Lexing it for each change would be felt, and its colors come with its last fence.
func liveCode(c *ui.Context, lang, code string) {
	if len(code) > liveCodeKeep {
		cut := len(code) - liveCodeKeep
		for cut < len(code) && !utf8.RuneStart(code[cut]) {
			cut++
		}
		code = code[cut:]
		if nl := strings.IndexByte(code, '\n'); nl >= 0 {
			code = code[nl+1:]
		}
	}
	ui.Column(c).FillWidth().Radius(theme.RadiusCard).Clip().Border(1, theme.Border).Children(func() {
		ui.Row(c).FillWidth().Height(30).PaddingX(theme.S3).Gap(theme.S2).Background(theme.Raised).
			BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
			label := lang
			if label == "" {
				label = "text"
			}
			Running(c, label, RunStyle{Size: theme.TextLabel, Color: theme.Muted, Mono: true})
		})
		Scroll(c).FillWidth().Background(theme.Sunken).Children(func() {
			ui.Text(c, code).Font(theme.MonoFont).FontSize(theme.TextLabel).Padding(theme.S2, theme.S3).TextColor(theme.Text)
		})
	})
}
