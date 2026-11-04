package projectdocsview

import (
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

func docLine(status string, hash string) string {
	return projectdocs.Meta{Kind: "PRD", Status: status, Version: 1, Hash: hash}.Line() + "\n\n## Features\n\nText\n"
}

// What the gate says: off lets it through, approved is ready, and each document
// blocking it is named with why.
func TestGateText(t *testing.T) {
	if g := gateOf(projectdocs.ApprovalOff, nil); !g.Ready || !strings.Contains(g.Title, "off") {
		t.Errorf("off: %+v", g)
	}
	if g := gateOf(projectdocs.ApprovalRequired, nil); !g.Ready || !strings.HasPrefix(g.Title, "Ready") {
		t.Errorf("ready: %+v", g)
	}
	g := gateOf(projectdocs.ApprovalRequired, []projectdocs.Blocker{{Doc: "PRD", Why: "is draft, not approved"}, {Doc: "Technical Spec", Why: "changed since it was approved"}})
	if g.Ready || g.Lines[0] != "PRD is draft, not approved" || g.Lines[1] != "Technical Spec changed since it was approved" {
		t.Errorf("blocked: %+v", g)
	}
}

// What the strip offers per status: only an approved document reopens, and only
// one changed since is both approved again and reopened.
func TestActionsPerStatus(t *testing.T) {
	for _, status := range []string{projectdocs.StatusDraft, projectdocs.StatusReview, projectdocs.StatusOutdated} {
		a := docViewOf(docLine(status, "")).actions()
		if !a.Approve || a.ApproveLabel != "Approve" || a.Reopen {
			t.Errorf("%s: %+v", status, a)
		}
	}
	approved, err := projectdocs.Approve(docLine(projectdocs.StatusDraft, ""))
	if err != nil {
		t.Fatal(err)
	}
	if a := docViewOf(approved).actions(); a.Approve || !a.Reopen {
		t.Errorf("approved: %+v", a)
	}
	d := docViewOf(approved + "\nMore text\n")
	if !d.changed {
		t.Fatal("changed since approved doesn't say so")
	}
	if a := d.actions(); !a.Approve || a.ApproveLabel != "Approve again" || !a.Reopen {
		t.Errorf("changed: %+v", a)
	}
	if slices.Contains(statusChoices(), projectdocs.StatusApproved) {
		t.Error("the status menu approves, without Approve's confirmation")
	}
}

// Marking reviewed takes the source's hash as it is now, whole or its section,
// and the link then no longer reads as stale.
func TestMarkReviewed(t *testing.T) {
	source := "Project Docs: PRD · {{Draft}} · v1\n\n## Features\n\nNew text\n\n## Other\n\nElse\n"
	body := "Project Docs: Technical Spec · {{Draft}} · v1\n\nBased on: [[PRD#Features]] @00000000\n\n## Stack\n\nGo\n"
	links := projectdocs.Links(body)
	if len(links) != 1 {
		t.Fatalf("links: %+v", links)
	}
	got, err := reviewed(body, links[0], source)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := projectdocs.SectionHash(source, "Features")
	after := projectdocs.Links(got)
	if len(after) != 1 || after[0].Hash != want {
		t.Fatalf("after: %+v, want hash %s:\n%s", after, want, got)
	}
	rows := linkRows(after, []projectdocs.Stale{{Link: links[0], Reason: "changed"}})
	if rows[0].Why != "" {
		t.Errorf("a link brought up to date still reads stale: %+v", rows[0])
	}
	if _, err := reviewed(body, projectdocs.Link{Doc: "PRD", Section: "Gone", Hash: "00000000"}, source); err == nil {
		t.Error("a section that's gone was marked reviewed")
	}
}

// Stale links name their source and section, and a source gone isn't offered as
// reviewed.
func TestLinkRowsNameTheSource(t *testing.T) {
	l := projectdocs.Link{Doc: "PRD", Section: "Features", Hash: "00000000"}
	g := projectdocs.Link{Doc: "Old", Hash: "11111111"}
	rows := linkRows([]projectdocs.Link{l, g}, []projectdocs.Stale{{Link: l, Reason: "changed"}, {Link: g, Reason: "gone"}})
	if !strings.Contains(rows[0].Why, "PRD › Features") || rows[0].Gone {
		t.Errorf("changed: %+v", rows[0])
	}
	if !rows[1].Gone || !strings.Contains(rows[1].Why, "Old") {
		t.Errorf("gone: %+v", rows[1])
	}
}

func TestCrumbsStartAtTheProject(t *testing.T) {
	list := []proto.NoteInfo{{ID: "top"}, {ID: "p", Parent: "top"}, {ID: "a", Parent: "p"}, {ID: "d", Parent: "a"}}
	var ids []string
	for _, n := range crumbsOf(list, "d", "p") {
		ids = append(ids, n.ID)
	}
	if strings.Join(ids, ">") != "p>a>d" {
		t.Errorf("crumbs: %v", ids)
	}
}

// A note changing elsewhere changes the list but not the project's tree.
func TestTreeSumOnlyFollowsTheProject(t *testing.T) {
	list := []proto.NoteInfo{{ID: "p"}, {ID: "d", Parent: "p"}, {ID: "other"}}
	before := treeSum(list, "d")
	list[2].Title = "changed"
	if treeSum(list, "d") != before {
		t.Error("another note changed the project's sum")
	}
	list[1].Title = "changed"
	if treeSum(list, "d") == before {
		t.Error("its document changing didn't")
	}
}

// The model lists documents under their areas, the catalog's others to add by
// area, and the files an export writes.
func TestModelOfAFullPack(t *testing.T) {
	s := newStore()
	root := s.project(t, "Shop", projectdocs.SizeFull)
	p, err := projectdocs.Load(s.list(), root.ID, func(id string) (proto.Note, error) { return s.notes[id], nil })
	if err != nil {
		t.Fatal(err)
	}
	m := modelOf(p)
	if m.gate.Ready || len(m.gate.Lines) != 2 {
		t.Errorf("a new project's gate: %+v", m.gate)
	}
	if !m.rows[0].Area || m.rows[0].Title != projectdocs.AreaProduct || m.rows[1].Area {
		t.Errorf("rows: %+v", m.rows[:2])
	}
	if len(m.missing) != 0 {
		t.Errorf("a full pack lacks %+v", m.missing)
	}
	files := repoFiles(m)
	if files[0] != "README.md" || !slices.Contains(files, "business-and-product/prd.md") {
		t.Errorf("files: %v", files[:3])
	}
	core := newStore()
	croot := core.project(t, "Small", projectdocs.SizeCore)
	cp, err := projectdocs.Load(core.list(), croot.ID, func(id string) (proto.Note, error) { return core.notes[id], nil })
	if err != nil {
		t.Fatal(err)
	}
	cm := modelOf(cp)
	if len(cm.missing) == 0 || cm.missing[0].Area != projectdocs.AreaProduct {
		t.Errorf("a core pack's missing: %+v", cm.missing)
	}
	for _, a := range cm.missing {
		for _, e := range a.Entries {
			if e.Core {
				t.Errorf("%s is in the core pack and listed to add", e.Name)
			}
		}
	}
}

func TestDocsMissingProjects(t *testing.T) {
	s := newStore()
	root := s.project(t, "Shop", projectdocs.SizeCore)
	p, err := projectdocs.Load(s.list(), root.ID, func(id string) (proto.Note, error) { return s.notes[id], nil })
	if err != nil {
		t.Fatal(err)
	}
	want := []proto.NoteProject{{Ref: "git:x/y"}}
	m := modelOf(p)
	if n := len(missingProjects(s.list(), m, want)); n != len(m.project.Docs) {
		t.Errorf("all lack it: %d", n)
	}
	first := m.project.Docs[0].ID
	note := s.notes[first]
	note.Projects = want
	s.notes[first] = note
	if n := len(missingProjects(s.list(), m, want)); n != len(m.project.Docs)-1 {
		t.Errorf("one has it: %d", n)
	}
	if missingProjects(s.list(), m, nil) != nil {
		t.Error("nothing to apply when the project isn't attached")
	}
}
