// Package docs ships enowX's documentation in the program, so an agent on any
// host can read how a feature works without the repository being there.
package docs

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

// Every top-level doc but PLAN.md, the roadmap: it says what is planned as much
// as what exists, which misleads an answer about what enowX does. plans/,
// spikes/ and decisions/ stay out for the same reason.
//
//go:embed [^P]*.md
var files embed.FS

// Names are the docs, sorted, without ".md".
func Names() []string {
	entries, _ := fs.ReadDir(files, ".")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(names)
	return names
}

// Text is doc name's Markdown; name may carry ".md" and any case.
func Text(name string) (string, error) {
	want := strings.ToLower(strings.TrimSuffix(name, ".md"))
	data, err := files.ReadFile(want + ".md")
	if err != nil {
		return "", fmt.Errorf("no doc %q: %s", name, strings.Join(Names(), ", "))
	}
	return string(data), nil
}

// Heading is one heading of a doc, and where its lines start.
type Heading struct {
	Level int
	Title string
	line  int
}

// Headings are text's headings, leaving out "#" comments in code blocks.
func Headings(text string) []Heading {
	var out []Heading
	fenced := false
	for i, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		level := len(line) - len(strings.TrimLeft(line, "#"))
		if level > 0 && level <= 6 && strings.HasPrefix(line[level:], " ") {
			out = append(out, Heading{Level: level, Title: strings.TrimSpace(line[level:]), line: i})
		}
	}
	return out
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	return strings.Trim(notSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// Section is the heading of text that query names (the whole title, its slug,
// or the first that holds it) down to the next heading as high. The error lists
// the headings there are.
func Section(text, query string) (string, error) {
	headings := Headings(text)
	want := slug(query)
	found := -1
	for i, h := range headings {
		if slug(h.Title) == want {
			found = i
			break
		}
	}
	for i := 0; found < 0 && i < len(headings); i++ {
		if want != "" && strings.Contains(slug(headings[i].Title), want) {
			found = i
		}
	}
	if found < 0 {
		titles := make([]string, len(headings))
		for i, h := range headings {
			titles[i] = h.Title
		}
		return "", fmt.Errorf("no section %q: %s", query, strings.Join(titles, "; "))
	}
	lines := strings.Split(text, "\n")
	end := len(lines)
	for _, h := range headings[found+1:] {
		if h.Level <= headings[found].Level {
			end = h.line
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[headings[found].line:end], "\n")) + "\n", nil
}

// Hit is a line of a doc that a search found.
type Hit struct {
	Doc, Section, Line string
	score              int
}

const hitLimit = 12

// Search finds the lines of every doc holding the most of query's words, all of
// them if any line does, the best first.
func Search(query string) []Hit {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	var hits []Hit
	best := 0
	for _, name := range Names() {
		text, _ := Text(name)
		section, fenced := "", false
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "```") {
				fenced = !fenced
			}
			if !fenced && strings.HasPrefix(line, "#") {
				section = strings.TrimSpace(strings.TrimLeft(line, "#"))
			}
			lower, score := strings.ToLower(line), 0
			for _, w := range words {
				if strings.Contains(lower, w) {
					score++
				}
			}
			if score > 0 {
				hits = append(hits, Hit{Doc: name, Section: section, Line: excerpt(line, lower, words), score: score})
				best = max(best, score)
			}
		}
	}
	kept := hits[:0]
	for _, h := range hits {
		if h.score == best {
			kept = append(kept, h)
		}
	}
	if len(kept) > hitLimit {
		kept = kept[:hitLimit]
	}
	return kept
}

const excerptLimit = 200

// excerpt is line around the first of words it holds; lower is line in lower case.
func excerpt(line, lower string, words []string) string {
	line = strings.TrimSpace(line)
	if len(line) <= excerptLimit {
		return line
	}
	at := len(lower)
	for _, w := range words {
		if i := strings.Index(lower, w); i >= 0 && i < at {
			at = i
		}
	}
	start := max(0, at-excerptLimit/2)
	for start > 0 && start < len(line) && line[start]&0xC0 == 0x80 {
		start-- // not in the middle of a character
	}
	end := min(len(line), start+excerptLimit)
	for end < len(line) && line[end]&0xC0 == 0x80 {
		end++
	}
	return "…" + line[start:end] + "…"
}

var (
	markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	sentenceEnd  = regexp.MustCompile(`[.!?](\s|$)`)
)

const summaryLimit = 180

// Summary is text's title, the first sentence of its first paragraph and its
// main sections, in a line, for the list of docs.
func Summary(text string) string {
	title, first := "", ""
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "```"):
			fenced = !fenced
		case fenced || line == "":
		case strings.HasPrefix(line, "#"):
			if title == "" {
				title = strings.TrimSpace(strings.TrimLeft(line, "#"))
			}
		case first == "" && !strings.HasPrefix(line, "|") && !strings.HasPrefix(line, ">"):
			first = line
		}
		if first != "" {
			break
		}
	}
	first = strings.NewReplacer("**", "", "`", "").Replace(markdownLink.ReplaceAllString(first, "$1"))
	if loc := sentenceEnd.FindStringIndex(first); loc != nil {
		first = first[:loc[0]+1]
	}
	if len(first) > summaryLimit {
		cut := strings.LastIndexByte(first[:summaryLimit], ' ')
		if cut <= 0 {
			cut = summaryLimit
		}
		first = first[:cut] + "…"
	}
	line := title
	if first != "" {
		line += ": " + first
	}
	return line + sectionsNote(text)
}

const sectionsLimit = 160

// sectionsNote names text's main sections, as a question's words are often in
// a section's title and not in the doc's first sentence.
func sectionsNote(text string) string {
	var titles []string
	n := 0
	for _, h := range Headings(text) {
		if h.Level != 2 {
			continue
		}
		if n += len(h.Title) + 2; n > sectionsLimit {
			break
		}
		titles = append(titles, h.Title)
	}
	if len(titles) == 0 {
		return ""
	}
	return " Sections: " + strings.Join(titles, "; ") + "."
}
