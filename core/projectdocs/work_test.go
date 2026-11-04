package projectdocs

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// workProject is a project of the given documents, title to body, each body
// under its meta line.
func workProject(t *testing.T, docs [][2]string) *Project {
	t.Helper()
	list := []proto.NoteInfo{{ID: "root", Title: "Shop"}}
	notes := map[string]proto.Note{"root": {NoteInfo: list[0], Body: ProjectBody(SizeCore, ApprovalRequired)}}
	for i, d := range docs {
		info := proto.NoteInfo{ID: fmt.Sprintf("doc%d", i), Title: d[0], Parent: "root"}
		list = append(list, info)
		notes[info.ID] = proto.Note{NoteInfo: info, Body: Meta{Kind: d[0], Status: StatusApproved, Version: 1}.Line() + "\n\n" + d[1]}
	}
	p, err := Load(list, "root", func(id string) (proto.Note, error) { return notes[id], nil })
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlanTasksReadsTheWorkBreakdown(t *testing.T) {
	p := workProject(t, [][2]string{{"Work Breakdown and Backlog", "## Tasks\n\n### Stage 1\n\n" +
		"- [x] T-1: set up the repository (foundation, no dependencies)\n" +
		"- [ ] T-2: create the first migration (foundation, depends on T-1)\n" +
		"- [ ] T-3: add checkout (first feature, depends on T-1, T-2)\n" +
		"- [ ] T-4: write the docs (depends on T-9)\n" +
		"- [ ] T-5: tidy up\n" +
		"- [ ] not a task of the plan\n" +
		"```\n- [ ] T-6: in code, no task\n```\n"}})
	tasks := PlanTasks(p)
	if len(tasks) != 5 {
		t.Fatalf("tasks: %+v", tasks)
	}
	first, third := tasks[0], tasks[2]
	if first.ID != "T-1" || first.Text != "set up the repository" || first.Stage != "foundation" || len(first.Depends) != 0 || !first.Done {
		t.Fatalf("T-1: %+v", first)
	}
	if third.Stage != "first feature" || !slices.Equal(third.Depends, []string{"T-1", "T-2"}) || third.Done || third.Section != "stage-1" {
		t.Fatalf("T-3: %+v", third)
	}
	if tasks[3].Stage != "" || !slices.Equal(tasks[3].Depends, []string{"T-9"}) {
		t.Fatalf("T-4: %+v", tasks[3])
	}
	if tasks[4].Text != "tidy up" || tasks[4].Stage != "" {
		t.Fatalf("T-5: %+v", tasks[4])
	}

	ids := func(list []PlannedTask) (out []string) {
		for _, t := range list {
			out = append(out, t.ID)
		}
		return out
	}
	// T-3 waits on T-2, T-4 on a task the plan doesn't hold.
	if got := ids(Ready(tasks, nil)); !slices.Equal(got, []string{"T-2", "T-5"}) {
		t.Fatalf("ready: %v", got)
	}
	if got := ids(Ready(tasks, func(id string) bool { return id == "T-2" })); !slices.Equal(got, []string{"T-5"}) {
		t.Fatalf("ready, T-2 taken: %v", got)
	}
	if PlanTasks(workProject(t, nil)) != nil {
		t.Fatal("tasks without a Work Breakdown")
	}
}

func TestCriteriaOfADocumentElseTheStories(t *testing.T) {
	p := workProject(t, [][2]string{
		{"User Stories and Acceptance Criteria", "## Main flows\n\n- [ ] a step, no criterion\n\n## Acceptance criteria\n\n### Story: reset\n\n- [x] AC-1: a link is sent\n- [ ] AC-2: an old link says so\n\n### Story: sign in\n\n- [ ] AC-3: it signs in\n"},
		{"Test Plan", "## Levels\n\ntext\n\n### Acceptance Criteria\n\n- [ ] TP-1: the suite runs\n\n## Other\n\n- [ ] not one\n"},
		{"ADR", "## Decisions\n\n- [ ] no criteria here\n"},
	})
	texts := func(list []Criterion) (out []string) {
		for _, c := range list {
			out = append(out, c.Text)
		}
		return out
	}
	all := []string{"AC-1: a link is sent", "AC-2: an old link says so", "AC-3: it signs in"}
	if got := texts(Criteria(p, "Test Plan")); !slices.Equal(got, []string{"TP-1: the suite runs"}) {
		t.Fatalf("a document's own, at any depth: %v", got)
	}
	if got := texts(Criteria(p, "Test Plan#levels")); !slices.Equal(got, []string{"TP-1: the suite runs"}) {
		t.Fatalf("a section's: %v", got)
	}
	if got := texts(Criteria(p, "User Stories and Acceptance Criteria#story-sign-in")); !slices.Equal(got, []string{"AC-3: it signs in"}) {
		t.Fatalf("a section under the heading: %v", got)
	}
	for _, ref := range []string{"ADR", "ADR#decisions", "Test Plan#other", "no such document", ""} {
		if got := texts(Criteria(p, ref)); !slices.Equal(got, all) {
			t.Fatalf("%q falls back to the stories: %v", ref, got)
		}
	}
	if c := Criteria(p, "ADR")[1]; c.Doc != p.Doc("User Stories and Acceptance Criteria").ID || c.Done || c.Section != "story-reset" {
		t.Fatalf("criterion: %+v", c)
	}
}

// shopDocs are a PRD and a Technical Spec with sections to be based on.
func shopDocs(t *testing.T) (prd, spec string, hash func(body, section string) string) {
	t.Helper()
	return "## Features and priorities\n\nCheckout takes cards.\n\n## Out of scope\n\nNo refunds yet.\n",
		"## Data model and migrations\n\nOrders have totals.\n\n## Security\n\nNothing stored.\n",
		func(body, section string) string {
			h, err := SectionHash(body, section)
			if err != nil {
				t.Fatal(err)
			}
			return h
		}
}

func TestTaskContextCarriesASectionAndWhatItIsBasedOn(t *testing.T) {
	prd, spec, hash := shopDocs(t)
	work := "## Tasks\n\n### Stage 1\n\nBased on: [[PRD#Features and priorities]] @" + hash(prd, "Features and priorities") +
		", [[Technical Spec#Data model and migrations]] @00000000, [[Brief#Problem]] @11111111\n\n" +
		"- [ ] T-1: add checkout (first feature, no dependencies)\n\n### Stage 2\n\nBased on: [[PRD#Out of scope]] @" + hash(prd, "Out of scope") + "\n\n- [ ] T-2: later\n"
	p := workProject(t, [][2]string{
		{"PRD", prd}, {"Technical Spec", spec}, {"Brief", "## Problem\n\nA brief, not quoted.\n"},
		{"Work Breakdown and Backlog", work},
		{"User Stories and Acceptance Criteria", "## Acceptance criteria\n\n- [x] AC-1: already met\n- [ ] AC-2: a card is charged\n"},
	})
	got, err := TaskContext(p, "Work Breakdown and Backlog#stage-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	doc := p.Doc("Work Breakdown and Backlog")
	if first, _, _ := strings.Cut(got, "\n"); first != "Project Docs: root "+doc.ID+"#stage-1" {
		t.Fatalf("first line: %q", first)
	}
	for _, want := range []string{"T-1: add checkout", "Checkout takes cards.", "Orders have totals.", "AC-2: a card is charged", "It has changed since"} {
		if !strings.Contains(got, want) {
			t.Fatalf("no %q in:\n%s", want, got)
		}
	}
	for _, not := range []string{"T-2: later", "No refunds yet.", "Nothing stored.", "A brief, not quoted.", "AC-1: already met", "has these sections"} {
		if strings.Contains(got, not) {
			t.Fatalf("%q in:\n%s", not, got)
		}
	}
	if pos := []int{strings.Index(got, "T-1: add"), strings.Index(got, "Checkout takes"), strings.Index(got, "AC-2:")}; !slices.IsSorted(pos) {
		t.Fatalf("out of order %v:\n%s", pos, got)
	}
	// Only the first link was current, so only the second says it changed.
	if strings.Count(got, "It has changed since") != 1 {
		t.Fatalf("stale:\n%s", got)
	}

	project, gotDoc, section, ok := DocRefOf(got + "\nThe task itself.")
	if !ok || project != "root" || gotDoc != doc.ID || section != "stage-1" {
		t.Fatalf("read back: %q %q %q %v", project, gotDoc, section, ok)
	}
}

func TestTaskContextWithoutLinksGivesTheOutline(t *testing.T) {
	prd, spec, _ := shopDocs(t)
	p := workProject(t, [][2]string{{"PRD", prd}, {"Technical Spec", spec},
		{"Test Plan", "## Levels\n\nUnit tests.\n\n### Unit\n\nFast ones.\n"}})
	d := p.Doc("Test Plan")
	section, err := TaskContext(p, "Test Plan#levels", 0)
	if err != nil {
		t.Fatal(err)
	}
	whole, err := TaskContext(p, d.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]string{"section": section, "whole": whole} {
		for _, want := range []string{"#levels Levels", "  - #unit Unit", `"$ENOWX_BIN" notes show ` + d.ID + " --section S", `"$ENOWX_BIN" notes show ` + p.Doc("PRD").ID + " --section S"} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s: no %q in:\n%s", name, want, got)
			}
		}
	}
	if !strings.Contains(section, "Unit tests.") || strings.Contains(whole, "Unit tests.") {
		t.Fatalf("the section's words belong to the section's context only:\n%s\n---\n%s", section, whole)
	}
	if _, _, _, ok := DocRefOf(whole); !ok {
		t.Fatalf("no line to read back:\n%s", whole)
	}
	if _, err := TaskContext(p, "Test Plan#nothing", 0); err == nil || !strings.Contains(err.Error(), "no section") {
		t.Fatalf("no such section: %v", err)
	}
	if _, err := TaskContext(p, "Nothing", 0); err == nil || !strings.Contains(err.Error(), "Test Plan") {
		t.Fatalf("no such document: %v", err)
	}
}

func TestTaskContextIsCappedAndSaysWhatWasCut(t *testing.T) {
	prd := "## Features\n\n" + strings.Repeat("A long line of what the PRD asks for.\n", 200)
	h, err := SectionHash(prd, "Features")
	if err != nil {
		t.Fatal(err)
	}
	p := workProject(t, [][2]string{{"PRD", prd},
		{"Test Plan", "## Levels\n\nBased on: [[PRD#Features]] @" + h + "\n\nUnit tests.\n"},
		{"User Stories and Acceptance Criteria", "## Acceptance criteria\n\n- [ ] AC-1: met or not\n"}})
	full, err := TaskContext(p, "Test Plan#levels", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{1500, 800} {
		got, err := TaskContext(p, "Test Plan#levels", limit)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) > limit || len(got) >= len(full) {
			t.Fatalf("limit %d: %d bytes", limit, len(got))
		}
		if !strings.Contains(got, "Unit tests.") {
			t.Fatalf("limit %d: the section went first:\n%s", limit, got)
		}
		if !strings.Contains(got, "of PRD #features. Read them with: \"$ENOWX_BIN\" notes show "+p.Doc("PRD").ID+" --section features") {
			t.Fatalf("limit %d: doesn't say what was cut:\n%s", limit, got)
		}
		if !strings.Contains(got, "of the acceptance criteria. Read them with:") {
			t.Fatalf("limit %d: doesn't say the criteria were cut:\n%s", limit, got)
		}
	}
	// Too small for all that: still where it came from, and how to read it.
	got, err := TaskContext(p, "Test Plan#levels", 120)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := DocRefOf(got); !ok || len(got) > 120 {
		t.Fatalf("tiny limit (%d bytes):\n%s", len(got), got)
	}
	if fits, err := TaskContext(p, "Test Plan#levels", len(full)); err != nil || fits != full {
		t.Fatalf("a limit it fits changes nothing: %v", err)
	}
}

func TestDocRefOfOnlyReadsItsOwnLine(t *testing.T) {
	for _, prompt := range []string{
		"",
		"Fix the totals",
		"Project Docs: PRD · {{Draft}} · v1 · @12345678\n\nA document's own first line",
		"Project Docs: Area",
		"See the Project Docs: PRD and more words",
	} {
		if _, _, _, ok := DocRefOf(prompt); ok {
			t.Fatalf("%q reads as a task's line", prompt)
		}
	}
	project, doc, section, ok := DocRefOf("Project Docs: abc def\nthe rest")
	if !ok || project != "abc" || doc != "def" || section != "" {
		t.Fatalf("no section: %q %q %q %v", project, doc, section, ok)
	}
	if RefLine("abc", "def", "") != "Project Docs: abc def" || RefLine("abc", "def", "s") != "Project Docs: abc def#s" {
		t.Fatal("RefLine")
	}
}

func TestTaskIDsAreWhatATitleNames(t *testing.T) {
	if got := TaskIDs("T-3: add checkout, then T-12"); !slices.Equal(got, []string{"T-3", "T-12"}) {
		t.Fatalf("%v", got)
	}
	if got := TaskIDs("OUT-3 and T-x"); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}
