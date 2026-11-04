package projectdocs

import (
	"fmt"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/proto"
)

// Doc is a document of a project, read.
type Doc struct {
	proto.Note
	Meta    Meta
	Entry   *Entry // of the catalog, nil for the user's own kinds
	Links   []Link
	Stale   []Stale
	Changed bool // ChangedSinceApproval
	// Done and Todos count its to-dos; Unfilled names sections still to write.
	Done, Todos int
	Unfilled    []string
}

// Stale says a link's source has changed since this document was based on it.
type Stale struct {
	Link   Link
	Reason string // "changed", or "gone" when the source is no more
}

// Project is a project note and every document under it.
type Project struct {
	Note  proto.Note
	Meta  Meta
	Areas []proto.NoteInfo
	Docs  []Doc
}

// Reader reads what a project needs of the notes: a daemon's client, or a
// store.
type Reader func(id string) (proto.Note, error)

// Load reads the project at note id from the list of notes, finding the
// project above id when id is one of its documents.
func Load(list []proto.NoteInfo, id string, read Reader) (*Project, error) {
	byID := map[string]proto.NoteInfo{}
	children := map[string][]proto.NoteInfo{}
	for _, n := range list {
		byID[n.ID] = n
		children[n.Parent] = append(children[n.Parent], n)
	}
	cur, ok := byID[id]
	if !ok {
		return nil, fmt.Errorf("projectdocs: no note %q", id)
	}
	for depth := 0; ; depth++ {
		n, err := read(cur.ID)
		if err != nil {
			return nil, err
		}
		if m, ok := Parse(n.Body); ok && m.IsProject() {
			p := &Project{Note: n, Meta: m}
			if err := p.walk(children, cur.ID, read); err != nil {
				return nil, err
			}
			p.markStale()
			return p, nil
		}
		parent, ok := byID[cur.Parent]
		if !ok || depth > 32 {
			return nil, fmt.Errorf("projectdocs: %q isn't in a project", id)
		}
		cur = parent
	}
}

func (p *Project) walk(children map[string][]proto.NoteInfo, id string, read Reader) error {
	for _, c := range children[id] {
		n, err := read(c.ID)
		if err != nil {
			return err
		}
		m, ok := Parse(n.Body)
		switch {
		case ok && m.Kind == KindArea:
			p.Areas = append(p.Areas, c)
		case ok && m.IsDocument():
			done, todos := noteblocks.Progress(n.Body)
			p.Docs = append(p.Docs, Doc{Note: n, Meta: m, Entry: Find(m.Kind), Links: Links(n.Body),
				Changed: ChangedSinceApproval(n.Body), Done: done, Todos: todos, Unfilled: Unfilled(n.Body)})
		}
		if err := p.walk(children, c.ID, read); err != nil {
			return err
		}
	}
	return nil
}

// Doc finds a document by its ID, title or kind.
func (p *Project) Doc(name string) *Doc {
	for i := range p.Docs {
		d := &p.Docs[i]
		if d.ID == name || strings.EqualFold(d.Title, name) || strings.EqualFold(d.Meta.Kind, name) ||
			d.Entry != nil && strings.EqualFold(d.Entry.Key, name) {
			return d
		}
	}
	return nil
}

func (p *Project) markStale() {
	for i := range p.Docs {
		d := &p.Docs[i]
		for _, l := range d.Links {
			src := p.Doc(l.Doc)
			if src == nil {
				d.Stale = append(d.Stale, Stale{Link: l, Reason: "gone"})
				continue
			}
			now := ContentHash(src.Body)
			if l.Section != "" {
				var err error
				if now, err = SectionHash(src.Body, l.Section); err != nil {
					d.Stale = append(d.Stale, Stale{Link: l, Reason: "gone"})
					continue
				}
			}
			if now != l.Hash {
				d.Stale = append(d.Stale, Stale{Link: l, Reason: "changed"})
			}
		}
	}
}

// ToFill starts a line a template leaves for the writer; Skipped starts one
// saying why a section doesn't apply, so it isn't asked for again.
const (
	ToFill  = "_To fill:"
	Skipped = "Skipped:"
)

// Unfilled is the sections of body with a line still to fill, by their #ID,
// the part before any heading as "".
func Unfilled(body string) []string {
	lines := noteblocks.Lines(body)
	sections := noteblocks.Outline(body)
	var out []string
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), ToFill) {
			continue
		}
		in := ""
		for _, s := range sections {
			if s.Line < i && i < s.End {
				in = s.ID
			}
		}
		if !slices.Contains(out, in) {
			out = append(out, in)
		}
	}
	return out
}

// Blocker is why a project may not be built yet.
type Blocker struct {
	Doc, Why string
}

// Gate says what still stops the project being built: its gated documents
// not approved, or changed since. Nothing stops a project with approval off.
func (p *Project) Gate() []Blocker {
	if p.Meta.Approval == ApprovalOff {
		return nil
	}
	var out []Blocker
	for _, e := range Catalog {
		if !e.Gate {
			continue
		}
		d := p.Doc(e.Name)
		switch {
		case d == nil:
			out = append(out, Blocker{e.Name, "has no document in this project"})
		case d.Meta.Status != StatusApproved:
			out = append(out, Blocker{d.Title, "is " + strings.ToLower(orDraft(d.Meta.Status)) + ", not approved"})
		case d.Changed:
			out = append(out, Blocker{d.Title, "changed since it was approved"})
		}
	}
	return out
}

func orDraft(s string) string {
	if s == "" {
		return StatusDraft
	}
	return s
}

// Summary is a project at a glance.
type Summary struct {
	Docs            int
	ByStatus        map[string]int
	Done, Todos     int
	OpenQuestions   []string
	Stale, Unfilled int
	Blockers        []Blocker
}

// OpenQuestionsHeading names the sections whose unticked to-dos are the
// project's open questions.
const OpenQuestionsHeading = "Open questions"

// Summarize is p at a glance.
func (p *Project) Summarize() Summary {
	s := Summary{Docs: len(p.Docs), ByStatus: map[string]int{}, Blockers: p.Gate()}
	collect := func(body string) {
		lines := noteblocks.Lines(body)
		for _, sec := range noteblocks.Outline(body) {
			if !strings.EqualFold(sec.Title, OpenQuestionsHeading) {
				continue
			}
			for _, l := range lines[sec.Line+1 : sec.End] {
				if done, text, ok := noteblocks.Todo(l); ok && !done {
					s.OpenQuestions = append(s.OpenQuestions, text)
				}
			}
		}
	}
	collect(p.Note.Body)
	for _, d := range p.Docs {
		s.ByStatus[orDraft(d.Meta.Status)]++
		s.Done += d.Done
		s.Todos += d.Todos
		if len(d.Stale) > 0 {
			s.Stale++
		}
		s.Unfilled += len(d.Unfilled)
		collect(d.Body)
	}
	return s
}
