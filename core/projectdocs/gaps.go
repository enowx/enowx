package projectdocs

import (
	"strings"

	"github.com/enowx/enowx/core/noteblocks"
)

// Gap is a section of a document still to write: a Topic's, with its question
// to ask, or another (Name and Question "").
type Gap struct {
	Area      string `json:"area,omitempty"`
	Name      string `json:"name,omitempty"`
	Question  string `json:"question,omitempty"`
	DocID     string `json:"docId"`
	Doc       string `json:"doc"`
	Section   string `json:"section"`
	SectionID string `json:"sectionId,omitempty"`
}

// Gaps is what's left to write in p: each Topic in order, then the documents'
// other unfilled sections. A document the project doesn't have asks nothing,
// and a section with a Skipped line is settled.
func (p *Project) Gaps() []Gap {
	var out []Gap
	covered := map[string]bool{}
	for _, t := range Topics {
		d := p.Doc(t.Doc)
		if d == nil {
			continue
		}
		sections := noteblocks.Outline(d.Body)
		s, err := noteblocks.Find(sections, t.Heading)
		if err != nil {
			continue
		}
		for _, in := range sections {
			if s.Line <= in.Line && in.End <= s.End {
				covered[d.ID+"#"+in.ID] = true
			}
		}
		if !sectionHas(d.Body, s, Skipped) && sectionHas(d.Body, s, ToFill) {
			out = append(out, Gap{Area: t.Area, Name: t.Name, Question: t.Question, DocID: d.ID, Doc: d.Title, Section: s.Title, SectionID: s.ID})
		}
	}
	for _, d := range p.Docs {
		sections := noteblocks.Outline(d.Body)
		for _, id := range d.Unfilled {
			if covered[d.ID+"#"+id] {
				continue
			}
			g := Gap{DocID: d.ID, Doc: d.Title, SectionID: id}
			if id != "" {
				s, err := noteblocks.Find(sections, id)
				if err != nil || sectionHas(d.Body, s, Skipped) {
					continue
				}
				g.Section = s.Title
			}
			out = append(out, g)
		}
	}
	return out
}

// sectionHas says a line of section s starts with prefix.
func sectionHas(body string, s noteblocks.Section, prefix string) bool {
	lines := noteblocks.Lines(body)
	for _, l := range lines[s.Line+1 : min(s.End, len(lines))] {
		if strings.HasPrefix(strings.TrimSpace(l), prefix) {
			return true
		}
	}
	return false
}

// StripMeta is body without what records a document's state and origin, for
// a template to start other documents from.
func StripMeta(body string) string {
	var out []string
	for _, l := range noteblocks.Lines(body) {
		if !isMetaLine(l) {
			out = append(out, l)
		}
	}
	return strings.TrimLeft(strings.Join(out, "\n"), "\n")
}
