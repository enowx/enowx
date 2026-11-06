package browserview

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// lookShot keeps what tt shows as name under $SHOTS, to compare with the design.
func lookShot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}

// The Browser's list in a panel's width and its bar over a tab of the center,
// through their states.
func TestBrowserStatesInAPanelAndATab(t *testing.T) {
	var tt *ui.Tester
	draw := func(name string, v *View, o Options, want ...string) {
		t.Helper()
		tt = ui.NewTester(func(c *ui.Context) {
			c.SetTheme(theme.Window())
			ui.Row(c).Width(1060).Height(640).Background(theme.Frame).TextColor(theme.Text).Children(func() {
				ui.Column(c).Width(740).FillHeight().Children(func() { v.Draw(c, o) })
				ui.Column(c).Width(320).FillHeight().Children(func() { v.DrawPages(c, o) })
			})
		}, 1060, 640)
		tt.Frame()
		tt.Frame()
		tt.Frame()
		for _, w := range want {
			if !tt.HasText(w) {
				t.Errorf("%s: no %q in %q", name, w, tt.Texts())
			}
		}
		lookShot(t, tt, "browser-"+name)
	}
	page := &fakePage{}
	o := options(page, new(string))
	o.ToCanvas = func([]byte, string, int) {}
	o.Unslop = func() {}
	two := []proto.Port{{Port: 5173, Process: "node"}, {Port: 8080, Process: "enowx-cloud"}}
	v := New()
	v.loaded = true
	v.setPorts(o, two)
	draw("page", v, o, "Pages", "Open in a tab", "localhost:5173", "node", "localhost:8080", "Fill the room")
	if err := tt.Click("Page width: Fill the room"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, w := range []string{"Desktop, 1280", "Tablet, 768", "Phone, 390"} {
		if !tt.HasText(w) {
			t.Errorf("the width menu: no %q in %q", w, tt.Texts())
		}
	}
	lookShot(t, tt, "browser-width-menu")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if err := tt.Click("Ports"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("localhost:8080 · enowx-cloud") {
		t.Errorf("the port picker: %q", tt.Texts())
	}
	lookShot(t, tt, "browser-ports")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	o.Unslopping = true
	draw("unslop-starting", v, o, "Reading the page, making a branch…")
	o.Unslopping, o.UnslopErr, o.UnslopDismiss = false, "the project has changes not committed", func() {}
	draw("unslop-error", v, o, "the project has changes not committed")
	o.UnslopErr = ""
	v.err = "That isn't an address: type a path, a host or a URL."
	draw("error", v, o, "That isn't an address: type a path, a host or a URL.")
	v = New()
	v.loaded = true
	draw("waiting", v, options(&fakePage{}, new(string)), "Open a page")
	draw("looking", New(), options(&fakePage{}, new(string)), "Looking for the session's dev server…")
	draw("no-webview", New(), Options{OpenURL: func(string) {}}, "This device has no web view to show pages in. Open them in the browser instead.")
}
