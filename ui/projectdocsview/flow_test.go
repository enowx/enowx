package projectdocsview

import (
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// Making a project: the title typed, the pack picked, a template of the user's
// ticked; it opens when it's done.
func TestMakeAProject(t *testing.T) {
	s := newStore()
	tn, _ := s.put(proto.Note{NoteInfo: proto.NoteInfo{Title: templatesNote}})
	s.put(proto.Note{NoteInfo: proto.NoteInfo{Title: "Our checklist", Parent: tn.ID}, Body: "- [ ] ship\n"})
	s.view.OpenNew()
	tt := s.tester(1000, 900)
	s.frames(tt, 8)
	if f := s.view.form; !f.approval || f.size != projectdocs.SizeCore {
		t.Errorf("defaults: %+v", f)
	}
	click(t, tt, "Our checklist")
	s.view.form.title = "Shop"
	s.frames(tt, 2)
	click(t, tt, "Create")
	s.settle(t, tt, "the project", func() bool { return s.open != "" })
	body := s.body(s.open)
	m, ok := projectdocs.Parse(body)
	if !ok || !m.IsProject() || m.Approval != projectdocs.ApprovalRequired || m.Size != projectdocs.SizeCore {
		t.Fatalf("project: %q", body)
	}
	if s.view.form != nil {
		t.Error("the form stays open")
	}
	own := s.body(s.docNamedUnder(t, s.open, "Our checklist"))
	if om, ok := projectdocs.Parse(own); !ok || !om.IsDocument() || !strings.Contains(own, "- [ ] ship") {
		t.Errorf("the template's document: %q", own)
	}
}

// projectdocs.New itself, once every template of the pack is on the branch.
func TestMakeAProjectFromTheTemplates(t *testing.T) {
	for _, e := range projectdocs.Pack(projectdocs.SizeCore) {
		if _, err := projectdocs.Template(e.Key); err != nil {
			t.Skipf("template %s isn't on this branch yet", e.Key)
		}
	}
	s := newStore()
	s.view.makeProject = nil
	s.view.OpenNew()
	tt := s.tester(1000, 900)
	s.frames(tt, 8)
	s.view.form.title = "Shop"
	s.frames(tt, 2)
	click(t, tt, "Create")
	s.settle(t, tt, "the project", func() bool { return s.open != "" })
	if want := packNotes(projectdocs.SizeCore); len(s.list()) != want {
		t.Errorf("%d notes, want %d", len(s.list()), want)
	}
}

func (s *store) docNamedUnder(t *testing.T, parent, title string) string {
	t.Helper()
	for _, n := range s.list() {
		if n.Parent == parent && n.Title == title {
			return n.ID
		}
	}
	t.Fatalf("no %q under %s", title, parent)
	return ""
}

// When a save fails, what was made goes again, and the form says why.
func TestAFailedProjectTakesItselfBack(t *testing.T) {
	s := newStore()
	s.failAfter = 3
	s.view.OpenNew()
	tt := s.tester(1000, 900)
	s.frames(tt, 8)
	s.view.form.title = "Shop"
	s.frames(tt, 2)
	click(t, tt, "Create")
	s.settle(t, tt, "the failure", func() bool { return s.view.form != nil && s.view.form.err != "" })
	if !strings.Contains(s.view.form.err, "the disk is full") || s.view.form.busy {
		t.Errorf("form: %+v", s.view.form)
	}
	if n := len(s.list()); n != 0 {
		t.Errorf("%d notes left behind: %+v", n, s.list())
	}
	if s.open != "" {
		t.Error("opened a project that failed")
	}
}

// Approving asks first, saying what it means, and waits for what's typed to be
// saved: then the body as edited is what's approved.
func TestApproveWaitsForTheSave(t *testing.T) {
	s := newStore()
	s.project(t, "Shop", projectdocs.SizeCore)
	prd := s.docNamed(t, "PRD")
	s.open = prd
	tt := s.tester(1000, 900)
	s.settle(t, tt, "the strip", func() bool { return tt.HasText(projectdocs.StatusDraft) })
	s.unsaved = true
	click(t, tt, "Approve")
	s.frames(tt, 8)
	if !tt.HasText("Agents can't change it directly") {
		t.Fatalf("no confirmation: %q", tt.Texts())
	}
	if len(s.rewrites) != 0 {
		t.Fatal("approved before the confirmation")
	}
	click(t, tt, "Yes, approve")
	s.frames(tt, 4)
	if len(s.rewrites) != 0 {
		t.Fatal("wrote over edits not saved")
	}
	if s.flushes == 0 {
		t.Error("didn't save the edits first")
	}
	s.unsaved = false
	s.setBody(prd, func(b string) string { return b + "\nTyped just now\n" })
	s.frames(tt, 3)
	if len(s.rewrites) != 1 {
		t.Fatalf("rewrites: %d", len(s.rewrites))
	}
	body := s.body(prd)
	m, _ := projectdocs.Parse(body)
	if m.Status != projectdocs.StatusApproved || m.Hash != projectdocs.ContentHash(body) || !strings.Contains(body, "Typed just now") {
		t.Errorf("approved: %+v\n%s", m, body)
	}
	if !tt.HasText("Reopen") || tt.HasText("Yes, reopen") {
		t.Errorf("after approving: %q", tt.Texts())
	}
}

// Leaving approved asks with what's lost; a draft's status changes at once.
func TestStatusMenuAndReopen(t *testing.T) {
	s := newStore()
	s.project(t, "Shop", projectdocs.SizeCore)
	prd := s.docNamed(t, "PRD")
	s.open = prd
	tt := s.tester(1000, 900)
	s.settle(t, tt, "the strip", func() bool { return tt.HasText(projectdocs.StatusDraft) })
	click(t, tt, "Status: Draft")
	s.frames(tt, 6)
	if tt.HasText(projectdocs.StatusApproved) {
		t.Error("the status menu offers approved")
	}
	click(t, tt, projectdocs.StatusReview)
	s.frames(tt, 4)
	if m, _ := projectdocs.Parse(s.body(prd)); m.Status != projectdocs.StatusReview {
		t.Fatalf("status: %+v", m)
	}

	s.setBody(prd, func(b string) string {
		out, err := projectdocs.Approve(b)
		if err != nil {
			t.Fatal(err)
		}
		return out
	})
	s.frames(tt, 3)
	click(t, tt, "Reopen")
	s.frames(tt, 8)
	if !tt.HasText("loses its approval") {
		t.Fatalf("no warning: %q", tt.Texts())
	}
	click(t, tt, "Yes, reopen")
	s.frames(tt, 4)
	m, _ := projectdocs.Parse(s.body(prd))
	if m.Status != projectdocs.StatusDraft {
		t.Errorf("reopened: %+v", m)
	}
}

// A stale link is reviewed against the source as the host has it now.
func TestMarkReviewedThroughTheStrip(t *testing.T) {
	s := newStore()
	s.project(t, "Shop", projectdocs.SizeCore)
	spec, prd := s.docNamed(t, "Technical Spec"), s.docNamed(t, "PRD")
	s.setBody(spec, func(b string) string {
		first, rest, _ := strings.Cut(b, "\n")
		return first + "\n\nBased on: [[PRD]] @00000000\n" + rest
	})
	s.open = spec
	tt := s.tester(1000, 900)
	s.settle(t, tt, "the warning", func() bool { return tt.HasText("PRD changed since this was based on it") })
	click(t, tt, "Mark reviewed")
	s.frames(tt, 4)
	links := projectdocs.Links(s.body(spec))
	if len(links) != 1 || links[0].Hash != projectdocs.ContentHash(s.body(prd)) {
		t.Fatalf("links: %+v", links)
	}
	s.now = s.now.Add(10 * time.Second)
	s.settle(t, tt, "the warning gone", func() bool { return !tt.HasText("changed since this was based on it") })
	click(t, tt, "Open source")
	if s.open != prd {
		t.Errorf("open source: %q", s.open)
	}
}

// A line in the strip says a proposal waits for the document.
func TestStripSaysAProposalWaits(t *testing.T) {
	s := newStore()
	s.project(t, "Shop", projectdocs.SizeCore)
	prd := s.docNamed(t, "PRD")
	s.open = prd
	s.view.now = func() time.Time { return s.now }
	page := func() Page { p := s.page(); p.Proposals = 1; return p }
	if p := page(); p.Proposals != 1 {
		t.Fatal(p)
	}
	tt := s.proposalTester(page)
	s.settle(t, tt, "the line", func() bool { return tt.HasText("1 change from an agent waiting") })
}

// Sixty documents are read once; after that only what changed, no more often
// than the throttle, and frames with no change read nothing.
func TestDerivingIsThrottledAndReadsWhatChanged(t *testing.T) {
	s := newStore()
	root := s.project(t, "Big", projectdocs.SizeCore)
	for i := range 60 {
		s.put(proto.Note{NoteInfo: proto.NoteInfo{Title: "Doc " + string(rune('A'+i%26)) + string(rune('a'+i/26)), Parent: root.ID},
			Body: projectdocs.Meta{Kind: "Extra", Status: projectdocs.StatusDraft, Version: 1}.Line() + "\n"})
	}
	s.open = root.ID
	tt := s.tester(1000, 2400)
	s.settle(t, tt, "the summary", func() bool { return s.view.model != nil })
	first := s.readCount()
	if first < 60 {
		t.Fatalf("reads: %d", first)
	}
	s.frames(tt, 30)
	if got := s.readCount(); got != first {
		t.Fatalf("frames with no change read %d notes", got-first)
	}

	doc := s.docNamed(t, "PRD")
	s.setBody(doc, func(b string) string { return b + "\nOne\n" })
	s.frames(tt, 10) // within the throttle: the clock stands still
	if got := s.readCount(); got != first {
		t.Fatalf("read within the throttle: %d", got-first)
	}
	s.now = s.now.Add(deriveEvery)
	s.settle(t, tt, "the change", func() bool { return s.readCount() > first && s.view.loading == nil })
	if got := s.readCount() - first; got != 1 {
		t.Errorf("one note changed, %d read", got)
	}
	// A note elsewhere changing reads nothing.
	second := s.readCount()
	s.put(proto.Note{NoteInfo: proto.NoteInfo{Title: "Elsewhere"}})
	s.now = s.now.Add(deriveEvery)
	s.frames(tt, 5)
	if got := s.readCount(); got != second {
		t.Errorf("a note elsewhere read %d notes", got-second)
	}
}

// Attaching is the project note's; Apply copies its projects to the documents.
func TestApplyProjectsToDocuments(t *testing.T) {
	s := newStore()
	root := s.project(t, "Shop", projectdocs.SizeCore)
	s.open = root.ID
	tt := s.tester(1000, 1600)
	s.settle(t, tt, "the summary", func() bool { return s.view.model != nil })
	if tt.HasText("Apply to") {
		t.Error("offered with no project attached")
	}
	n := s.notes[root.ID]
	n.Projects = []proto.NoteProject{{Ref: "git:x/y", Name: "y"}}
	s.notes[root.ID] = n
	s.frames(tt, 3)
	docs := len(s.view.model.project.Docs)
	label := "Apply to " + plural(docs, "document", "documents")
	click(t, tt, label)
	s.frames(tt, 3)
	for _, d := range s.view.model.project.Docs {
		if len(s.notes[d.ID].Projects) != 1 {
			t.Errorf("%s has %v", d.Title, s.notes[d.ID].Projects)
		}
	}
	if !tt.HasText("Applied to") {
		t.Errorf("after: %q", tt.Texts())
	}
}

// Turning approval off says what changes, and a full pack adds under its area.
func TestApprovalAndAddDocument(t *testing.T) {
	s := newStore()
	root := s.project(t, "Shop", projectdocs.SizeCore)
	s.open = root.ID
	tt := s.tester(1000, 1600)
	s.settle(t, tt, "the summary", func() bool { return s.view.model != nil })
	click(t, tt, "Require approval")
	s.frames(tt, 8)
	if !tt.HasText("Agents may start building") {
		t.Fatalf("no confirmation: %q", tt.Texts())
	}
	if m, _ := projectdocs.Parse(s.body(root.ID)); m.Approval != projectdocs.ApprovalRequired {
		t.Fatal("changed before it was confirmed")
	}
	click(t, tt, "Turn off")
	s.frames(tt, 4)
	if m, _ := projectdocs.Parse(s.body(root.ID)); m.Approval != projectdocs.ApprovalOff {
		t.Fatalf("approval: %+v", m)
	}

	click(t, tt, "Add document")
	s.frames(tt, 6)
	if tt.HasText("Technical Spec") == false || !tt.HasText("Glossary") {
		t.Fatalf("the menu: %q", tt.Texts())
	}
	click(t, tt, "Glossary")
	s.frames(tt, 3)
	g := s.body(s.docNamedUnder(t, root.ID, "Glossary"))
	if m, ok := projectdocs.Parse(g); !ok || m.Kind != "Glossary" {
		t.Errorf("added: %q", g)
	}

	full := newStore()
	froot := full.project(t, "Big", projectdocs.SizeFull)
	glossary := full.docNamed(t, "Glossary")
	area := full.notes[glossary].Parent
	p, err := projectdocs.Load(full.list(), froot.ID, func(id string) (proto.Note, error) { return full.notes[id], nil })
	if err != nil {
		t.Fatal(err)
	}
	var e projectdocs.Entry
	for _, x := range projectdocs.Catalog {
		if x.Key == "glossary" {
			e = x
		}
	}
	full.view.addEntry(full.options(), modelOf(p), e)
	added := 0
	for _, n := range full.list() {
		if n.Title == "Glossary" && n.Parent == area {
			added++
		}
	}
	if added != 2 {
		t.Errorf("under its area: %d", added)
	}
}

// Exporting shows the files first, confirms, and overwrites only when asked to
// after the host refused.
func TestExportShowsFilesAndConfirms(t *testing.T) {
	s := newStore()
	root := s.project(t, "Shop", projectdocs.SizeCore)
	n := s.notes[root.ID]
	n.Projects = []proto.NoteProject{{Ref: "git:x/y", Name: "y", Path: "/work/y"}}
	s.notes[root.ID] = n
	s.open = root.ID
	type call struct {
		folder string
		force  bool
	}
	var calls []call
	refuse := true
	tt := s.exportTester(func(_, folder string, force bool, done func(string, error)) {
		calls = append(calls, call{folder, force})
		if refuse && !force {
			done("", errRefused)
			return
		}
		done(folder, nil)
	})
	s.settle(t, tt, "the summary", func() bool { return s.view.model != nil })
	click(t, tt, "Export to repo…")
	s.frames(tt, 8)
	for _, want := range []string{"README.md", "prd.md", "y · /work/y"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	if s.view.export.path != "/work/y" {
		t.Errorf("folder: %q", s.view.export.path)
	}
	click(t, tt, "Export…")
	s.frames(tt, 8)
	if len(calls) != 0 {
		t.Fatal("wrote before confirming")
	}
	click(t, tt, "Export")
	s.frames(tt, 8)
	if len(calls) != 1 || calls[0].force {
		t.Fatalf("calls: %+v", calls)
	}
	if !tt.HasText("Overwrite…") {
		t.Fatalf("no way to overwrite: %q", tt.Texts())
	}
	click(t, tt, "Overwrite…")
	s.frames(tt, 8)
	if !tt.HasText("what they hold is lost") || len(calls) != 1 {
		t.Fatalf("no warning: %q, %+v", tt.Texts(), calls)
	}
	click(t, tt, "Overwrite")
	s.frames(tt, 8)
	if len(calls) != 2 || !calls[1].force {
		t.Fatalf("calls: %+v", calls)
	}
	if !tt.HasText("Written to /work/y") {
		t.Errorf("after: %q", tt.Texts())
	}
}
