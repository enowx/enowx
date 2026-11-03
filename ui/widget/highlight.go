package widget

import (
	"path"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// Code is colored by its language's lexer (every language the lexer
// library knows), in the theme's colors: the kind of each piece is kept,
// its color picked when drawn, so a theme change shows at once.

// Span is a piece of a line of code and what kind of token it is.
type Span struct {
	Text string
	kind spanKind
}

type spanKind uint8

const (
	plainSpan spanKind = iota
	commentSpan
	keywordSpan
	stringSpan
	numberSpan
	nameSpan // functions, classes, tags: what a line is about
	attrSpan // attributes, properties, builtins
	punctSpan
	insertedSpan
	deletedSpan
)

func (k spanKind) color() ui.Color {
	switch k {
	case commentSpan:
		return theme.Muted
	case keywordSpan:
		return theme.Working
	case stringSpan, insertedSpan:
		return theme.Done
	case numberSpan:
		return theme.Warning
	case nameSpan:
		return theme.Accent
	case attrSpan:
		return theme.Warning
	case punctSpan:
		return theme.TextSoft
	case deletedSpan:
		return theme.Error
	}
	return theme.Text
}

func kindOf(t chroma.TokenType) spanKind {
	switch {
	case t.InCategory(chroma.Comment):
		return commentSpan
	case t == chroma.GenericInserted:
		return insertedSpan
	case t == chroma.GenericDeleted:
		return deletedSpan
	case t == chroma.GenericSubheading || t == chroma.GenericHeading:
		return keywordSpan
	case t.InCategory(chroma.Keyword), t == chroma.NameTag, t == chroma.NameDecorator:
		return keywordSpan
	case t.InSubCategory(chroma.LiteralString):
		return stringSpan
	case t.InSubCategory(chroma.LiteralNumber):
		return numberSpan
	case t == chroma.NameFunction, t == chroma.NameClass, t == chroma.NameFunctionMagic, t == chroma.NameNamespace:
		return nameSpan
	case t == chroma.NameAttribute, t == chroma.NameBuiltin, t == chroma.NameBuiltinPseudo, t == chroma.NameConstant,
		t == chroma.NameProperty:
		return attrSpan
	case t.InCategory(chroma.Operator), t.InCategory(chroma.Punctuation):
		return punctSpan
	}
	return plainSpan
}

// alsoKnownAs are languages of extensions the lexers don't claim.
var alsoKnownAs = map[string]string{".mdx": "markdown", ".jsonc": "json", ".cjs": "javascript", ".mts": "typescript", ".cts": "typescript"}

// lexerFor finds the lexer of a file named name, else of language lang
// (as a code block names it), else by what code looks like; nil for text.
func lexerFor(name, lang, code string) chroma.Lexer {
	var l chroma.Lexer
	if name != "" {
		l = lexers.Match(name)
		if l == nil {
			l = lexers.Get(alsoKnownAs[strings.ToLower(path.Ext(name))])
		}
	}
	if l == nil && lang != "" {
		l = lexers.Get(lang)
	}
	if l == nil && name == "" && lang == "" {
		l = lexers.Analyse(code)
	}
	if l == nil || l.Config().Name == "plaintext" {
		return nil
	}
	return chroma.Coalesce(l)
}

// HighlightLines splits code into lines of colored spans, the file named
// name (or in language lang) lexed whole, so that comments and strings
// over several lines are colored throughout. nil when it is text.
func HighlightLines(name, lang, code string) [][]Span {
	code = strings.ReplaceAll(code, "\r\n", "\n")
	l := lexerFor(name, lang, code)
	if l == nil {
		return nil
	}
	it, err := l.Tokenise(nil, code)
	if err != nil {
		return nil
	}
	lines := [][]Span{nil}
	for tok := it(); tok != chroma.EOF; tok = it() {
		k := kindOf(tok.Type)
		parts := strings.Split(tok.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				lines = append(lines, nil)
			}
			if p != "" {
				last := len(lines) - 1
				lines[last] = append(lines[last], Span{Text: p, kind: k})
			}
		}
	}
	if len(lines) > 1 && len(lines[len(lines)-1]) == 0 && strings.HasSuffix(code, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// Color is the span's color.
func (s Span) Color() ui.Color { return s.kind.color() }

// CodeSpans draws one line of HighlightLines in a RichText it returns for
// the caller to style; plain stands for the line when it has no spans.
func CodeSpans(c *ui.Context, spans []Span, plain string) ui.Element {
	return ui.RichText(c).Children(func() {
		if spans == nil {
			ui.Text(c, plain).TextColor(theme.TextSoft)
			return
		}
		for i, s := range spans {
			ui.Text(c.Key(i), s.Text).TextColor(s.kind.color())
		}
	})
}

// highlightCache keeps what was lexed lately: a code block is drawn frame after frame, and
// lexing it each time is what made a long one lag. It belongs to the main thread.
var highlightCache = map[[2]string][][]Span{}

const highlightKeep = 256

func highlightedOnce(lang, code string) [][]Span {
	key := [2]string{lang, code}
	if lines, ok := highlightCache[key]; ok {
		return lines
	}
	if len(highlightCache) >= highlightKeep {
		clear(highlightCache)
	}
	lines := HighlightLines("", lang, code)
	highlightCache[key] = lines
	return lines
}

// highlight draws a code block in lang inside a RichText; one that names
// no language shows plain.
func highlight(c *ui.Context, lang, code string) {
	var lines [][]Span
	if lang != "" && len(code) <= longHighlight {
		lines = highlightedOnce(lang, code)
	}
	if lines == nil {
		ui.Text(c, code).TextColor(theme.Text)
		return
	}
	n := 0
	for i, line := range lines {
		if i > 0 {
			ui.Text(c.Key(n), "\n")
			n++
		}
		for _, s := range line {
			ui.Text(c.Key(n), s.Text).TextColor(s.kind.color())
			n++
		}
	}
}
