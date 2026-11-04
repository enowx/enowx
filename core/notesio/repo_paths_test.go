package notesio

import (
	"slices"
	"testing"

	"github.com/enowx/enowx/core/notes"
	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// The app lists RepoPaths before the daemon writes: both must name the same
// files, a second document of one title too.
func TestExportWritesWhatRepoPathsList(t *testing.T) {
	st := notes.Open(t.TempDir())
	root, _, err := projectdocs.New(projectdocs.Options{Title: "Shop", Size: projectdocs.SizeFull}, st.Save)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Save(proto.Note{NoteInfo: proto.NoteInfo{Title: "PRD", Parent: root.ID},
		Body: projectdocs.Meta{Kind: "PRD", Version: 1}.Line() + "\n\nAnother one.\n"}); err != nil {
		t.Fatal(err)
	}
	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	p, err := projectdocs.Load(list, root.ID, st.Get)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ExportRepo(st, root.ID, t.TempDir(), RepoOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := projectdocs.RepoPaths(p); !slices.Equal(res.Files, want) {
		t.Fatalf("export %v\nlisted %v", res.Files, want)
	}
}
