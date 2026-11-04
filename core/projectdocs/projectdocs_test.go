package projectdocs

import (
	"strings"
	"testing"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/core/notes"
	"github.com/enowx/enowx/proto"
)

func TestMetaRoundTrip(t *testing.T) {
	for _, m := range []Meta{
		{Kind: KindProject, Size: SizeFull, Approval: ApprovalOff},
		{Kind: KindProject, Size: SizeCore, Approval: ApprovalRequired},
		{Kind: "PRD", Status: StatusApproved, Version: 3, Hash: "9f8e7d6c"},
		{Kind: "Technical Spec", Status: StatusReview, Version: 1},
		{Kind: KindArea},
	} {
		got, ok := Parse(m.Line() + "\n\nbody")
		if !ok || got != m {
			t.Fatalf("Parse(%q) = %+v, %v; want %+v", m.Line(), got, ok, m)
		}
	}
	if _, ok := Parse("An ordinary note\n\nProject Docs: PRD"); ok {
		t.Fatal("an ordinary note read as a document")
	}
}

func TestTickingIsNotChangingContent(t *testing.T) {
	body, err := Approve(Meta{Kind: "PRD", Version: 1}.Line() + "\n\n## AC\n- [ ] logs in\n")
	if err != nil {
		t.Fatal(err)
	}
	ticked := strings.Replace(body, "- [ ] logs in", "- [x] logs in", 1)
	if ChangedSinceApproval(ticked) {
		t.Fatal("ticking a criterion counted as changing the document")
	}
	if !ChangedSinceApproval(strings.Replace(body, "logs in", "signs up", 1)) {
		t.Fatal("changed content wasn't noticed")
	}
	if a, _, _ := AgentWrite(body, ticked); a != WriteDirect {
		t.Fatal("an agent's tick must go through directly")
	}
}

func TestAgentWriteProposesApprovedAndNeverApproves(t *testing.T) {
	draft := Meta{Kind: "PRD", Status: StatusDraft, Version: 1}.Line() + "\n\ntext\n"
	if a, _, _ := AgentWrite(draft, draft+"more\n"); a != WriteDirect {
		t.Fatal("a draft should be written directly")
	}
	self, _ := Approve(draft)
	if a, _, why := AgentWrite(draft, self); a != WritePropose || why == "" {
		t.Fatal("an agent approved its own document")
	}
	a, body, _ := AgentWrite(self, self+"more\n")
	if a != WritePropose {
		t.Fatal("a change to an approved document must be a proposal")
	}
	m, _ := Parse(body)
	if m.Status != StatusApproved || m.Version != 2 || ChangedSinceApproval(body) {
		t.Fatalf("accepted, it should stay approved at the next version: %+v", m)
	}
	project := ProjectBody(SizeCore, ApprovalRequired)
	off, _ := SetApproval(project, ApprovalOff)
	if a, _, _ := AgentWrite(project, off); a != WritePropose {
		t.Fatal("an agent turned approval off")
	}
	if AgentMayRemove(self) == nil || AgentMayRemove(project) == nil || AgentMayRemove(draft) != nil {
		t.Fatal("removal rules are wrong")
	}
}

func TestProjectThroughStore(t *testing.T) {
	st := notes.Open(t.TempDir())
	root, _, err := New(Options{Title: "Shop", Size: SizeCore}, st.Save)
	if err != nil {
		t.Fatal(err)
	}
	if root.ID == "" {
		t.Fatal("no project note")
	}
	// An ordinary note to every reader that doesn't know Project Docs.
	n, err := st.Get(root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "Shop" || !strings.HasPrefix(n.Body, Marker+KindProject) {
		t.Fatalf("project note: %+v", n)
	}
	if len(noteblocks.Outline(n.Body)) == 0 {
		t.Fatal("no sections")
	}
}

func TestStaleAndGate(t *testing.T) {
	st := notes.Open(t.TempDir())
	root, _ := st.Save(proto.Note{NoteInfo: proto.NoteInfo{Title: "P"}, Body: ProjectBody(SizeCore, ApprovalRequired)})
	prdBody := Meta{Kind: "PRD", Version: 1}.Line() + "\n\n## Features\n\nsearch\n\n## Other\n\nx\n"
	prd, _ := st.Save(proto.Note{NoteInfo: proto.NoteInfo{Title: "PRD", Parent: root.ID}, Body: prdBody})
	h, err := SectionHash(prdBody, "Features")
	if err != nil {
		t.Fatal(err)
	}
	specBody, err := AddLink(Meta{Kind: "Technical Spec", Version: 1}.Line()+"\n\n## Data\n\nrows\n", Link{Doc: "PRD", Section: "Features", Hash: h, In: "Data"})
	if err != nil {
		t.Fatal(err)
	}
	st.Save(proto.Note{NoteInfo: proto.NoteInfo{Title: "Technical Spec", Parent: root.ID}, Body: specBody})
	load := func() *Project {
		list, _ := st.List()
		p, err := Load(list, root.ID, st.Get)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := load()
	if d := p.Doc("Technical Spec"); d == nil || len(d.Stale) != 0 || d.Links[0].In != "data" {
		t.Fatalf("fresh link: %+v", d)
	}
	if len(p.Gate()) != 2 {
		t.Fatalf("gate should stop on both: %+v", p.Gate())
	}
	// Another section of the source changes nothing; this one does.
	n, _ := st.Get(prd.ID)
	n.Body = strings.Replace(n.Body, "\nx\n", "\ny\n", 1)
	st.Save(n)
	if d := load().Doc("Technical Spec"); len(d.Stale) != 0 {
		t.Fatalf("another section marked it stale: %+v", d.Stale)
	}
	n.Body = strings.Replace(n.Body, "search", "search and filter", 1)
	st.Save(n)
	if d := load().Doc("Technical Spec"); len(d.Stale) != 1 || d.Stale[0].Reason != "changed" {
		t.Fatalf("stale not marked: %+v", d.Stale)
	}
	// Approved both: the gate opens; approval off opens it too.
	for _, doc := range load().Docs {
		b, err := Approve(doc.Body)
		if err != nil {
			t.Fatal(err)
		}
		doc.Note.Body = b
		st.Save(doc.Note)
	}
	if g := load().Gate(); len(g) != 0 {
		t.Fatalf("gate after approval: %+v", g)
	}
}

func TestEveryTopicHasItsSection(t *testing.T) {
	for _, tp := range Topics {
		if Find(tp.Doc) == nil {
			t.Errorf("topic %q: no document %q", tp.Name, tp.Doc)
		}
	}
}
