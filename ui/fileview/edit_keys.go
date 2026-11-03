package fileview

import (
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The keys people expect of a code editor, and find and replace.

// key handles a key in the text; true when it did. What changes the text is done as the next
// frame is built, as the area can't take a change while it handles a key.
func (d *Doc) key(c *ui.Context, o Options, k ui.TextKey) bool {
	e := d.edit
	later := func(fn func()) bool {
		e.pending = fn
		c.After(0)
		return true
	}
	primary := primaryMod()
	switch {
	case k.Mods == primary && k.Key == ui.KeyS:
		return later(func() { d.save(o, false) })
	case k.Mods == primary && k.Key == ui.KeyZ:
		return later(e.stepBack)
	case k.Mods == primary|ui.Shift && k.Key == ui.KeyZ, k.Mods == ui.Ctrl && k.Key == ui.KeyY:
		return later(e.stepForward)
	case k.Mods == primary && k.Key == ui.KeyF:
		e.find.open = true
		c.After(0)
		return true
	case k.Mods == ui.Ctrl && k.Key == ui.KeyG:
		e.gotoOpen = true
		c.After(0)
		return true
	case k.Mods == primary && k.Key == ui.KeySlash:
		prefix := commentPrefix(d.path)
		if prefix == "" {
			return false
		}
		return later(func() { e.toggleComment(prefix) })
	case k.Mods == 0 && k.Key == ui.KeyTab:
		return later(func() { e.indent(false) })
	case k.Mods == ui.Shift && k.Key == ui.KeyTab:
		return later(func() { e.indent(true) })
	case k.Mods == 0 && k.Key == ui.KeyEnter:
		return later(e.newLine)
	case k.Mods == 0 && k.Key == ui.KeyEscape && (e.find.open || e.gotoOpen):
		e.find.open, e.gotoOpen = false, false
		e.find.hits = nil
		c.After(0)
		return true
	}
	return false
}

// indentUnit is what the file indents with: a tab, or as many spaces as its first indented
// line has.
func (e *editor) indentUnit() string {
	snap := e.buf.Snapshot()
	for i := 0; i < min(snap.LineCount(), 400); i++ {
		line := snap.Line(i)
		if strings.HasPrefix(line, "\t") {
			return "\t"
		}
		if n := len(line) - len(strings.TrimLeft(line, " ")); n > 0 && n < len(line) {
			return strings.Repeat(" ", n)
		}
	}
	return "    "
}

// indent indents or outdents the lines selected; with none selected and going in, it adds
// an indent where the caret is.
func (e *editor) indent(out bool) {
	unit := e.indentUnit()
	a, b := min(e.selA, e.selB), max(e.selA, e.selB)
	snap := e.buf.Snapshot()
	first, last := snap.LineAt(a), snap.LineAt(b)
	if !out && first == last {
		n := utf8.RuneCountInString(unit)
		e.change(a, b, unit, a+n, a+n)
		return
	}
	start, _ := snap.LineRange(first)
	_, end := snap.LineRange(last)
	var lines []string
	for i := first; i <= last; i++ {
		line := snap.Line(i)
		if out {
			switch {
			case strings.HasPrefix(line, unit):
				line = line[len(unit):]
			default:
				line = strings.TrimLeft(line[:min(len(line), len(unit))], " \t") + line[min(len(line), len(unit)):]
			}
		} else if line != "" {
			line = unit + line
		}
		lines = append(lines, line)
	}
	text := strings.Join(lines, "\n")
	e.change(start, end, text, start, start+utf8.RuneCountInString(text))
}

// newLine breaks the line, the next starting as indented as this one.
func (e *editor) newLine() {
	a, b := min(e.selA, e.selB), max(e.selA, e.selB)
	snap := e.buf.Snapshot()
	line := snap.Line(snap.LineAt(a))
	lead := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	n := 1 + utf8.RuneCountInString(lead)
	e.change(a, b, "\n"+lead, a+n, a+n)
}

// toggleComment comments the lines selected out, or back in when they all are.
func (e *editor) toggleComment(prefix string) {
	a, b := min(e.selA, e.selB), max(e.selA, e.selB)
	snap := e.buf.Snapshot()
	first, last := snap.LineAt(a), snap.LineAt(b)
	start, _ := snap.LineRange(first)
	_, end := snap.LineRange(last)
	all := true
	lead := -1
	for i := first; i <= last; i++ {
		line := snap.Line(i)
		body := strings.TrimLeft(line, " \t")
		if body == "" {
			continue
		}
		if !strings.HasPrefix(body, prefix) {
			all = false
		}
		if n := len(line) - len(body); lead < 0 || n < lead {
			lead = n
		}
	}
	var lines []string
	for i := first; i <= last; i++ {
		line := snap.Line(i)
		body := strings.TrimLeft(line, " \t")
		switch {
		case body == "":
		case all:
			body = strings.TrimPrefix(strings.TrimPrefix(body, prefix), " ")
			line = line[:len(line)-len(strings.TrimLeft(line, " \t"))] + body
		default:
			line = line[:lead] + prefix + " " + line[lead:]
		}
		lines = append(lines, line)
	}
	text := strings.Join(lines, "\n")
	e.change(start, end, text, start, start+utf8.RuneCountInString(text))
}

// commentPrefix is how a file comments out a line, "" where it's not known.
func commentPrefix(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".c", ".h", ".cc", ".cpp", ".hpp", ".java", ".kt", ".swift", ".rs", ".cs", ".dart", ".scss", ".less", ".php", ".m", ".proto":
		return "//"
	case ".py", ".rb", ".sh", ".bash", ".zsh", ".yml", ".yaml", ".toml", ".conf", ".pl", ".r", ".ex", ".exs", ".mk", ".dockerfile":
		return "#"
	case ".sql", ".lua", ".hs":
		return "--"
	case ".vim":
		return "\""
	case ".clj", ".lisp", ".el":
		return ";"
	}
	if name := strings.ToLower(path.Base(p)); name == "makefile" || name == "dockerfile" {
		return "#"
	}
	return ""
}

// findBar is the search of the file being edited.
type findBar struct {
	open    bool
	q, with string
	hits    [][2]int // in runes
	at      int
	gen     int
}

// findHits are where q is in text, in runes, ignoring case where that keeps offsets.
func findHits(text, q string) [][2]int {
	if q == "" {
		return nil
	}
	hay, needle := text, q
	if low := strings.ToLower(text); len(low) == len(text) {
		hay, needle = low, strings.ToLower(q)
	}
	const most = 5000
	var hits [][2]int
	runes, from := 0, 0
	n := utf8.RuneCountInString(needle)
	for len(hits) < most {
		i := strings.Index(hay[from:], needle)
		if i < 0 {
			break
		}
		runes += utf8.RuneCountInString(hay[from : from+i])
		hits = append(hits, [2]int{runes, runes + n})
		runes += n
		from += i + len(needle)
	}
	return hits
}

// search looks for what's asked, off the main thread.
func (e *editor) search(o Options) {
	e.find.gen++
	gen, q, snap := e.find.gen, e.find.q, e.buf.Snapshot()
	go func() {
		hits := findHits(snap.String(), q)
		o.Update(func() {
			if gen != e.find.gen {
				return
			}
			e.find.hits, e.find.at = hits, 0
			for i, h := range hits {
				if h[0] >= e.selA {
					e.find.at = i
					break
				}
			}
			if len(hits) > 0 {
				h := hits[e.find.at]
				e.setSel = &[2]int{h[0], h[1]}
			}
		})
	}()
}

func (e *editor) findView(c *ui.Context, o Options) {
	f := &e.find
	if !f.open {
		return
	}
	step := func(by int) {
		if n := len(f.hits); n > 0 {
			f.at = (f.at + by + n) % n
			h := f.hits[f.at]
			e.setSel = &[2]int{h[0], h[1]}
		}
	}
	ui.Row(c.Key("find")).FillWidth().Shrink(0).Wrap().Padding(theme.S2, theme.S3).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		in := widget.Input(c.Key("q"), &f.q, "Find").Width(220).AutoFocus()
		if in.Changed() {
			e.search(o)
		}
		word := "None"
		if n := len(f.hits); n > 0 {
			word = strconv.Itoa(f.at+1) + " of " + strconv.Itoa(n)
		}
		if f.q != "" {
			ui.Text(c.Key("n"), word).FontSize(theme.TextLabel).TextColor(ui.Hex("#8A8FA3")).SingleLine().Shrink(0)
		}
		if widget.Button(c.Key("prev"), widget.Secondary, nil, "Previous", "").Clicked() {
			step(-1)
		}
		if widget.Button(c.Key("next"), widget.Secondary, nil, "Next", "").Clicked() || in.Submitted() {
			step(1)
		}
		widget.Input(c.Key("with"), &f.with, "Replace with").Width(220)
		if widget.Button(c.Key("one"), widget.Secondary, nil, "Replace", "").Clicked() && len(f.hits) > 0 {
			h := f.hits[f.at]
			n := utf8.RuneCountInString(f.with)
			e.change(h[0], h[1], f.with, h[0], h[0]+n)
			e.search(o)
		}
		if widget.Button(c.Key("all"), widget.Secondary, nil, "Replace all", "").Clicked() && len(f.hits) > 0 {
			hits := f.hits
			e.pushUndo()
			for i := len(hits) - 1; i >= 0; i-- {
				e.buf.Replace(hits[i][0], hits[i][1], f.with)
			}
			e.current = editPoint{snap: e.buf.Snapshot()}
			e.afterChange()
			e.search(o)
		}
	})
}

// gotoView asks which line to go to.
func (e *editor) gotoView(c *ui.Context) {
	if !e.gotoOpen {
		return
	}
	ui.Row(c.Key("goto")).FillWidth().Shrink(0).Padding(theme.S2, theme.S3).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		in := widget.Input(c.Key("line"), &e.gotoLine, "Go to line").Width(160).AutoFocus()
		if in.Submitted() {
			if n, err := strconv.Atoi(strings.TrimSpace(e.gotoLine)); err == nil {
				a, _ := e.buf.Snapshot().LineRange(max(min(n-1, e.buf.LineCount()-1), 0))
				e.setSel = &[2]int{a, a}
				e.gotoOpen, e.gotoLine = false, ""
			}
		}
	})
}
