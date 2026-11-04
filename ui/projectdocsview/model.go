package projectdocsview

import (
	"fmt"
	"hash/fnv"
	"io"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// What the page shows, worked out apart from drawing it: a project is derived
// off the main thread, as hashing sixty documents would show in a frame.

// model is a project as its summary shows it.
type model struct {
	project *projectdocs.Project
	summary projectdocs.Summary
	counts  []statusCount
	rows    []docRow
	gate    gateText
	missing []areaEntries
	notes   map[string]bool // the project, its areas and documents, by ID
}

// statusCount is how many documents are in a status.
type statusCount struct {
	Status string
	N      int
}

// docRow is a line of the documents' list: a document, or an area over them.
type docRow struct {
	ID, Title, Icon, Status string
	Area                    bool
	Done, Todos             int
	Stale, Changed          bool
}

// gateText says whether the project may be built.
type gateText struct {
	Ready bool
	Title string
	Lines []string // what blocks it, one line a document
}

// areaEntries are the documents of an area of the catalog.
type areaEntries struct {
	Area    string
	Entries []projectdocs.Entry
}

// modelOf is p as its summary shows it.
func modelOf(p *projectdocs.Project) *model {
	s := p.Summarize()
	m := &model{project: p, summary: s, gate: gateOf(p.Meta.Approval, s.Blockers), missing: missingOf(p),
		notes: map[string]bool{p.Note.ID: true}}
	for _, st := range projectdocs.Statuses {
		m.counts = append(m.counts, statusCount{st, s.ByStatus[st]})
	}
	areas := map[string]proto.NoteInfo{}
	for _, a := range p.Areas {
		areas[a.ID] = a
		m.notes[a.ID] = true
	}
	shown := ""
	for _, d := range p.Docs {
		m.notes[d.ID] = true
		if a, ok := areas[d.Parent]; ok && a.ID != shown {
			m.rows = append(m.rows, docRow{ID: a.ID, Title: titleOf(a), Icon: a.Icon, Area: true})
			shown = a.ID
		}
		m.rows = append(m.rows, docRow{ID: d.ID, Title: titleOf(d.NoteInfo), Icon: d.Icon, Status: statusOf(d.Meta),
			Done: d.Done, Todos: d.Todos, Stale: len(d.Stale) > 0, Changed: d.Changed})
	}
	return m
}

// has says the note is part of the project.
func (m *model) has(id string) bool { return m != nil && m.notes[id] }

// doc is the document with this ID, nil for none.
func (m *model) doc(id string) *projectdocs.Doc {
	if m == nil {
		return nil
	}
	for i := range m.project.Docs {
		if m.project.Docs[i].ID == id {
			return &m.project.Docs[i]
		}
	}
	return nil
}

// statusOf is a document's status as it reads: one with none is a draft.
func statusOf(m projectdocs.Meta) string {
	if m.Status == "" {
		return projectdocs.StatusDraft
	}
	return m.Status
}

// gateOf says what approval and the blockers come to.
func gateOf(approval string, blockers []projectdocs.Blocker) gateText {
	if approval == projectdocs.ApprovalOff {
		return gateText{Ready: true, Title: "Approval is off: agents may build without it"}
	}
	if len(blockers) == 0 {
		return gateText{Ready: true, Title: "Ready to build: the PRD and Technical Spec are approved"}
	}
	g := gateText{Title: "Blocked from building"}
	for _, b := range blockers {
		g.Lines = append(g.Lines, b.Doc+" "+b.Why)
	}
	return g
}

// missingOf is the catalog's documents the project hasn't, by area in the
// catalog's order.
func missingOf(p *projectdocs.Project) []areaEntries {
	var out []areaEntries
	for _, area := range projectdocs.Areas {
		a := areaEntries{Area: area}
		for _, e := range projectdocs.Catalog {
			if e.Area != area || p.Doc(e.Name) != nil {
				continue
			}
			a.Entries = append(a.Entries, e)
		}
		if len(a.Entries) > 0 {
			out = append(out, a)
		}
	}
	return out
}

// titleOf is a note's title as it reads.
func titleOf(n proto.NoteInfo) string {
	if t := strings.TrimSpace(n.Title); t != "" {
		return t
	}
	return "Untitled"
}

// plural says n of a thing, in words.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// docView is what a document's strip shows of the body on the page, which
// runs ahead of the project as last derived.
type docView struct {
	meta        projectdocs.Meta
	changed     bool
	links       []projectdocs.Link
	done, todos int
}

func docViewOf(body string) docView {
	m, _ := projectdocs.Parse(body)
	done, todos := noteblocks.Progress(body)
	return docView{meta: m, changed: projectdocs.ChangedSinceApproval(body), links: projectdocs.Links(body), done: done, todos: todos}
}

// actions are what a document's strip offers in its status.
type actions struct {
	Approve bool
	// ApproveLabel is Approve's words: approving again says it's the new
	// content that is approved.
	ApproveLabel string
	Reopen       bool
}

func (d docView) actions() actions {
	switch {
	case statusOf(d.meta) != projectdocs.StatusApproved:
		return actions{Approve: true, ApproveLabel: "Approve"}
	case d.changed:
		return actions{Approve: true, ApproveLabel: "Approve again", Reopen: true}
	}
	return actions{Reopen: true}
}

// statusChoices are the statuses the strip's menu sets: only approving, with
// its confirmation, makes a document approved.
func statusChoices() []string {
	return []string{projectdocs.StatusDraft, projectdocs.StatusReview, projectdocs.StatusOutdated}
}

// linkRow is a Based on link as the strip shows it.
type linkRow struct {
	Link  projectdocs.Link
	Label string
	Why   string // "" while its source is as it was
	Gone  bool
}

// linkRows are what the body says it's based on, with what the project as
// derived says of each. A link brought up to date since no longer matches the
// stale one derived, which is why they're matched by what's written.
func linkRows(links []projectdocs.Link, stale []projectdocs.Stale) []linkRow {
	out := make([]linkRow, 0, len(links))
	for _, l := range links {
		r := linkRow{Link: l, Label: linkLabel(l)}
		for _, s := range stale {
			if s.Link != l {
				continue
			}
			r.Gone = s.Reason == "gone"
			if r.Gone {
				r.Why = r.Label + " is no longer in this project"
			} else {
				r.Why = r.Label + " changed since this was based on it"
			}
		}
		out = append(out, r)
	}
	return out
}

// linkLabel names a link's source and section.
func linkLabel(l projectdocs.Link) string {
	if l.Section != "" {
		return l.Doc + " › " + l.Section
	}
	return l.Doc
}

// reviewed is body confirming link l against the source as it is now.
func reviewed(body string, l projectdocs.Link, source string) (string, error) {
	sum := projectdocs.ContentHash(source)
	if l.Section != "" {
		var err error
		if sum, err = projectdocs.SectionHash(source, l.Section); err != nil {
			return "", err
		}
	}
	l.Hash = sum
	return projectdocs.AddLink(body, l)
}

// crumbsOf is the notes from the top down to id, starting at the project when
// it's known.
func crumbsOf(list []proto.NoteInfo, id, project string) []proto.NoteInfo {
	byID := make(map[string]proto.NoteInfo, len(list))
	for _, n := range list {
		byID[n.ID] = n
	}
	var path []proto.NoteInfo
	for n, ok := byID[id]; ok && len(path) < 16; n, ok = byID[n.Parent] {
		path = append(path, n)
		if n.ID == project {
			break
		}
	}
	slices.Reverse(path)
	return path
}

// listSum changes when the list does, cheaply enough to ask every frame.
func listSum(list []proto.NoteInfo) uint64 {
	h := fnv.New64a()
	for i := range list {
		sumNote(h, &list[i])
	}
	return h.Sum64()
}

func sumNote(h io.Writer, n *proto.NoteInfo) {
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s\x00%d\x00%d\n", n.ID, n.Parent, n.Title, n.Updated.UnixNano(), n.Order)
}

// treeSum changes when a note of id's whole tree does, so that a note
// changing elsewhere doesn't read the project again.
func treeSum(list []proto.NoteInfo, id string) uint64 {
	byID := make(map[string]proto.NoteInfo, len(list))
	children := make(map[string][]int, len(list))
	for i, n := range list {
		byID[n.ID] = n
		children[n.Parent] = append(children[n.Parent], i)
	}
	top := id
	for n, ok := byID[id]; ok && n.Parent != ""; n, ok = byID[n.Parent] {
		top = n.Parent
	}
	h := fnv.New64a()
	var walk func(string)
	walk = func(p string) {
		for _, i := range children[p] {
			sumNote(h, &list[i])
			walk(list[i].ID)
		}
	}
	if n, ok := byID[top]; ok {
		sumNote(h, &n)
	}
	walk(top)
	return h.Sum64()
}

// missingProjects are the documents that lack a project their project note
// is attached to.
func missingProjects(list []proto.NoteInfo, m *model, want []proto.NoteProject) []string {
	if m == nil || len(want) == 0 {
		return nil
	}
	byID := make(map[string]proto.NoteInfo, len(list))
	for _, n := range list {
		byID[n.ID] = n
	}
	var out []string
	for _, d := range m.project.Docs {
		n, ok := byID[d.ID]
		if !ok {
			continue
		}
		for _, p := range want {
			if !slices.ContainsFunc(n.Projects, func(x proto.NoteProject) bool { return x.Ref == p.Ref }) {
				out = append(out, d.ID)
				break
			}
		}
	}
	return out
}

// repoFiles are the files exporting the project writes, for showing first:
// the project note as README.md, its documents under folders of their areas.
func repoFiles(m *model) []string {
	if m == nil {
		return nil
	}
	return projectdocs.RepoPaths(m.project)
}
