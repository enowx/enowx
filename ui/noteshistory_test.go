package ui

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

// A page's earlier version, kept before an agent's change was accepted,
// comes back from Version history.
func TestNoteVersionComesBack(t *testing.T) {
	h := newNotesHarness(t)
	n, _ := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Plan"}, Body: "- [ ] the first plan\n"})
	full, _ := h.cl.Note(n.ID)
	full.Body = "- [ ] a plan rewritten\n"
	p, err := h.cl.ProposeNote(full, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.cl.AcceptNoteProposal(p.ID); err != nil {
		t.Fatal(err)
	}
	h.settle("the page", func() bool { return h.tt.HasText("Plan") })
	if err := h.tt.Click("Plan"); err != nil {
		t.Fatal(err)
	}
	h.settle("the page open", func() bool { return h.tt.HasText("a plan rewritten") })
	if err := h.tt.Click("More for this note"); err != nil {
		t.Fatal(err)
	}
	h.settle("the menu", func() bool { return h.tt.HasText("Version history…") })
	if err := h.tt.Click("Version history…"); err != nil {
		t.Fatal(err)
	}
	h.settle("the version", func() bool { return h.tt.HasText("the first plan") && h.tt.HasText("Restore") })
	shot(t, h.tt, "notes-history.png")
	if err := h.tt.Click("Restore"); err != nil {
		t.Fatal(err)
	}
	h.settle("restored", func() bool {
		got, err := h.cl.Note(n.ID)
		return err == nil && got.Body == "- [ ] the first plan\n" && !h.tt.HasText("Restore")
	})
}
