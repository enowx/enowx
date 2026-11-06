package browserview

import (
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// Docked in the side panel, at its narrowest and when wide, the bar keeps
// back, forward, reload, the address and the way out inside the panel, none
// over another, and the page's tools on a row of their own; the button
// moves the page.
func TestDockedBrowserFitsThePanel(t *testing.T) {
	for _, width := range []float32{280, 480, 900} {
		page := &fakePage{}
		o := options(page, new(string))
		o.ToCanvas = func([]byte, string, int) {}
		o.Unslop = func() {}
		docked := true
		o.Docked = true
		o.Dock = func(d bool) { docked = d }
		v := New()
		v.loaded = true
		v.setPorts(o, []proto.Port{{Port: 5173, Process: "node"}, {Port: 8080}})
		page.url = "http://localhost:5173/" + strings.Repeat("a-very-long-path/", 12)
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(theme.Window())
			ui.Row(c).Width(width).Height(600).Background(theme.Frame).TextColor(theme.Text).Children(func() {
				ui.Column(c).FillWidth().FillHeight().Children(func() { v.Draw(c, o) })
			})
		}, int(width), 600)
		tt.Frame()
		tt.Frame()
		var seen []ui.Rect
		for _, label := range []string{"Back", "Forward", "Reload", "Address", "Open as a page", "More for the page", "Page width: Fill the room", "Screenshot"} {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("%v: no %q in %q", width, label, tt.Texts())
			}
			if r.X < 0 || r.X+r.W > width+0.5 || r.W <= 0 {
				t.Errorf("%v: %q out of the panel: %+v", width, label, r)
			}
			for _, q := range seen {
				if r.X < q.X+q.W && q.X < r.X+r.W && r.Y < q.Y+q.H && q.Y < r.Y+r.H {
					t.Errorf("%v: %q over another control: %+v %+v", width, label, r, q)
				}
			}
			seen = append(seen, r)
		}
		if r, _ := tt.Find("Address"); r.W < 80 {
			t.Errorf("%v: the address is %v wide", width, r.W)
		}
		back, _ := tt.Find("Back")
		shot, _ := tt.Find("Screenshot")
		if shot.Y <= back.Y {
			t.Errorf("%v: the page's tools share the address's row", width)
		}
		lookShot(t, tt, "browser-docked-"+strconv.Itoa(int(width)))
		if err := tt.Click("Open as a page"); err != nil {
			t.Fatal(err)
		}
		if docked {
			t.Fatalf("%v: the button didn't pop the page out", width)
		}
	}
}
