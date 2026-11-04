package projectdocs

import (
	"regexp"
	"strings"

	"github.com/enowx/enowx/core/noteblocks"
)

// Link says the section under which it is written is based on another
// document (or one section of it) as it was at Hash. It reads as a line of
// text anywhere: "Based on: [[PRD#Features]] @9f8e7d6c, [[Brief]] @1a2b3c4d".
type Link struct {
	Doc, Section string // the source: a document's title, and one section's or ""
	Hash         string
	In           string // the #ID of the section it's in, "" for the whole document
}

var linkRe = regexp.MustCompile(`\[\[([^\]|#]+)(?:#([^\]|]+))?(?:\|[^\]]*)?\]\]\s*@([0-9a-f]{8})`)

// Links is what body says it's based on.
func Links(body string) []Link {
	var out []Link
	lines := noteblocks.Lines(body)
	sections := noteblocks.Outline(body)
	for i, line := range lines {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), basedPrefix)
		if !ok {
			continue
		}
		in := ""
		for _, s := range sections {
			if s.Line < i && i < s.End {
				in = s.ID // the innermost: sections come outermost first
			}
		}
		for _, m := range linkRe.FindAllStringSubmatch(rest, -1) {
			out = append(out, Link{Doc: strings.TrimSpace(m[1]), Section: strings.TrimSpace(m[2]), Hash: m[3], In: in})
		}
	}
	return out
}

// Ref is a link as written.
func (l Link) Ref() string {
	if l.Section != "" {
		return "[[" + l.Doc + "#" + l.Section + "]] @" + l.Hash
	}
	return "[[" + l.Doc + "]] @" + l.Hash
}

// AddLink is body saying it's based on l: under the section l.In, or under
// its first line; a link to the same source there is brought up to date.
func AddLink(body string, l Link) (string, error) {
	lines := noteblocks.Lines(body)
	at := 0
	if _, ok := Parse(body); ok {
		at = 1
	}
	if l.In != "" {
		s, err := noteblocks.Find(noteblocks.Outline(body), l.In)
		if err != nil {
			return "", err
		}
		at = s.Line + 1
		l.In = s.ID
	}
	old := Links(body)
	for i, line := range lines {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), basedPrefix)
		if !ok {
			continue
		}
		for _, o := range old {
			if o.Doc == l.Doc && o.Section == l.Section && o.In == l.In && strings.Contains(rest, o.Ref()) {
				lines[i] = strings.Replace(line, o.Ref(), l.Ref(), 1)
				return strings.Join(lines, "\n"), nil
			}
		}
	}
	lines = append(lines[:at:at], append([]string{"", basedPrefix + l.Ref(), ""}, lines[at:]...)...)
	return strings.Join(lines, "\n"), nil
}
