package ui

import (
	"encoding/json"
	"testing"

	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/proto"
)

// An agent asking to keep a skill raises a card; only a click on it opens the
// dialog, and what asked decides no more than which design.
func TestSkillAskedOnlyRaisesACard(t *testing.T) {
	h := newCanvasHarness(t)
	data, err := json.Marshal(proto.LibrarySkillAsked{ItemID: "0123456789abcdef", Version: 2, From: "session"})
	if err != nil {
		t.Fatal(err)
	}
	apply := h.app.librarySkillAsked(data)
	if apply == nil {
		t.Fatal("no card")
	}
	apply()
	if h.app.libLink != nil {
		t.Fatal("the dialog opened by itself")
	}
	if len(h.app.sessionToasts) != 1 || h.app.sessionToasts[0].open == nil {
		t.Fatalf("cards: %+v", h.app.sessionToasts)
	}
	h.app.sessionToasts[0].open()
	if ll := h.app.libLink; ll == nil || !ll.open || ll.link.Action != designshare.LinkSkill || ll.link.Item != "0123456789abcdef" || ll.link.Version != 2 {
		t.Fatalf("the dialog: %+v", ll)
	}
	bad, _ := json.Marshal(proto.LibrarySkillAsked{ItemID: "../../etc"})
	if h.app.librarySkillAsked(bad) != nil {
		t.Fatal("a card for what isn't an ID")
	}
}
