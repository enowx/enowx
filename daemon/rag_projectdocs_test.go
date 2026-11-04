package daemon

import (
	"strings"
	"testing"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/core/rag"
	"github.com/enowx/enowx/proto"
)

// A document attached to the project is indexed with it, without what only
// records it; one under an attached project, itself not attached, isn't.
func TestRAGIndexesAttachedDocuments(t *testing.T) {
	srv, _ := hookServer(t)
	dir := t.TempDir()
	on := []proto.NoteProject{{Path: dir}}
	root, err := srv.notes.Save(proto.Note{NoteInfo: proto.NoteInfo{Title: "Shop", Projects: on}, Body: projectdocs.ProjectBody(projectdocs.SizeCore, projectdocs.ApprovalRequired)})
	if err != nil {
		t.Fatal(err)
	}
	body := projectdocs.Meta{Kind: "PRD", Status: projectdocs.StatusDraft, Version: 1}.Line() +
		"\n\n## Goals\n\nRefunds reach the buyer within a day.\n\n## Users\n\n_To fill: each main kind of user._\n"
	prd, err := srv.notes.Save(proto.Note{NoteInfo: proto.NoteInfo{Title: "PRD", Parent: root.ID, Projects: on}, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	loose, err := srv.notes.Save(proto.Note{NoteInfo: proto.NoteInfo{Title: "ERD", Parent: root.ID}, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{}
	for _, d := range srv.ragExtra(dir, rag.ProjectID(dir)) {
		docs[d.ID] = d.Text
	}
	got, ok := docs["note:"+prd.ID]
	if !ok {
		t.Fatalf("the document isn't indexed: %v", docs)
	}
	if !strings.Contains(got, "Refunds reach the buyer") || strings.Contains(got, "To fill") || strings.Contains(got, "Project Docs:") {
		t.Fatalf("indexed as %q", got)
	}
	if _, ok := docs["note:"+loose.ID]; ok {
		t.Fatal("a note not attached is indexed")
	}
}
