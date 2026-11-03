package widget

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// Find in a LongText: off the main thread, the rows with a match marked, up to a limit.

func (l *LongText) isFound(row int) bool {
	if len(l.found) == 0 {
		return false
	}
	_, ok := slices.BinarySearch(l.found, row)
	return ok
}

// searchNow looks for what's asked, in the rows there are now.
func (l *LongText) searchNow(post func(func())) {
	q := strings.ToLower(l.find)
	l.findGen++
	gen := l.findGen
	if q == "" || !l.ready {
		l.found, l.foundAt = nil, 0
		return
	}
	lines, rows := l.lines, l.rows
	work := func() []int {
		var found []int
		for i, r := range rows {
			if strings.Contains(strings.ToLower(lines[r.line].text[r.from:r.to]), q) {
				found = append(found, i)
				if len(found) >= longFoundMost {
					break
				}
			}
		}
		return found
	}
	done := func(found []int) {
		if gen != l.findGen {
			return
		}
		l.found, l.foundAt = found, 0
		if len(found) > 0 {
			l.list.ScrollTo(found[0], ui.Center)
		}
	}
	if post == nil || len(l.text) <= longInline {
		done(work())
		return
	}
	go func() {
		found := work()
		post(func() { done(found) })
	}()
}

// FindBar is the search of the text: a field, how many there are, and the arrows.
func (l *LongText) FindBar(c *ui.Context, post func(func())) {
	ui.Row(c.Key("long-find")).FillWidth().Shrink(0).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		in := Input(c.Key("q"), &l.find, "Find in this text").Grow(1).MinWidth(0)
		if in.Changed() {
			l.searchNow(post)
		}
		if l.find == "" {
			return
		}
		word := "None"
		if n := len(l.found); n > 0 {
			word = strconv.Itoa(l.foundAt+1) + " of " + strconv.Itoa(n)
			if n >= longFoundMost {
				word += "+"
			}
		}
		ui.Text(c.Key("n"), word).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		step := func(by int) {
			if n := len(l.found); n > 0 {
				l.foundAt = (l.foundAt + by + n) % n
				l.list.ScrollTo(l.found[l.foundAt], ui.Center)
			}
		}
		if IconButton(c.Key("up"), icon.ChevronUp, "Previous match", false).Clicked() {
			step(-1)
		}
		if IconButton(c.Key("down"), icon.ChevronDown, "Next match", false).Clicked() || in.Submitted() {
			step(1)
		}
	})
}

// IndentJSON lays out JSON a value to a line, also when it's cut off partway, as the start of
// a long body is: it walks the bytes rather than parsing them.
func IndentJSON(data []byte) string {
	var out strings.Builder
	out.Grow(len(data) + len(data)/4)
	depth, inString, escaped := 0, false, false
	line := func() {
		out.WriteByte('\n')
		for range depth {
			out.WriteString("  ")
		}
	}
	for i := 0; i < len(data); i++ {
		ch := data[i]
		switch {
		case inString:
			out.WriteByte(ch)
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
		case ch == '"':
			inString = true
			out.WriteByte(ch)
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
		case ch == '{' || ch == '[':
			out.WriteByte(ch)
			if i+1 < len(data) && (data[i+1] == '}' || data[i+1] == ']') {
				out.WriteByte(data[i+1])
				i++
				continue
			}
			depth++
			line()
		case ch == '}' || ch == ']':
			depth = max(depth-1, 0)
			line()
			out.WriteByte(ch)
		case ch == ',':
			out.WriteByte(ch)
			line()
		case ch == ':':
			out.WriteString(": ")
		default:
			out.WriteByte(ch)
		}
	}
	return out.String()
}
