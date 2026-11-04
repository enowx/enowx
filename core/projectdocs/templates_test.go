package projectdocs

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/core/notes"
)

// headings is the level 2 headings of a template, in order.
func headings(body string) []string {
	var out []string
	for _, s := range noteblocks.Outline(body) {
		if s.Level == 2 && s.Kind == "h2" {
			out = append(out, s.Title)
		}
	}
	return out
}

func TestEveryEntryHasACompleteTemplate(t *testing.T) {
	for _, e := range Catalog {
		body, err := Template(e.Key)
		if err != nil {
			t.Errorf("%s: %v", e.Key, err)
			continue
		}
		if !strings.HasPrefix(body, "::progress\n") {
			t.Errorf("%s: doesn't start with ::progress", e.Key)
		}
		hs := headings(body)
		if len(hs) < 2 || hs[len(hs)-2] != OpenQuestionsHeading || hs[len(hs)-1] != "Change history" {
			t.Errorf("%s: doesn't end with Open questions and Change history: %q", e.Key, hs)
		}
		if strings.ContainsRune(body, '—') {
			t.Errorf("%s: has an em dash", e.Key)
		}
		lines := noteblocks.Lines(body)
		fills := 0
		for _, l := range lines {
			if !strings.HasPrefix(l, ToFill) {
				continue
			}
			fills++
			if !strings.HasPrefix(l, ToFill+" ") || !strings.HasSuffix(l, "_") {
				t.Errorf("%s: malformed line %q", e.Key, l)
			}
		}
		if fills == 0 {
			t.Errorf("%s: nothing to fill", e.Key)
		}
		// Each section but the closing two asks for something, or a writer
		// has no way to tell it is still empty.
		for _, s := range noteblocks.Outline(body) {
			if s.Kind != "h2" || s.Title == OpenQuestionsHeading || s.Title == "Change history" {
				continue
			}
			if !slices.ContainsFunc(lines[s.Line+1:s.End], func(l string) bool { return strings.HasPrefix(l, ToFill) }) {
				t.Errorf("%s: section %q has no line to fill", e.Key, s.Title)
			}
		}
		if e.Diagram && e.Area != AreaUX {
			if !slices.Contains(hs, "Diagram") {
				t.Errorf("%s: a diagram without a Diagram section", e.Key)
			}
			if !strings.Contains(body, "_To fill: a canvas of kind diagram drawn from the schema and code, linked with docs diagram; no canvas yet._") {
				t.Errorf("%s: no canvas line", e.Key)
			}
			if !slices.Contains(hs, "What the diagram leaves out") {
				t.Errorf("%s: doesn't say what the diagram leaves out", e.Key)
			}
		}
	}
}

func TestEveryTopicsHeadingIsInItsTemplate(t *testing.T) {
	for _, tp := range Topics {
		body, err := Template(tp.Doc)
		if err != nil {
			t.Errorf("topic %q: %v", tp.Name, err)
			continue
		}
		if !slices.Contains(headings(body), tp.Heading) {
			t.Errorf("topic %q: %s has no heading %q", tp.Name, tp.Doc, tp.Heading)
		}
	}
}

var pageLink = regexp.MustCompile(`\[\[([^\]#]+)(?:#([^\]]+))?\]\]`)

// A link in a template is a promise to the writer: a misspelled page or
// section would stay in every project made from it.
func TestTemplateLinksResolve(t *testing.T) {
	for _, e := range Catalog {
		body, err := Template(e.Key)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range pageLink.FindAllStringSubmatch(body, -1) {
			target := Find(m[1])
			if target == nil || target.Name != m[1] {
				t.Errorf("%s: %s names no Catalog entry", e.Key, m[0])
				continue
			}
			if m[2] == "" {
				continue
			}
			to, err := Template(target.Key)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(headings(to), m[2]) {
				t.Errorf("%s: %s: no such section", e.Key, m[0])
			}
		}
	}
}

// An untested backup doesn't count as done, so its test stays a to-do.
func TestBackupsAskForARestoreTest(t *testing.T) {
	for _, key := range []string{"deployment-plan", "backup-dr"} {
		body, err := Template(key)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body, "- [ ] restore tested on (date):") {
			t.Errorf("%s: no restore test to tick", key)
		}
	}
}

func TestNewMakesBothPacks(t *testing.T) {
	for _, c := range []struct {
		size             string
		docs, areas, all int
	}{
		{SizeCore, 12, 0, 13},
		{SizeFull, 57, 10, 68},
	} {
		st := notes.Open(t.TempDir())
		root, made, err := New(Options{Title: "Shop", Size: c.size}, st.Save)
		if err != nil {
			t.Fatalf("%s: %v", c.size, err)
		}
		if len(made) != c.all {
			t.Errorf("%s: made %d notes, want %d", c.size, len(made), c.all)
		}
		list, err := st.List()
		if err != nil {
			t.Fatal(err)
		}
		p, err := Load(list, root.ID, st.Get)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Docs) != c.docs || len(p.Areas) != c.areas {
			t.Errorf("%s: %d documents and %d areas, want %d and %d", c.size, len(p.Docs), len(p.Areas), c.docs, c.areas)
		}
		for _, d := range p.Docs {
			if d.Entry == nil || d.Meta.Status != StatusDraft || len(d.Unfilled) == 0 {
				t.Errorf("%s: %q: entry %v, status %q, unfilled %d", c.size, d.Title, d.Entry != nil, d.Meta.Status, len(d.Unfilled))
			}
		}
	}
}
