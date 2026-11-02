package ui

import (
	"testing"
	"time"

	"github.com/enowx/enowx/ui/theme"
)

// The panel is sliced out of a window with nothing under it: however a slice
// is lit, dimmed or tinted, the desktop never shows through it, in either mode.
func TestFloatingSlicesStayOpaque(t *testing.T) {
	for _, light := range []bool{false, true} {
		app := New(Options{})
		if light {
			app.state.Theme = "light"
		}
		for _, tint := range []bool{false, true} {
			app.state.FloatingTint = tint
			if s := app.floatingSurface(); s != theme.Solid(s) {
				t.Errorf("light %v, tint %v: the surface is %v", light, tint, s)
			}
			for _, act := range []floatingAction{
				{enabled: true},
				{enabled: false},
				{enabled: true, fan: floatingActiveFan},
				{enabled: true, status: &sessionStatus{color: theme.Warning}},
				{enabled: false, status: &sessionStatus{color: theme.Error}},
			} {
				for _, hover := range []float32{0, 0.5, 1} {
					st := &floatingState{fan: floatingActiveFan}
					if f := app.sliceFill(st, act, hover); f != theme.Solid(f) {
						t.Errorf("light %v, tint %v, slice %+v at %v: %v", light, tint, act, hover, f)
					}
					if f := app.floatingFanFill(act, hover); f != theme.Solid(f) {
						t.Errorf("light %v, tint %v, fan child %+v at %v: %v", light, tint, act, hover, f)
					}
				}
			}
		}
	}
}

// A picture of the panel and of its fan, for comparing with the design's.
func TestFloatingPanelPicture(t *testing.T) {
	app := New(Options{})
	tt := floatingTester(t, app)
	for range 20 {
		tt.Advance(50 * time.Millisecond)
		tt.Frame()
	}
	shot(t, tt, "floating/pie.png")
	if err := tt.Click("Active sessions"); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		tt.Advance(50 * time.Millisecond)
		tt.Frame()
	}
	shot(t, tt, "floating/fan.png")
}
