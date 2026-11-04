package terminal

import "unicode"

// editor is the line being typed: its text, the cursor, and the lines sent before.
type editor struct {
	text []rune
	cur  int
	hist [][]rune
	at   int    // where Up has gone in hist; len(hist) is the line being typed
	kept []rune // the line being typed, while Up shows another
}

func (e *editor) empty() bool    { return len(e.text) == 0 }
func (e *editor) String() string { return string(e.text) }

func (e *editor) set(r []rune) {
	e.text = append([]rune(nil), r...)
	e.cur = len(e.text)
}

func (e *editor) clear() { e.set(nil) }

// insert types s at the cursor.
func (e *editor) insert(s string) {
	r := []rune(s)
	e.text = append(e.text[:e.cur:e.cur], append(r, e.text[e.cur:]...)...)
	e.cur += len(r)
}

// remember keeps a line sent, for Up, and clears the line.
func (e *editor) remember() {
	if !e.empty() {
		e.hist = append(e.hist, append([]rune(nil), e.text...))
	}
	e.at = len(e.hist)
	e.kept = nil
	e.clear()
}

// apply does an editing key; false when k edits nothing.
func (e *editor) apply(k Key) bool {
	switch k.Kind {
	case KeyRune:
		e.insert(string(k.Rune))
	case KeyPaste:
		e.insert(k.Text)
	case KeyNewline:
		e.insert("\n")
	case KeyTab:
		e.insert("    ")
	case KeyBackspace:
		if e.cur > 0 {
			e.text = append(e.text[:e.cur-1], e.text[e.cur:]...)
			e.cur--
		}
	case KeyDelete, KeyCtrlD:
		if e.cur < len(e.text) {
			e.text = append(e.text[:e.cur], e.text[e.cur+1:]...)
		}
	case KeyLeft:
		e.cur = max(e.cur-1, 0)
	case KeyRight:
		e.cur = min(e.cur+1, len(e.text))
	case KeyHome:
		e.cur = 0
	case KeyEnd:
		e.cur = len(e.text)
	case KeyKillLine:
		e.text = e.text[e.cur:]
		e.cur = 0
	case KeyKillEnd:
		e.text = e.text[:e.cur]
	case KeyKillWord:
		start := e.cur
		for start > 0 && unicode.IsSpace(e.text[start-1]) {
			start--
		}
		for start > 0 && !unicode.IsSpace(e.text[start-1]) {
			start--
		}
		e.text = append(e.text[:start], e.text[e.cur:]...)
		e.cur = start
	case KeyUp:
		if e.at > 0 {
			if e.at == len(e.hist) {
				e.kept = append([]rune(nil), e.text...)
			}
			e.at--
			e.set(e.hist[e.at])
		}
	case KeyDown:
		if e.at < len(e.hist) {
			e.at++
			if e.at == len(e.hist) {
				e.set(e.kept)
			} else {
				e.set(e.hist[e.at])
			}
		}
	default:
		return false
	}
	return true
}
