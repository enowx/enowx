package widget

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// LongText shows text of any length without the window feeling it. Whatever takes time
// (formatting it, lexing it, cutting it into rows, searching it) runs off the main thread
// and is done once; what's drawn each frame is only the rows in view, and a row is never
// longer than fits, so a line of a megabyte costs what a screenful does. A line too long for
// the width carries on in the next row, marked, instead of being shaped whole and clipped.
//
// It belongs to the main thread; the work it starts posts its results back through Post.
type LongText struct {
	id       string
	gen      int // the latest work asked for: older results are dropped
	ready    bool
	text     string
	lines    []longLine
	rows     []longRow
	cols     int  // what rows are cut for, in characters
	cutting  bool // new rows are being cut
	wantCols int  // the width changed meanwhile
	list     ui.ListState

	findOpen bool
	find     string
	found    []int // rows with a match, in order
	foundAt  int
	findGen  int
	goneTo   int // the line last scrolled to, for GoToLine

	Copied CopyFeedback
}

type longLine struct {
	text  string
	spans []Span // nil when it isn't highlighted
}

// longRow is the part of a line a row shows, in bytes.
type longRow struct {
	line, from, to int
}

// LongSource is what a LongText shows. ID says what it is: the same ID isn't prepared again.
type LongSource struct {
	ID string
	// Make gives the text, formatted as it should be, and the language it's in, "" for
	// none: it runs off the main thread when Size says it's long.
	Make func() (text, lang string)
	Size int
	// Name picks how it's highlighted when Make names no language: a file's name.
	Name string
	// Keep stays where it was scrolled to: the same text having grown or changed.
	Keep bool
}

// LongOptions are how a LongText draws.
type LongOptions struct {
	// Post runs fn on the main thread, and has the window drawn again.
	Post func(fn func())
	// Head and Foot draw above and below the text, scrolling with it.
	Head, Foot func()
	// Numbers shows line numbers.
	Numbers bool
	// MarkFrom and MarkTo are lines to mark (1-based).
	MarkFrom, MarkTo int
	// OnCode: colors for CodeGround, whose dark stays in light mode.
	OnCode bool
	// Ink is the plain text's color; zero for the theme's.
	Ink ui.Color
}

const (
	// longInline is how long a text may be to be prepared where it's asked for: small enough
	// not to be noticed, and it shows with no "Reading…" between.
	longInline = 32 << 10
	// longSelectable is how long a text may be to draw as one piece, which can be selected
	// from line to line; longer, each row selects on its own and Copy takes it all.
	longSelectable = 16 << 10
	// longHighlight is the most that's lexed: past it, colors aren't worth the wait.
	longHighlight = 256 << 10
	// longTabWidth is what a tab is drawn as.
	longTabWidth = 4
	// longRowHeight is a row's height.
	longRowHeight = 19
	// longMonoAspect is a monospace character's width for its size, a little over so that
	// a row cut for a width never runs past it.
	longMonoAspect = .62
	longFoundMost  = 20000
)

// Text is the text as prepared, for Copy and Save.
func (l *LongText) Text() string { return l.text }

// Ready says the text is prepared.
func (l *LongText) Ready() bool { return l.ready }

// Load shows src, unless it already does.
func (l *LongText) Load(src LongSource, post func(func())) {
	if src.ID == l.id {
		return
	}
	l.id = src.ID
	l.gen++
	gen, cols := l.gen, max(l.cols, 120)
	l.found, l.foundAt = nil, 0
	finish := func(text string, lines []longLine, rows []longRow) {
		l.text, l.lines, l.rows, l.cols, l.ready = text, lines, rows, cols, true
		if !src.Keep {
			l.list = ui.ListState{}
		}
		l.searchNow(post)
	}
	if src.Size <= longInline || post == nil {
		text, lines, rows := prepareLong(src, cols)
		finish(text, lines, rows)
		return
	}
	l.ready = l.ready && src.Keep // what shows stays until what's new is ready
	go func() {
		text, lines, rows := prepareLong(src, cols)
		post(func() {
			if gen == l.gen {
				finish(text, lines, rows)
			}
		})
	}()
}

// prepareLong makes the text, its lines and its rows.
func prepareLong(src LongSource, cols int) (string, []longLine, []longRow) {
	made, lang := src.Make()
	text := strings.ReplaceAll(made, "\r\n", "\n")
	lines := make([]longLine, 0, strings.Count(text, "\n")+1)
	var spans [][]Span
	if (src.Name != "" || lang != "") && len(text) <= longHighlight {
		spans = HighlightLines(src.Name, lang, text)
	}
	for i, rest := 0, text; ; i++ {
		cut := strings.IndexByte(rest, '\n')
		one := rest
		if cut >= 0 {
			one = rest[:cut]
		}
		ln := longLine{text: one}
		if i < len(spans) {
			ln.spans = spans[i]
			if ln.spans == nil {
				ln.spans = []Span{}
			}
		}
		lines = append(lines, ln)
		if cut < 0 {
			break
		}
		rest = rest[cut+1:]
	}
	if n := len(lines); n > 1 && lines[n-1].text == "" {
		lines = lines[:n-1] // a final newline isn't a line of its own
	}
	return text, lines, cutRows(lines, cols)
}

// cutRows cuts lines into rows of at most cols characters, wide ones counting double.
func cutRows(lines []longLine, cols int) []longRow {
	rows := make([]longRow, 0, len(lines))
	for i := range lines {
		s := lines[i].text
		if len(s) <= cols && !strings.ContainsRune(s, '\t') && asciiOnly(s) {
			rows = append(rows, longRow{i, 0, len(s)}) // nearly every line
			continue
		}
		from, used := 0, 0
		for at, r := range s {
			w := 1
			switch {
			case r == '\t':
				w = longTabWidth
			case r >= 0x1100:
				w = 2
			}
			if used+w > cols {
				rows = append(rows, longRow{i, from, at})
				from, used = at, 0
			}
			used += w
		}
		rows = append(rows, longRow{i, from, len(s)})
	}
	return rows
}

func asciiOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// rewrap cuts the rows anew for a width, off the main thread when there are many.
func (l *LongText) rewrap(cols int, post func(func())) {
	if cols == l.cols || !l.ready {
		return
	}
	if l.cutting {
		l.wantCols = cols
		return
	}
	lines, gen := l.lines, l.gen
	if len(l.text) <= longInline || post == nil {
		l.rows, l.cols = cutRows(lines, cols), cols
		l.searchNow(post)
		return
	}
	l.cutting, l.wantCols = true, 0
	go func() {
		rows := cutRows(lines, cols)
		post(func() {
			l.cutting = false
			if gen != l.gen {
				return
			}
			l.rows, l.cols = rows, cols
			l.searchNow(post)
			if l.wantCols != 0 {
				l.rewrap(l.wantCols, post)
			}
		})
	}()
}

// Draw draws it, filling what its parent has left.
func (l *LongText) Draw(c *ui.Context, o LongOptions) ui.Element {
	head, foot := 0, 0
	if o.Head != nil {
		head = 1
	}
	if o.Foot != nil {
		foot = 1
	}
	ink := o.Ink
	if ink.A == 0 {
		ink = theme.Text
		if o.OnCode {
			ink = CodeText
		}
	}
	whole := l.ready && len(l.text) <= longSelectable && !o.Numbers
	body := len(l.rows)
	switch {
	case !l.ready:
		body = 1
	case whole:
		body = 1
	}
	numbers := float32(0)
	if o.Numbers {
		numbers = max(float32(len(strconv.Itoa(len(l.lines))))*8+theme.S4, 44)
	}
	e := ui.List(c, &l.list, head+body+foot, func(i int) {
		switch {
		case i < head:
			ui.Column(c.Key("head")).FillWidth().Gap(theme.S2).Padding(0, 0, theme.S2, 0).Children(o.Head)
		case i >= head+body:
			ui.Column(c.Key("foot")).FillWidth().Gap(theme.S2).Padding(theme.S2, 0, 0, 0).Children(o.Foot)
		case !l.ready:
			ui.Text(c.Key("reading"), "Reading…").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		case whole:
			l.drawWhole(c, o, ink)
		default:
			l.drawRow(c, o, i-head, numbers, ink)
		}
	}).Grow(1).FillWidth().MinHeight(0)
	if w := e.LayoutBounds().W; w > 0 {
		avail := w - numbers - theme.S4*2 - ScrollGutter
		size := theme.TextUI
		l.rewrap(max(int(avail/(size*longMonoAspect)), 20), o.Post)
	}
	return e
}

// drawWhole is a short text as one piece, to select across its lines.
func (l *LongText) drawWhole(c *ui.Context, o LongOptions, ink ui.Color) {
	ui.RichText(c.Key("whole")).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(ink).Selectable().FillWidth().Children(func() {
		n := 0
		for i, ln := range l.lines {
			if i > 0 {
				ui.Text(c.Key(n), "\n")
				n++
			}
			if ln.spans == nil {
				ui.Text(c.Key(n), strings.ReplaceAll(ln.text, "\t", "    "))
				n++
				continue
			}
			for _, s := range ln.spans {
				ui.Text(c.Key(n), s.Text).TextColor(spanColor(s, o.OnCode))
				n++
			}
		}
	})
}

// drawRow is one row: the part of its line that fits.
func (l *LongText) drawRow(c *ui.Context, o LongOptions, i int, numbers float32, ink ui.Color) {
	r := l.rows[i]
	ln := l.lines[r.line]
	row := ui.Row(c).FillWidth().Height(longRowHeight).Clip()
	marked := r.line+1 >= o.MarkFrom && r.line+1 <= o.MarkTo && o.MarkFrom > 0
	if marked || l.isFound(i) {
		row.Background(theme.Selected)
	}
	row.Children(func() {
		if o.Numbers {
			word := ""
			if r.from == 0 {
				word = strconv.Itoa(r.line + 1)
			}
			ui.Text(c, word).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(ui.Hex("#6E7386")).Width(numbers).PaddingX(theme.S2).TextAlign(ui.End).SingleLine().Shrink(0)
		}
		if r.from > 0 {
			ui.Icon(c, icon.Reply).FontSize(10).TextColor(CodeDim).Shrink(0).Margin(0, 2, 0, 0)
		}
		part := ln.text[r.from:r.to]
		if ln.spans == nil || len(ln.spans) == 0 {
			ui.Text(c.Key("text"), strings.ReplaceAll(part, "\t", "    ")).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(ink).SingleLine().Shrink(1).Selectable()
			return
		}
		ui.RichText(c.Key("text")).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(ink).SingleLine().Shrink(1).Children(func() {
			for k, s := range spansBetween(ln.spans, r.from, r.to) {
				ui.Text(c.Key(k), strings.ReplaceAll(s.Text, "\t", "    ")).TextColor(spanColor(s, o.OnCode))
			}
		})
	})
}

func spanColor(s Span, onCode bool) ui.Color {
	if onCode {
		return codeColor(s.kind)
	}
	return s.kind.color()
}

// spansBetween is the part of a line's spans from byte from to byte to.
func spansBetween(spans []Span, from, to int) []Span {
	var out []Span
	at := 0
	for _, s := range spans {
		end := at + len(s.Text)
		if end > from && at < to {
			a, b := max(from-at, 0), min(to-at, len(s.Text))
			out = append(out, Span{Text: s.Text[a:b], kind: s.kind})
		}
		at = end
		if at >= to {
			break
		}
	}
	return out
}

// longCode is a long code block: a window on it with its own scrolling, whose rows only are
// drawn, as thousands of colored runs laid out each frame is what makes a block like this lag.
func longCode(c *ui.Context, lang, code string) {
	const most = 400
	box := ui.Column(c.Key("long-code")).FillWidth().Height(most).Background(theme.Sunken).Padding(theme.S2, 0, theme.S2, theme.S3)
	long := ui.Local(box, "long-text", func() LongText { return LongText{} })
	if len(code) > longInline*2 {
		lang = "" // what's asked for here is done where it's asked, and lexing would be felt
	}
	long.Load(LongSource{ID: code, Make: func() (string, string) { return code, lang }}, nil)
	box.Children(func() { long.Draw(c.Key("text"), LongOptions{}) })
}

// GoToLine scrolls line (1-based) into view, once for each line asked.
func (l *LongText) GoToLine(line int) {
	if !l.ready || line == l.goneTo {
		return
	}
	for i, r := range l.rows {
		if r.line+1 >= line {
			l.goneTo = line
			l.list.ScrollTo(i, ui.Center)
			return
		}
	}
}

// LongLine is s as much as a row that clips can show, for lists whose rows are one line each:
// shaping a whole megabyte to show its first hundred characters is what would hurt.
func LongLine(s string) string {
	const most = 2000
	if len(s) <= most {
		return s
	}
	cut := most
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// LongWords is s as much as a wrapping row is worth showing, saying how much more there is.
func LongWords(s string) string {
	const most = 8000
	if len(s) <= most {
		return s
	}
	cut := most
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "… (" + strconv.Itoa(len(s)-cut) + " more bytes)"
}
