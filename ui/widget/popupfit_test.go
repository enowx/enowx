package widget

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

const fitW, fitH = 1200, 720

func inWindow(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	b, ok := tt.Find(label)
	if !ok {
		t.Fatalf("%q isn't there", label)
	}
	if b.W == 0 || b.H == 0 || b.X < 0 || b.Y < 0 || b.X+b.W > fitW || b.Y+b.H > fitH {
		t.Errorf("%q at %+v, not in the %dx%d window", label, b, fitW, fitH)
	}
}

// menuWindow is a window with a button that opens a menu of n items, at (x, y);
// picked gets the label of the item clicked.
func menuWindow(n int, x, y float32, beside bool, picked *string) *ui.Tester {
	var m ContextMenu
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(fitW, fitH).Children(func() {
			ui.Box(c.Key("outside")).Size(10, 10).Label("Outside")
			b := ui.ButtonBase(c.Key("open")).Absolute().Left(x).Top(y).Size(60, 28).Label("Open")
			m.OpenBelow(b, "menu", 28)
			items := func(done func()) {
				for i := range n {
					label := fmt.Sprintf("Item %d", i)
					if MenuItem(c.Key(i), nil, label, false) {
						*picked = label
						done()
					}
				}
			}
			if beside {
				m.ShowBeside(c, b, "menu", items)
			} else {
				m.Show(c, b, "menu", items)
			}
		})
	}, fitW, fitH)
	tt.Frame()
	return tt
}

// Wherever it opens, a long menu's first item shows in the window and its last
// is reached by scrolling, and clicked at once: a scrolled menu loses no click.
func TestLongMenuStaysInTheWindow(t *testing.T) {
	spots := []struct {
		name   string
		x, y   float32
		beside bool
	}{
		{"top left", 8, 8, false},
		{"middle", 560, 340, false},
		{"bottom right", 1130, 680, false},
		{"beside at bottom right", 1130, 680, true},
		{"beside in the middle", 560, 340, true},
	}
	for _, s := range spots {
		t.Run(s.name, func(t *testing.T) {
			picked := ""
			tt := menuWindow(60, s.x, s.y, s.beside, &picked)
			if err := tt.Click("Open"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			tt.Frame()
			first, last := "Item 0", "Item 59"
			if b, ok := tt.Find(first); !ok || b.H == 0 {
				t.Fatalf("the first item isn't shown: %+v", b)
			}
			inWindow(t, tt, first)
			top, _ := tt.Find(first)
			tt.Scroll(top.X+top.W/2, top.Y+top.H/2, 0, 4000)
			tt.Frame()
			inWindow(t, tt, last)
			if err := tt.Click(last); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if picked != last {
				t.Errorf("picked %q", picked)
			}
			if tt.HasText(last) {
				t.Error("the menu stayed open after its pick")
			}
		})
	}
}

// A menu that fits near the window's foot moves up whole: it doesn't scroll, and
// has no room kept for a bar.
func TestShortMenuNearTheFootShowsWhole(t *testing.T) {
	picked := ""
	tt := menuWindow(6, 1130, 680, false, &picked)
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	for i := range 6 {
		inWindow(t, tt, fmt.Sprintf("Item %d", i))
	}
	first, _ := tt.Find("Item 0")
	if first.W < 180 {
		t.Errorf("an item %v wide in a menu 190 wide: room kept for a bar", first.W)
	}
	if err := tt.Click("Item 5"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if picked != "Item 5" {
		t.Errorf("picked %q", picked)
	}
}

func TestMenuClosesWithEscapeAndAClickOutside(t *testing.T) {
	picked := ""
	tt := menuWindow(4, 300, 300, false, &picked)
	open := func() {
		t.Helper()
		if err := tt.Click("Open"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !tt.HasText("Item 0") {
			t.Fatal("no menu")
		}
	}
	open()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if tt.HasText("Item 0") {
		t.Error("Escape left it open")
	}
	open()
	tt.ClickAt(900, 600)
	tt.Frame()
	if tt.HasText("Item 0") {
		t.Error("a click outside left it open")
	}
}

// A menu goes with what it belongs to: when the user comes back to it, it
// doesn't open by itself.
func TestMenuGoesWithItsAnchor(t *testing.T) {
	var m ContextMenu
	shows := true
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Size(fitW, fitH).Children(func() {
			if !shows {
				return
			}
			b := ui.ButtonBase(c.Key("open")).Size(60, 28).Label("Open")
			m.OpenBelow(b, "menu", 28)
			m.Show(c, b, "menu", func(done func()) { MenuItem(c.Key("one"), nil, "Item", false) })
		})
	}, fitW, fitH)
	tt.Frame()
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !tt.HasText("Item") {
		t.Fatal("no menu")
	}
	shows = false
	tt.Frame()
	tt.Frame()
	shows = true
	tt.Frame()
	tt.Frame()
	if tt.HasText("Item") {
		t.Error("the menu came back with its anchor")
	}
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Item") {
		t.Error("it doesn't open again")
	}
}

// A select whose list fits on neither side of it stays in the window: its last
// option is reached by scrolling. It closes with Escape and a click outside.
func TestChoiceStaysInTheWindow(t *testing.T) {
	const w, h = 800, 480
	var options []string
	for i := range 80 {
		options = append(options, fmt.Sprintf("Option %d", i))
	}
	for _, y := range []float32{8, 120, 226, 340, 444} {
		cur := 0
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Size(w, h).Children(func() {
				ui.Box(c.Key("at")).Absolute().Left(300).Top(y).Width(200).Children(func() {
					if i := Choice(c, "Pick", options, cur, 200); i >= 0 {
						cur = i
					}
				})
			})
		}, w, h)
		tt.Frame()
		open := func() {
			t.Helper()
			if err := tt.Click("Pick"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			tt.Frame()
		}
		open()
		// Option 0 is on the trigger too; the rest are the panel's alone.
		one, ok := tt.Find("Option 1")
		if !ok || one.H == 0 {
			t.Fatalf("at %v: Option 1 at %+v", y, one)
		}
		// The pointer first, as a hand does: the row it lands on takes the highlight,
		// which scrolls to it.
		tt.Move(one.X+one.W/2, one.Y+one.H/2)
		tt.Frame()
		tt.Scroll(one.X+one.W/2, one.Y+one.H/2, 0, 8000)
		tt.Frame()
		last, ok := tt.Find("Option 79")
		if !ok || last.H < 14 || last.Y < 0 || last.Y+last.H > h {
			t.Errorf("at %v: scrolled to its end, the last option is at %+v in a window %v high", y, last, h)
		}
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if tt.HasText("Option 1") {
			t.Errorf("at %v: Escape left it open", y)
		}
		open()
		tt.ClickAt(780, 240)
		tt.Frame()
		if tt.HasText("Option 1") {
			t.Errorf("at %v: a click outside left it open", y)
		}
	}
}

// A tooltip goes with the pointer leaving its element, or with the element.
func TestTooltipGoesWithPointerAndElement(t *testing.T) {
	shows := true
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Size(fitW, fitH).Padding(80).Children(func() {
			if shows {
				ui.ButtonBase(c.Key("b")).Size(60, 28).Label("Tipped").Tooltip("The tip")
			}
		})
	}, fitW, fitH)
	tt.Frame()
	rest := func() {
		t.Helper()
		b, ok := tt.Find("Tipped")
		if !ok {
			t.Fatal("no button")
		}
		tt.Move(b.X+b.W/2, b.Y+b.H/2)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		if !tt.HasText("The tip") {
			t.Fatal("no tip after resting")
		}
	}
	rest()
	tt.Move(900, 600)
	tt.Frame()
	if tt.HasText("The tip") {
		t.Error("the tip stayed as the pointer left")
	}
	tt.Advance(time.Second)
	rest()
	shows = false
	tt.Frame()
	tt.Frame()
	if tt.HasText("The tip") {
		t.Error("the tip stayed with its element gone")
	}
}
