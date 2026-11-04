package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The page asks through the host's daemon: with no cloud on it, it says there's
// no library and offers nothing, and Canvas goes back.
func TestCommunityPageWithoutACloud(t *testing.T) {
	h := newCanvasHarness(t)
	h.app.page = pageCommunity
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1280).Height(800).Background(theme.Background).Children(func() { h.app.workspaceView(c) })
	}, 1280, 800)
	h.settle("the page's answer", func() bool { return h.tt.HasText("This host's cloud has no community library") })
	if h.tt.HasText("Search designs") {
		t.Fatalf("something to browse: %q", h.tt.Texts())
	}
	if err := h.tt.Click("Canvas"); err != nil {
		t.Fatal(err)
	}
	h.settle("back on Canvas", func() bool { return h.app.page == pageCanvas })
	if h.tt.HasText("Community library") {
		t.Fatalf("the list offers a library the cloud hasn't: %q", h.tt.Texts())
	}
}

// An agent's asking for the form is a card: it moves nothing until it's clicked.
func TestAnAgentsAskingForTheFormIsACard(t *testing.T) {
	h := newCanvasHarness(t)
	cv, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Landing", Kind: proto.CanvasWireframe, Frames: []proto.CanvasFrame{{Name: "One"}}})
	if err != nil {
		t.Fatal(err)
	}
	h.app.page = pageSettings
	raw, _ := json.Marshal(proto.LibraryPublishAsked{Canvas: cv.ID, Frame: cv.Frames[0].ID, CanvasTitle: "Landing", FrameName: "One", From: "session"})
	apply := h.app.libraryAsked(raw)
	if apply == nil {
		t.Fatal("the event doesn't read")
	}
	apply()
	if h.app.page != pageSettings || len(h.app.sessionToasts) != 1 {
		t.Fatalf("page %q, cards %+v", h.app.page, h.app.sessionToasts)
	}
	card := h.app.sessionToasts[0]
	if card.name != "Landing" || !strings.Contains(card.text, "An agent asks you") || !strings.Contains(card.text, "Nothing is sent until you do it in the form") {
		t.Fatalf("the card: %+v", card)
	}
	card.open()
	h.settle("the canvas", func() bool {
		id, _ := h.app.canvases.Showing()
		return h.app.page == pageCanvas && id == cv.ID
	})
}
