package browserview

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// The toolbar's Unslop starts one on a page of the project running here,
// and is off for a page elsewhere; a run under way shows over the page.
func TestUnslopInTheBrowser(t *testing.T) {
	page := &fakePage{}
	o := options(page, new(string))
	started := 0
	o.Unslop = func() { started++ }
	v := New()
	v.loaded = true
	v.setPorts(o, []proto.Port{{Port: 5173}})
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 1000, 600)
	tt.Frame()
	if err := tt.Click("Unslop"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if started != 1 {
		t.Fatalf("started %d times", started)
	}
	// While it starts, the panel says so at once; one that didn't start
	// says why, until dismissed.
	o.Unslopping = true
	tt.Frame()
	if !tt.HasText("Unslop starting…") {
		t.Fatalf("no starting panel: %q", tt.Texts())
	}
	dismissed := false
	o.Unslopping, o.UnslopErr, o.UnslopDismiss = false, "the project has changes not committed", func() { dismissed = true }
	tt.Frame()
	if !tt.HasText("Unslop didn't start") || !tt.HasText("the project has changes not committed") {
		t.Fatalf("no error: %q", tt.Texts())
	}
	if err := tt.Click("Dismiss"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !dismissed {
		t.Fatal("the error wasn't dismissed")
	}
	o.UnslopErr = ""
	o.UnslopRun = &proto.CanvasUnslopRun{ID: "s1", Frame: "page", From: 8, Score: 8, Scans: 1, Passes: []string{"01 Truth", "02 Structure"}}
	tt.Frame()
	if !tt.HasText("Reading the page…") || !tt.HasText("Truth") {
		t.Fatalf("no panel over the page: %q", tt.Texts())
	}
	// Done, with its pictures taken: Compare opens its before and after.
	compared := 0
	o.UnslopCompare = func(proto.CanvasUnslopRun) { compared++ }
	o.UnslopRun = &proto.CanvasUnslopRun{ID: "s1", Frame: "page", From: 8, Score: 70, Scans: 3, Done: true, Shots: 3, Passes: []string{"01 Truth"}}
	tt.Frame()
	if err := tt.Click("Compare"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	if compared != 1 {
		t.Fatalf("compared %d times", compared)
	}
	page.url = "https://example.com/"
	o.UnslopRun = nil
	tt.Frame()
	if err := tt.Click("Unslop works on the project's pages running here (localhost)"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if started != 1 {
		t.Fatal("an unslop started on a page elsewhere")
	}
}
