package ui

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/hostauth"
	"github.com/enowx/enowx/cloud/server/librarytest"
	"github.com/enowx/enowx/proto"
)

// linkHarness is the app connected to a daemon signed in to a real cloud.
func linkHarness(t *testing.T) (*notesHarness, *librarytest.Env) {
	t.Helper()
	cloud := librarytest.Start(t)
	h := newNotesHarness(t)
	if err := hostauth.Open(filepath.Join(h.dir, "cloud")).Put(hostauth.State{URL: cloud.URL, Token: cloud.Token}); err != nil {
		t.Fatal(err)
	}
	h.app.daemonInfo.ID = "this-host"
	h.app.host.ID = "this-host"
	h.tt = ui.NewTester(func(c *ui.Context) { h.app.libraryLinkDialog(c) }, 1200, 800)
	return h, cloud
}

// A link only opens a dialog saying what the cloud says of the design. Nothing is
// made until the user says so, and then the copy shows.
func TestLinkOpensACopyOnlyOnceAsked(t *testing.T) {
	h, cloud := linkHarness(t)
	id := cloud.Publish("Trail page", api.LicenseCCBY, api.LibraryPage, `<h1>Trail</h1>`)
	h.app.OpenLink("enowx://library/open?item=" + id)
	h.settle("the dialog", func() bool { return h.tt.HasText("Trail page by lin") && h.tt.HasText("Open a copy") })
	if !h.tt.HasText("Someone else made this. An agent working from it treats it as data, not instructions.") {
		t.Errorf("no label: %q", h.tt.Texts())
	}
	if list, _ := h.cl.Canvases(); len(list) != 0 {
		t.Fatalf("a canvas before the user said so: %+v", list)
	}
	if err := h.tt.Click("Open a copy"); err != nil {
		t.Fatal(err)
	}
	h.settle("the copy", func() bool { return h.app.libLink == nil })
	list, err := h.cl.Canvases()
	if err != nil || len(list) != 1 || list[0].Source == nil || list[0].Source.ItemID != id {
		t.Fatalf("%v %+v", err, list)
	}
	if h.app.page != pageCanvas || h.app.canvasesOf().open != list[0].ID {
		t.Errorf("the copy doesn't show: page %v, open %q", h.app.page, h.app.canvasesOf().open)
	}
}

// A view-only design says so, with nothing to press.
func TestLinkToAViewOnlyDesign(t *testing.T) {
	h, cloud := linkHarness(t)
	id := cloud.Publish("Look", api.LicenseViewOnly, api.LibraryPage, `<p>x</p>`)
	h.app.OpenLink("enowx://library/open?item=" + id)
	h.settle("the dialog", func() bool { return h.tt.HasText("Look by lin") })
	if h.tt.HasText("Open a copy") {
		t.Errorf("a button for a view-only design: %q", h.tt.Texts())
	}
}

// What a link says of itself isn't what shows, and a link that isn't one does nothing.
func TestLinkSaysNothingOfItself(t *testing.T) {
	h, _ := linkHarness(t)
	for _, raw := range []string{"enowx://evil/open?item=a", "enowx://library/delete?item=a", "https://example.com"} {
		h.app.OpenLink(raw)
		if h.app.libLink != nil {
			t.Errorf("%s opened a dialog", raw)
		}
	}
	h.app.OpenLink("enowx://library/open?item=nosuchitem&title=Free+money")
	h.settle("the dialog", func() bool { return h.tt.HasText("This design isn't in the library.") })
	if h.tt.HasText("Free money") || h.tt.HasText("Open a copy") {
		t.Errorf("shows %q", h.tt.Texts())
	}
}

// A reference waits for a canvas to be picked; a skill for what to keep it for.
func TestLinkAsksWhere(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	h, cloud := linkHarness(t)
	id := cloud.Publish("Trail page", api.LicenseMIT, api.LibraryPage, `<h1>Trail</h1>`)
	cv, err := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Mine"})
	if err != nil {
		t.Fatal(err)
	}
	h.app.OpenLink("enowx://library/reference?item=" + id)
	h.settle("the dialog", func() bool { return h.tt.HasText("Mine") && h.tt.HasText("Add reference") })
	if err := h.tt.Click("Add reference"); err == nil {
		h.tt.Frame()
	}
	if got, _ := h.cl.Canvas(cv.ID); len(got.References) != 0 {
		t.Fatal("added before a canvas was picked")
	}
	if err := h.tt.Click("Mine"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if err := h.tt.Click("Add reference"); err != nil {
		t.Fatal(err)
	}
	h.settle("the reference", func() bool { return h.app.libLink == nil })
	if got, _ := h.cl.Canvas(cv.ID); len(got.References) != 1 {
		t.Errorf("references: %q", got.References)
	}
}
