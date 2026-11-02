package ui

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// Settings' Terminal section lists this device's monospaced fonts, and
// picking one makes it the terminals' font.
func TestFontSectionPicksTheTerminalsFont(t *testing.T) {
	a := New(Options{})
	// The device's own fonts, rather than asking the system for them.
	a.fonts = []ui.FontFamily{{Name: "Fira Code", Monospaced: true}, {Name: "Test Mono", Monospaced: true}}
	a.fontsLoaded = true
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(520).Height(420).Padding(20).Gap(16).Background(theme.Background).Children(func() {
			a.fontSection(c)
		})
	}, 520, 420)
	tt.Frame()
	if !tt.HasText("Test Mono") || !tt.HasText("System monospaced font") {
		t.Fatalf("the fonts are not listed: %q", tt.Texts())
	}
	if err := tt.Click("Test Mono"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.state.TermFont != "Test Mono" {
		t.Fatalf("picked %q, not Test Mono", a.state.TermFont)
	}
	if f := a.termFont(); f.Family != "Test Mono" {
		t.Fatalf("the terminals take %q", f.Family)
	}
	if err := tt.Click("System monospaced font"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.state.TermFont != "" {
		t.Fatalf("the system's font leaves %q", a.state.TermFont)
	}
}

// A device with no monospaced font of its own says so, and is not asked
// again: the section is drawn every frame.
func TestFontSectionWithNoMonospacedFont(t *testing.T) {
	a := New(Options{})
	a.fontsLoaded = true
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(520).Height(420).Padding(20).Gap(16).Background(theme.Background).Children(func() {
			a.fontSection(c)
		})
	}, 520, 420)
	tt.Frame()
	if !tt.HasText("This device has no other monospaced font.") {
		t.Fatalf("the section does not say the device has none: %q", tt.Texts())
	}
	if a.fontsLoading {
		t.Fatal("the fonts are read again, once a frame")
	}
}

// A size saved or zoomed outside the row of choices shows as a further one,
// picked; picking another changes it, and the size in use is never lost.
func TestTermSizeOutsideTheChoicesStaysShown(t *testing.T) {
	a := New(Options{})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Width(500).Children(func() { a.termSizeControl(c) })
	}, 500, 80)
	for _, size := range []float32{termSizeMin, 20, termSizeMax} {
		a.state.TermFontSize = size
		tt.Frame()
		if !tt.HasText(fmt.Sprintf("%g", size)) {
			t.Errorf("size %g isn't shown: %q", size, tt.Texts())
		}
		if a.termSize() != size {
			t.Errorf("drawing changed size %g to %g", size, a.termSize())
		}
	}
	if err := tt.Click("14"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.termSize() != 14 {
		t.Fatalf("picked 14, have %g", a.termSize())
	}
	if tt.HasText("32") {
		t.Errorf("the old size stays a choice once left: %q", tt.Texts())
	}
	if !tt.HasText("13") {
		t.Errorf("the default isn't a choice: %q", tt.Texts())
	}
}
