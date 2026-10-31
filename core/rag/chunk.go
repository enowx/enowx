package rag

import (
	"path"
	"regexp"
	"strings"
)

// chunkChars bounds a chunk's text, so with its header it stays under
// small embedding models' 512-token input (code runs ~3 chars a token).
const chunkChars = 1000

// chunk is a piece of a file: its text, the lines it spans (1-based, end
// included) and the symbol it starts with ("" when none).
type chunk struct {
	text       string
	start, end int
	symbol     string
}

// declarations find where a top-level declaration starts, by language,
// the name it declares in the last group.
var declarations = map[string]*regexp.Regexp{
	".go":     regexp.MustCompile(`^(?:func|type|var|const)\s+(?:\([^)]*\)\s*)?([A-Za-z_][\w]*)`),
	".py":     regexp.MustCompile(`^(?:async\s+)?(?:def|class)\s+([A-Za-z_]\w*)`),
	".js":     jsDecl,
	".jsx":    jsDecl,
	".ts":     jsDecl,
	".tsx":    jsDecl,
	".vue":    jsDecl,
	".svelte": jsDecl,
	".rs":     regexp.MustCompile(`^(?:pub(?:\([\w:]+\))?\s+)?(?:async\s+)?(?:fn|struct|enum|trait|impl|mod|type|const|static)\s+(?:<[^>]*>\s*)?([A-Za-z_]\w*)`),
	".java":   classDecl,
	".kt":     classDecl,
	".swift":  classDecl,
	".cs":     classDecl,
	".php":    classDecl,
	".rb":     regexp.MustCompile(`^(?:def|class|module)\s+([\w.:?!]+)`),
	".md":     regexp.MustCompile(`^#{1,3}\s+(.+)`),
	".proto":  regexp.MustCompile(`^(?:message|service|enum)\s+(\w+)`),
	".sql":    regexp.MustCompile(`(?i)^(?:create|alter)\s+(?:table|view|index|function)\s+(?:if\s+not\s+exists\s+)?([\w.]+)`),
}

var (
	jsDecl    = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?(?:function\*?|class|interface|type|enum|const|let)\s+([A-Za-z_$][\w$]*)`)
	classDecl = regexp.MustCompile(`^(?:(?:public|private|protected|internal|static|final|abstract|open|override|data|sealed|export)\s+)*(?:class|interface|enum|struct|record|fun|func|function|object|protocol|extension)\s+([A-Za-z_]\w*)`)
	comment   = regexp.MustCompile(`^\s*(?://|#|/\*|\*|--|"""|///)`)
)

// chunkFile cuts file name's text into chunks of at most max characters at
// its top-level declarations (with the comments right above them), small
// ones packed together; a declaration too long is cut at blank lines, then
// at lines. A language with no declarations known is cut at blank lines.
func chunkFile(text, name string, max int) []chunk {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	decl := declarations[strings.ToLower(path.Ext(name))]
	type seg struct {
		start, end int // 0-based, end excluded
		symbol     string
	}
	var segs []seg
	cur := seg{}
	begin := func(i int, symbol string) {
		if i > cur.start {
			cur.end = i
			segs = append(segs, cur)
		}
		cur = seg{start: i, symbol: symbol}
	}
	for i, line := range lines {
		if decl != nil {
			if m := decl.FindStringSubmatch(line); m != nil {
				at := i
				for at > cur.start && comment.MatchString(lines[at-1]) {
					at--
				}
				begin(at, strings.TrimSpace(m[len(m)-1]))
			}
		} else if strings.TrimSpace(line) == "" && i > cur.start {
			begin(i, "")
		}
	}
	cur.end = len(lines)
	segs = append(segs, cur)

	var out []chunk
	emit := func(s, e int, symbol string) {
		body := strings.Join(lines[s:e], "\n")
		if strings.TrimSpace(body) != "" {
			out = append(out, chunk{text: body, start: s + 1, end: e, symbol: symbol})
		}
	}
	packS, packE, packSym := -1, -1, ""
	flush := func() {
		if packS >= 0 {
			emit(packS, packE, packSym)
			packS, packE, packSym = -1, -1, ""
		}
	}
	for _, sg := range segs {
		if linesSize(lines[sg.start:sg.end]) > max {
			flush()
			for _, c := range cutLong(lines, sg.start, sg.end, max) {
				emit(c[0], c[1], sg.symbol)
			}
			continue
		}
		if packS >= 0 && linesSize(lines[packS:sg.end]) > max {
			flush()
		}
		if packS < 0 {
			packS, packSym = sg.start, sg.symbol
		} else if packSym == "" {
			packSym = sg.symbol
		}
		packE = sg.end
	}
	flush()
	return out
}

// cutLong cuts lines[s:e] into spans of at most max characters, at blank
// lines when it can, else at a line; a single line longer is cut alone.
func cutLong(lines []string, s, e, max int) [][2]int {
	var out [][2]int
	start, n, lastBlank := s, 0, -1
	for i := s; i < e; i++ {
		l := len(lines[i]) + 1
		if n+l > max && i > start {
			cut := i
			if lastBlank > start {
				cut = lastBlank
			}
			out = append(out, [2]int{start, cut})
			start, n, lastBlank = cut, linesSize(lines[cut:i]), -1
		}
		n += l
		if strings.TrimSpace(lines[i]) == "" {
			lastBlank = i
		}
	}
	if start < e {
		out = append(out, [2]int{start, e})
	}
	return out
}

func linesSize(lines []string) int {
	n := 0
	for _, l := range lines {
		n += len(l) + 1
	}
	return n
}

// secretName finds files that hold secrets, never indexed.
var secretName = regexp.MustCompile(`(?i)(^|/)(\.env(\..*)?|\.envrc|\.npmrc|\.netrc|\.pypirc|\.git-credentials|id_(rsa|dsa|ecdsa|ed25519)(\.pub)?|.*\.(pem|key|p12|pfx|jks|keystore|crt|cer|der|kdbx|ovpn)|credentials(\.\w+)?|.*secrets?\.(json|ya?ml|toml|env)|service[-_]account.*\.json)$`)

// secretText finds text that holds a secret: a private key, a cloud key, a
// token.
var secretText = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----|\bAKIA[0-9A-Z]{16}\b|\bgh[pousr]_[A-Za-z0-9]{30,}\b|\bsk-[A-Za-z0-9_-]{20,}\b|\bxox[abpr]-[A-Za-z0-9-]{10,}\b`)

// IsSecret reports whether the file at name (relative, with slashes) holds
// secrets, by its name.
func IsSecret(name string) bool { return secretName.MatchString(name) }
