package terminal

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

// KeyKind is what a key does, whichever bytes the terminal sent for it.
type KeyKind int

const (
	KeyRune  KeyKind = iota // Rune typed
	KeyPaste                // Text pasted, line breaks kept
	KeyEnter
	KeyNewline // Alt+Enter, Shift+Enter: a line break in the message
	KeyEsc
	KeyCtrlC
	KeyCtrlD
	KeyShiftTab
	KeyTab
	KeyBackspace
	KeyDelete
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyKillLine // Ctrl+U
	KeyKillEnd  // Ctrl+K
	KeyKillWord // Ctrl+W
	KeyRedraw   // Ctrl+L
)

// Key is one key pressed, or one paste.
type Key struct {
	Kind KeyKind
	Rune rune
	Text string
}

type parseState int

const (
	inText parseState = iota
	inEsc
	inCSI
	inSS3
	inPaste
)

const (
	pasteOn  = "\x1b[?2004h" // what turns bracketed paste on
	pasteOff = "\x1b[?2004l"
	pasteEnd = "\x1b[201~"
)

// maxSeq bounds an escape sequence kept while it is being read.
const maxSeq = 64

// Parser turns the bytes of a terminal into keys. It is a pure state machine: bytes
// may be cut anywhere between reads. A lone Esc can't be told from the start of a
// sequence until a moment passes, so the owner calls Flush when Pending stayed true
// for that moment.
type Parser struct {
	state  parseState
	seq    []byte // the sequence so far, or a rune's first bytes
	paste  []byte
	lastCR bool
}

// Pending says bytes wait for what follows them: a timer should run.
func (p *Parser) Pending() bool {
	return p.state == inEsc || p.state == inCSI || p.state == inSS3
}

// Flush ends what waits when nothing followed it: Esc alone, or nothing for a cut
// sequence.
func (p *Parser) Flush() []Key {
	was := p.state
	if !p.Pending() {
		return nil
	}
	p.state, p.seq = inText, p.seq[:0]
	if was == inEsc {
		return []Key{{Kind: KeyEsc}}
	}
	return nil
}

// Feed reads one read's bytes and returns the keys they finish.
func (p *Parser) Feed(data []byte) []Key {
	var out []Key
	for i := 0; i < len(data); i++ {
		b := data[i]
		switch p.state {
		case inText:
			out = p.text(b, out)
		case inEsc:
			switch {
			case b == '[':
				p.state, p.seq = inCSI, p.seq[:0]
			case b == 'O':
				p.state, p.seq = inSS3, p.seq[:0]
			case b == 0x1b:
				out = append(out, Key{Kind: KeyEsc}) // the first was alone; this one may start a sequence
			case b == '\r' || b == '\n':
				p.state = inText
				out = append(out, Key{Kind: KeyNewline})
			case b < 0x20 || b == 0x7f:
				p.state = inText
				out = append(out, Key{Kind: KeyEsc})
				out = p.text(b, out)
			case i == 0:
				// A later read: Esc pressed, then text sent (an interrupt, then a message).
				p.state = inText
				out = append(out, Key{Kind: KeyEsc})
				out = p.text(b, out)
			default:
				p.state = inText // Alt+key, in one read as terminals send it: nothing here
			}
		case inCSI:
			p.seq = append(p.seq, b)
			if b >= 0x40 && b <= 0x7e {
				p.state = inText
				seq := string(p.seq)
				p.seq = p.seq[:0]
				out = p.csi(seq, out)
			} else if len(p.seq) > maxSeq {
				p.state, p.seq = inText, p.seq[:0]
			}
		case inSS3:
			p.state = inText
			if k, ok := arrowKey(b); ok {
				out = append(out, Key{Kind: k})
			}
		case inPaste:
			p.paste = append(p.paste, b)
			if b == '~' && bytes.HasSuffix(p.paste, []byte(pasteEnd)) {
				text := cleanPaste(string(p.paste[:len(p.paste)-len(pasteEnd)]))
				p.paste, p.state = nil, inText
				if text != "" {
					out = append(out, Key{Kind: KeyPaste, Text: text})
				}
			}
		}
	}
	return out
}

// text reads byte b outside any sequence.
func (p *Parser) text(b byte, out []Key) []Key {
	if len(p.seq) > 0 || b >= 0x80 {
		p.lastCR = false
		p.seq = append(p.seq, b)
		if !utf8.FullRune(p.seq) {
			return out
		}
		r, size := utf8.DecodeRune(p.seq)
		rest := append([]byte(nil), p.seq[size:]...)
		p.seq = p.seq[:0]
		if r != utf8.RuneError || size > 1 {
			out = append(out, Key{Kind: KeyRune, Rune: r})
		}
		for _, c := range rest {
			out = p.text(c, out)
		}
		return out
	}
	if b == '\n' && p.lastCR {
		p.lastCR = false
		return out // the rest of a CRLF
	}
	p.lastCR = b == '\r'
	if b >= 0x20 && b != 0x7f {
		return append(out, Key{Kind: KeyRune, Rune: rune(b)})
	}
	kind, ok := KeyRune, true
	switch b {
	case 0x1b:
		p.state = inEsc
		return out
	case '\r', '\n':
		kind = KeyEnter
	case 0x03:
		kind = KeyCtrlC
	case 0x04:
		kind = KeyCtrlD
	case 0x7f, 0x08:
		kind = KeyBackspace
	case '\t':
		kind = KeyTab
	case 0x01:
		kind = KeyHome
	case 0x05:
		kind = KeyEnd
	case 0x02:
		kind = KeyLeft
	case 0x06:
		kind = KeyRight
	case 0x10:
		kind = KeyUp
	case 0x0e:
		kind = KeyDown
	case 0x15:
		kind = KeyKillLine
	case 0x0b:
		kind = KeyKillEnd
	case 0x17:
		kind = KeyKillWord
	case 0x0c:
		kind = KeyRedraw
	default:
		ok = false
	}
	if !ok {
		return out
	}
	return append(out, Key{Kind: kind})
}

// csi reads a whole CSI sequence, without its ESC [.
func (p *Parser) csi(seq string, out []Key) []Key {
	final := seq[len(seq)-1]
	params := seq[:len(seq)-1]
	first, _, _ := strings.Cut(params, ";")
	switch final {
	case 'Z':
		return append(out, Key{Kind: KeyShiftTab})
	case '~':
		n, _ := strconv.Atoi(first)
		switch n {
		case 200:
			p.state, p.paste = inPaste, p.paste[:0]
		case 1, 7:
			return append(out, Key{Kind: KeyHome})
		case 4, 8:
			return append(out, Key{Kind: KeyEnd})
		case 3:
			return append(out, Key{Kind: KeyDelete})
		}
		return out
	case 'u':
		if params == "13;2" || params == "13;3" {
			return append(out, Key{Kind: KeyNewline})
		}
		return out
	}
	if k, ok := arrowKey(final); ok {
		return append(out, Key{Kind: k})
	}
	return out
}

// arrowKey is the key a sequence ending in final means, arrows and Home, End.
func arrowKey(final byte) (KeyKind, bool) {
	switch final {
	case 'A':
		return KeyUp, true
	case 'B':
		return KeyDown, true
	case 'C':
		return KeyRight, true
	case 'D':
		return KeyLeft, true
	case 'H':
		return KeyHome, true
	case 'F':
		return KeyEnd, true
	}
	return 0, false
}

// cleanPaste keeps a paste's text and line breaks, and no control characters: a paste
// can't draw on the screen or press keys.
func cleanPaste(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			return -1
		}
		return r
	}, s)
}
