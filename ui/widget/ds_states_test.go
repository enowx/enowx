package widget

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

func dsShot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name+".png"), pngOf(t, tt), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pngOf(t *testing.T, tt *ui.Tester) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, tt.Image()); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// The space picker opens on the spaces, the current one checked, and picks.
func TestSpacePickerPicks(t *testing.T) {
	picked := "none"
	spaces := []Space{{Name: "Personal"}, {Team: "0123456789abcdef", Name: "Design"}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(480).Height(240).Padding(theme.S4).Background(theme.Panel).TextColor(theme.Text).Children(func() {
			SpacePicker(c, spaces[0], "on this Mac", theme.TextLabel, spaces, func(team string) { picked = team })
		})
	}, 480, 240)
	tt.Frame()
	if err := tt.Click("Space: Personal"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	tt.Frame()
	dsShot(t, tt, "ds-space-picker")
	if err := tt.Click("Design"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	if picked != "0123456789abcdef" {
		t.Fatalf("picked %q", picked)
	}
}

// Text dragged over what takes it outlines it, and let go there, it is taken.
func TestTextDragOutlinesAndDrops(t *testing.T) {
	got := ""
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(480).Height(240).Gap(theme.S4).Padding(theme.S4).Background(theme.Panel).TextColor(theme.Text).Children(func() {
			ui.Text(c, "A session").Height(32).Drag(TextDrag{Text: "the session's name"})
			well := ui.Row(c).FillWidth().Height(80).Radius(theme.RadiusCard).Background(theme.Sunken).Children(func() { ui.Text(c, "Composer") })
			if text, ok := TakeTextDrag(well); ok {
				got = text
			}
		})
	}, 480, 240)
	tt.Frame()
	from, ok := tt.Find("A session")
	to, ok2 := tt.Find("Composer")
	if !ok || !ok2 {
		t.Fatalf("%q", tt.Texts())
	}
	before := pngOf(t, tt)
	tt.Press(from.X+4, from.Y+8)
	tt.Move(from.X+4, from.Y+20)
	tt.Frame()
	tt.Move(to.X+8, to.Y+8)
	tt.Frame()
	tt.Frame()
	dsShot(t, tt, "ds-text-drag")
	if bytes.Equal(before, pngOf(t, tt)) {
		t.Error("nothing shows the drag over the composer")
	}
	tt.Release(to.X+8, to.Y+8)
	tt.Frame()
	tt.Frame()
	if got != "the session's name" {
		t.Fatalf("took %q", got)
	}
}

// A tooltip is the toolkit's, after a rest over its control, and shows whole.
func TestTooltipShowsOnRest(t *testing.T) {
	const tip = "More for this canvas, a tooltip long enough to see how it holds"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(480).Height(160).Padding(theme.S5).Background(theme.Panel).TextColor(theme.Text).Children(func() {
			IconButton(c, icon.More, tip, false)
		})
	}, 480, 160)
	tt.Frame()
	r, ok := tt.Find(tip)
	if !ok {
		t.Fatalf("no button: %q", tt.Texts())
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(2 * time.Second)
	tt.Frame()
	dsShot(t, tt, "ds-tooltip")
	shows := 0
	for _, x := range tt.Texts() {
		if x == tip {
			shows++
		}
	}
	if shows < 2 { // the button's label, and the tooltip's words
		t.Fatalf("no tooltip: %q", tt.Texts())
	}
}

// What moves rides its spring over time, and with Reduce Motion it jumps.
func TestMotionAndReduceMotion(t *testing.T) {
	draw := func(reduce bool) (first, later []byte) {
		open := false
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Width(200).Height(200).Background(theme.Panel).Children(func() {
				at := float32(0)
				if open {
					at = 80
				}
				ui.Box(c.Key("pill")).Size(60, 60).Margin(at, 0, 0, at).Background(theme.Accent).Transition(LayoutMotion)
			})
		}, 200, 200)
		tt.SetPreferences(ui.Preferences{ReduceMotion: reduce})
		tt.Frame()
		open = true
		tt.Frame()
		first = pngOf(t, tt)
		tt.Advance(2 * time.Second)
		tt.Frame()
		return first, pngOf(t, tt)
	}
	if first, later := draw(false); bytes.Equal(first, later) {
		t.Error("it didn't move: its first frame is its last")
	}
	if first, later := draw(true); !bytes.Equal(first, later) {
		t.Error("with Reduce Motion it still moved")
	}
}
