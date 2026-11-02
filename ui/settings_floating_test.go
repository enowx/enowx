package ui

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// floatingSectionFrame renders a section in a window the tests click in.
func floatingSectionFrame(t *testing.T, app *App, section func(c *ui.Context)) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(720).Height(900).Padding(20).Gap(16).Background(theme.Background).Children(func() {
			section(c)
		})
	}, 720, 900)
	tt.Frame()
	return tt
}

// The section says what the panel is, and its switch turns the feature on.
func TestFloatingSectionRendersAndSwitches(t *testing.T) {
	app := New(Options{})
	tt := floatingSectionFrame(t, app, app.floatingSection)
	if !tt.HasText("A radial panel of the main actions") {
		t.Fatalf("no intro in %q", tt.Texts())
	}
	if !tt.HasText("Shortcut") {
		t.Fatalf("no shortcut row in %q", tt.Texts())
	}
	if !tt.HasText(floatingKey) {
		t.Fatalf("no shortcut value in %q", tt.Texts())
	}
	// The switch is the button labeled "Floating control"; the intro is a
	// text, so the click has to land on the switch and not the paragraph.
	if err := tt.Click("Floating control switch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.state.FloatingControl {
		t.Fatalf("the switch did not turn the floating control on: %q", tt.Texts())
	}
	if err := tt.Click("Floating control switch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.state.FloatingControl {
		t.Fatalf("the switch did not turn the floating control off")
	}
}

// The two behaviours default to on, and the switches save what they are set
// to, both ways.
func TestFloatingSectionBehaviours(t *testing.T) {
	app := New(Options{})
	if !app.floatingFoldOnBlur() || !app.floatingAtPointer() {
		t.Fatalf("the behaviours do not default to on: %v, %v", app.floatingFoldOnBlur(), app.floatingAtPointer())
	}
	tt := floatingSectionFrame(t, app, app.floatingSection)
	if err := tt.Click("Fold when you go elsewhere switch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.state.FloatingFoldOnBlur == nil || *app.state.FloatingFoldOnBlur {
		t.Fatalf("folding was not turned off: %v", app.state.FloatingFoldOnBlur)
	}
	if app.floatingFoldOnBlur() {
		t.Fatal("the helper still reports folding on")
	}
	if err := tt.Click("Fold when you go elsewhere switch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.state.FloatingFoldOnBlur == nil || !*app.state.FloatingFoldOnBlur {
		t.Fatalf("folding was not turned back on: %v", app.state.FloatingFoldOnBlur)
	}
	if err := tt.Click("Show at the pointer switch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.state.FloatingAtPointer == nil || *app.state.FloatingAtPointer {
		t.Fatalf("the pointer was not turned off: %v", app.state.FloatingAtPointer)
	}
	if err := tt.Click("Show at the pointer switch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.state.FloatingAtPointer == nil || !*app.state.FloatingAtPointer {
		t.Fatalf("the pointer was not turned back on: %v", app.state.FloatingAtPointer)
	}
}

// The switch left App: its row is the Floating control section's now.
func TestSettingsAppSectionHasNoFloatingControl(t *testing.T) {
	app := New(Options{})
	tt := floatingSectionFrame(t, app, app.appSection)
	// Its shortcut stays listed there, so look for the switch and its words.
	if _, on := tt.Find("Floating control switch"); on || tt.HasText("The panel and its shortcut") {
		t.Fatalf("App still shows the floating control: %q", tt.Texts())
	}
}

// App carries none of the rows that are not the original's: the Sessions
// panel's switch is the Floating control section's, and it must not come back
// here.
func TestSettingsAppSectionHasNoHideSessions(t *testing.T) {
	app := New(Options{})
	tt := floatingSectionFrame(t, app, app.appSection)
	if tt.HasText("Hide the sidebar") {
		t.Fatalf("App still shows the sidebar's switch: %q", tt.Texts())
	}
	// The setting itself still lives, in the section that owns it.
	tt = floatingSectionFrame(t, app, app.floatingSection)
	if !tt.HasText("Hide the sidebar") {
		t.Fatalf("the Floating control section lost the switch: %q", tt.Texts())
	}
	if err := tt.Click("Hide the sidebar switch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.state.HideSessions {
		t.Fatal("the switch does not set state.HideSessions")
	}
	if !app.sessionsHidden() {
		t.Fatal("sessionsHidden does not follow the switch")
	}
}

// The section is dispatched: settingsBody draws it when it is picked.
func TestSettingsBodyDrawsTheFloatingSection(t *testing.T) {
	app := New(Options{})
	app.settingsSection = sectionFloating
	tt := floatingSectionFrame(t, app, app.settingsBody)
	if !tt.HasText("Fold when you go elsewhere") || !tt.HasText("Show at the pointer") {
		t.Fatalf("the section is not drawn: %q", tt.Texts())
	}
}

// Floating control sits right after App in the segmented control, and
// nothing indexes the slice positionally.
func TestFloatingSectionFollowsApp(t *testing.T) {
	if got, want := slices.Index(settingsSections(), sectionFloating), slices.Index(settingsSections(), sectionApp)+1; got != want {
		t.Fatalf("floating is at %d, App at %d", got, want)
	}
}
