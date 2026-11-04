package terminal

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"golang.org/x/text/width"
)

// ANSI styles.
const (
	styleDim    = "\x1b[2m"
	styleRed    = "\x1b[31m"
	styleGreen  = "\x1b[32m"
	styleYellow = "\x1b[33m"
	styleCyan   = "\x1b[36m"
	styleBold   = "\x1b[1m"
	styleReset  = "\x1b[0m"
)

// maxInputRows and maxPartialRows bound the live region, which has to fit the screen
// to be redrawn: a long message shows around its cursor, a long streamed paragraph
// goes up a row at a time.
const (
	maxInputRows   = 5
	maxPartialRows = 3
)

// screen draws on the terminal without an alternate screen: lines go up into the
// scrollback as any program's, and the last rows are a live region redrawn in place
// (the streamed line under way, thinking folded, the status, the input). The terminal
// wraps; rows are only counted, to find the region's top again. With live false
// (standard output isn't a terminal) it writes lines alone, in plain text. Only Run's
// goroutine uses it.
type screen struct {
	out  io.Writer
	live bool
	cols func() int

	buf strings.Builder // what the current update writes

	partial, partialStyle string // the streamed line not ended yet
	atLineStart           bool   // plain mode: whether the output sits at a line's start

	// The live region as it should be.
	thinking string
	footer   string
	marker   string
	input    []rune
	cursor   int

	// The live region as last drawn: its lines' widths and the cursor's column count
	// from the input's start. Recounted at the current width, a terminal that reflowed
	// on a resize is still found, or close to it.
	drawn       []int
	drawnCursor int
	shown       string // what was drawn, to skip drawing it again
	erased      bool
	lastCols    int
}

func newScreen(out io.Writer, live bool, cols func() int) *screen {
	return &screen{out: out, live: live, cols: cols, atLineStart: true}
}

func (s *screen) width() int {
	if w := s.cols(); w > 0 {
		return w
	}
	return 80
}

// paint styles text when the output takes styles.
func (s *screen) paint(style, text string) string {
	if !s.live || style == "" || text == "" {
		return text
	}
	return style + text + styleReset
}

// erase takes the live region off the screen, the cursor at its top.
func (s *screen) erase() {
	if s.erased {
		return
	}
	s.erased = true
	if len(s.drawn) == 0 {
		return
	}
	w := s.width()
	up := s.drawnCursor / w
	for _, cols := range s.drawn[:len(s.drawn)-1] {
		up += rowsOf(cols, w)
	}
	s.buf.WriteString("\r")
	if up > 0 {
		fmt.Fprintf(&s.buf, "\x1b[%dA", up)
	}
	s.buf.WriteString("\x1b[J")
	s.drawn, s.shown = nil, ""
}

// rowsOf is how many rows a line cols wide takes.
func rowsOf(cols, w int) int {
	if cols <= 0 {
		return 1
	}
	return (cols + w - 1) / w
}

// Line puts a line above the live region, for good. A line under way ends first.
func (s *screen) Line(text string) {
	s.EndStream()
	if !s.live {
		s.buf.WriteString(text + "\n")
		s.atLineStart = true
		return
	}
	s.erase()
	s.buf.WriteString(strings.ReplaceAll(text, "\n", "\r\n") + "\r\n")
}

// Stream adds streamed text in style, shown as it comes.
func (s *screen) Stream(text, style string) {
	if text == "" {
		return
	}
	if !s.live {
		s.buf.WriteString(text)
		s.atLineStart = strings.HasSuffix(text, "\n")
		return
	}
	s.partialStyle = style
	s.partial += text
	for {
		line, rest, ok := strings.Cut(s.partial, "\n")
		if !ok {
			break
		}
		s.erase()
		s.buf.WriteString(s.paint(style, line) + "\r\n")
		s.partial = rest
	}
	w := s.width()
	for displayWidth(s.partial) > maxPartialRows*w {
		head, rest := cutRow(s.partial, w)
		s.erase()
		s.buf.WriteString(s.paint(style, head) + "\r\n")
		s.partial = rest
	}
}

// EndStream ends the line under way, if one is.
func (s *screen) EndStream() {
	if !s.live {
		if !s.atLineStart {
			s.buf.WriteString("\n")
			s.atLineStart = true
		}
		return
	}
	if s.partial != "" {
		s.erase()
		s.buf.WriteString(s.paint(s.partialStyle, s.partial) + "\r\n")
		s.partial = ""
	}
}

// cutRow cuts a row of at most w columns off s, at a space where there is one in its
// second half.
func cutRow(s string, w int) (head, rest string) {
	cols, cut, space := 0, 0, -1
	for i, r := range s {
		c := runeCols(r)
		if cols+c > w {
			break
		}
		cols += c
		cut = i + len(string(r))
		if r == ' ' && cols > w/2 {
			space = i
		}
	}
	if space > 0 {
		return s[:space], s[space+1:]
	}
	return s[:cut], s[cut:]
}

// Flush draws the live region as it should be now and writes all of the update.
func (s *screen) Flush() {
	if s.live {
		s.drawLive()
	}
	if s.buf.Len() > 0 {
		_, _ = io.WriteString(s.out, s.buf.String())
		s.buf.Reset()
	}
}

// drawLive redraws the live region when it changed or was erased.
func (s *screen) drawLive() {
	w := s.width()
	if w < 2 {
		w = 2
	}
	footer := clipCols(s.footer, w-1)
	thinking := clipCols(s.thinking, w-1)
	display, cursorCols := inputWindow(s.input, s.cursor, displayWidth(s.marker), w)
	want := fmt.Sprint(w, "|", s.partial, "|", thinking, "|", footer, "|", s.marker, "|", display, "|", cursorCols)
	if !s.erased && want == s.shown && w == s.lastCols {
		return
	}
	s.erase()
	s.lastCols = w

	var drawn []int
	if s.partial != "" {
		s.buf.WriteString(s.paint(s.partialStyle, s.partial) + "\r\n")
		drawn = append(drawn, displayWidth(s.partial))
	}
	if thinking != "" {
		s.buf.WriteString(s.paint(styleDim, thinking) + "\r\n")
		drawn = append(drawn, displayWidth(thinking))
	}
	if footer != "" {
		s.buf.WriteString(s.paint(styleDim, footer) + "\r\n")
		drawn = append(drawn, displayWidth(footer))
	}
	s.buf.WriteString(s.marker + display)
	total := displayWidth(s.marker) + displayWidth(display)
	if total > 0 && total%w == 0 {
		s.buf.WriteString("\r\n") // out of the last column's pending wrap
	}
	drawn = append(drawn, total)
	if up := total/w - cursorCols/w; up > 0 {
		fmt.Fprintf(&s.buf, "\x1b[%dA", up)
	}
	fmt.Fprintf(&s.buf, "\x1b[%dG", cursorCols%w+1)
	s.drawn, s.drawnCursor, s.shown, s.erased = drawn, cursorCols, want, false
}

// inputWindow is the input as shown after a marker: line breaks as one mark each, and
// around the cursor alone when it wouldn't fit. It also says the cursor's column
// counted from the marker's start.
func inputWindow(input []rune, cursor, markerCols, w int) (string, int) {
	shown := make([]rune, len(input))
	for i, r := range input {
		switch {
		case r == '\n':
			shown[i] = '↵'
		case r < 0x20:
			shown[i] = ' '
		default:
			shown[i] = r
		}
	}
	limit := maxInputRows*w - markerCols - 2
	if limit < 4 {
		limit = 4
	}
	if displayWidth(string(shown)) <= limit {
		return string(shown), markerCols + displayWidth(string(shown[:cursor]))
	}
	// Up to the cursor first, what follows it in what room is left.
	start, cols := cursor, 0
	for start > 0 && cols+runeCols(shown[start-1]) <= limit-2 {
		start--
		cols += runeCols(shown[start])
	}
	end := cursor
	for end < len(shown) && cols+runeCols(shown[end]) <= limit-2 {
		cols += runeCols(shown[end])
		end++
	}
	out, cursorCols := string(shown[start:end]), markerCols+displayWidth(string(shown[start:cursor]))
	if start > 0 {
		out = "…" + out
		cursorCols++
	}
	if end < len(shown) {
		out += "…"
	}
	return out, cursorCols
}

// Close takes the live region away, leaving the cursor where the shell should go on.
func (s *screen) Close() {
	s.EndStream()
	if s.live {
		s.erase()
	}
	if s.buf.Len() > 0 {
		_, _ = io.WriteString(s.out, s.buf.String())
		s.buf.Reset()
	}
}

// runeCols is the columns a rune takes on a terminal.
func runeCols(r rune) int {
	switch {
	case r < 0x20 || r == 0x7f:
		return 0
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// displayWidth is the columns s takes, which holds no escape sequences.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeCols(r)
	}
	return n
}

// clipCols cuts s to at most n columns, with a mark where it was cut.
func clipCols(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if displayWidth(s) <= n {
		return s
	}
	cols, cut := 0, 0
	for i, r := range s {
		c := runeCols(r)
		if cols+c > n-1 {
			break
		}
		cols += c
		cut = i + len(string(r))
	}
	return s[:cut] + "…"
}

// cleanText is text a model or a tool wrote, safe to print: no control characters, so
// it can't move the cursor or fake what the terminal shows; tabs are spaces, columns
// being counted.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

// oneLine is s as one line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(cleanText(s)), " ")
}
