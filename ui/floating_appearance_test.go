package ui

import (
	"path/filepath"
	"strconv"
	"testing"

	mygoui "github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

func TestFloatingAppearanceIndependentAndPersisted(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	app := New(Options{StateFile: file})
	app.host = LocalHost()
	appAccent := theme.Accent
	tt := floatingSectionFrame(t, app, app.floatingAppearance)
	if err := tt.Click("Floating color #0E7490"); err != nil {
		t.Fatal(err)
	}
	if app.floatingAccent() != mygoui.Hex("#0E7490") || theme.Accent != appAccent || app.state.Accent != "" {
		t.Fatal("floating color changed the app theme or did not apply")
	}
	plain := app.sliceFill(&floatingState{source: -1}, floatingAction{enabled: true}, 0)
	for _, label := range []string{"Tint menu segments switch", "Compact floating control switch", "Reduce floating motion switch"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if app.sliceFill(&floatingState{source: -1}, floatingAction{enabled: true}, 0) == plain {
		t.Fatal("tinted style did not change the menu surface")
	}
	app.saveState()
	saved := loadState(file)
	if saved.FloatingAccent != "#0E7490" || !saved.FloatingTint || !saved.FloatingCompact || !saved.FloatingReduceMotion {
		t.Fatal("floating appearance was not saved")
	}
	if err := tt.Click("Follow app theme"); err != nil {
		t.Fatal(err)
	}
	if app.state.FloatingAccent != "" || app.floatingAccent() != theme.Accent {
		t.Fatal("following the app theme did not reset the floating color")
	}
}

func TestFloatingCustomColorValidation(t *testing.T) {
	app := New(Options{})
	tt := floatingSectionFrame(t, app, app.floatingAppearance)
	if err := tt.Click("Custom floating color"); err != nil {
		t.Fatal(err)
	}
	tt.Type("invalid")
	if app.state.FloatingAccent != "" || !tt.HasText("Type a color as #RRGGBB.") {
		t.Fatal("invalid custom color was accepted or had no feedback")
	}
	tt.Key(mygoui.Cmd, mygoui.KeyA)
	tt.Type("#7ccbd3")
	if app.state.FloatingAccent != "#7CCBD3" || tt.HasText("Type a color as #RRGGBB.") {
		t.Fatal("valid custom color was not applied")
	}
}

func TestFloatingCompactAndReducedMotionNavigation(t *testing.T) {
	for _, compact := range []bool{false, true} {
		t.Run(strconv.FormatBool(compact), func(t *testing.T) {
			app, _ := floatingWithSessions(t)
			app.state.FloatingCompact = compact
			app.state.FloatingReduceMotion = true
			size := app.floatingWindowSize()
			if compact && size >= floatingSize {
				t.Fatal("compact window did not shrink")
			}
			tt := mygoui.NewTester(app.floatingView, size, size)
			tt.Frame()
			// Hit the painted sector away from its icon, in window pixels.
			scale := app.floatingScale()
			tt.ClickAt(float32(floatingSize)/2*scale, (float32(floatingSize)/2-floatingPizzaOut+6)*scale)
			if !tt.HasText("This PC") {
				t.Fatal("reduced motion still delayed opening the submenu")
			}
			if err := tt.Click("This PC"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("Waiting one"); err != nil {
				t.Fatal(err)
			}
			if app.selected != "wait" {
				t.Fatal("compact menu did not open the selected session")
			}
		})
	}
}
