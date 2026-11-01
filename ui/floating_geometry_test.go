package ui

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// Clicking anywhere in a slice's sector picks it, not only its icon's 44 DIP
// box: the sector is the button. The pointer goes down on the slice's outer
// edge, well outside the icon, and the session must be the one selected.
func TestFloatingSliceSectorClicks(t *testing.T) {
	app, tt := floatingWithSessions(t)
	if err := tt.Click("Active sessions"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(floatingBloomTime)
	if err := tt.Click("This PC"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	// Slice 1 of the sessions pie, at the ring's outer edge: far from the
	// icon's box at floatingIconAt, inside the selector's reach.
	acts := app.floatingSessionWedges(app.floatingSessions(floatingThisPC))
	if len(acts) < 2 {
		t.Fatalf("%d session wedges, want at least two", len(acts))
	}
	pie := floatingSlicesFor(len(acts))
	i := 1
	angle := floatingMidOf(pie, i)
	r := float64(floatingPizzaOut) - 6 // inside the ring, past the icon box
	x := float32(float64(floatingSize)/2 + math.Cos(angle)*r)
	y := float32(float64(floatingSize)/2 - math.Sin(angle)*r)
	// The icon box must not cover this point, or the test proves nothing.
	ix := float64(floatingSize)/2 + math.Cos(angle)*floatingIconAt
	iy := float64(floatingSize)/2 - math.Sin(angle)*floatingIconAt
	if d := math.Hypot(float64(x)-ix, float64(y)-iy); d < floatingSliceTarget/2+2 {
		t.Fatalf("the point is %0.f DIP from the icon, inside its box", d)
	}

	tt.ClickAt(x, y)
	if app.floatingShown {
		t.Fatal("a click on the slice's sector left the panel up")
	}
	if want := acts[i].key; app.selected != strings.TrimPrefix(want, "session:") {
		t.Fatalf("selected %q, want the session of %q", app.selected, want)
	}
}

// The aim boxes never overlap: the arc between two neighbours is wider than
// the boxes that sit on them, for the pie and for both fans.
func TestFloatingAimBoxesClear(t *testing.T) {
	// The pie: seven slices, icons at floatingIconAt.
	arc := float64(floatingIconAt) * 2 * math.Pi / floatingWedges
	if arc <= floatingSliceTarget {
		t.Fatalf("two slices' icons are %0.1f DIP apart, want more than the %d target", arc, floatingSliceTarget)
	}
	// A fan: one wedge's span divided among its children, at the fan's
	// middle radius. Two children is the Active sessions fan; three is
	// Panels.
	mid := float64(floatingFanIn+floatingFanOut) / 2
	for _, n := range []int{2, 3} {
		gap := mid * floatingStep / float64(n)
		if gap <= floatingFanTarget {
			t.Fatalf("a %d-child fan leaves %0.1f DIP between icons, want more than the %d target", n, gap, floatingFanTarget)
		}
	}
}

// Every fill the pie paints as a body is opaque: the slices, the fan's
// sub-slices, and the hub. Only what the pie does not paint shows the desktop,
// so a hover, a status tint, a dimmed action and an open fan must all keep
// A == 255. Reaching for theme.Fade on a slice fails here.
func TestFloatingSlicesAreOpaque(t *testing.T) {
	app := New(Options{})
	st := &floatingState{source: -1}
	// A disabled action and an enabled one, both hovers, both statuses.
	acts := []floatingAction{
		{key: "enabled", label: "Enabled", enabled: true},
		{key: "disabled", label: "Disabled", enabled: false},
	}
	status := statusOf(proto.SessionInfo{State: proto.StateWaiting})
	acts = append(acts,
		floatingAction{key: "waiting", label: "Waiting", enabled: true, status: &status},
		floatingAction{key: "waiting-off", label: "Waiting off", enabled: false, status: &status},
	)
	for _, act := range acts {
		for _, hover := range []float32{0, 0.5, 1} {
			for _, fan := range []string{"", floatingActiveFan} {
				st.fan = fan
				if got := app.sliceFill(st, act, hover); got.A != 255 {
					t.Fatalf("%q at hover %v, fan %q: alpha %d, want 255", act.key, hover, fan, got.A)
				}
			}
		}
	}
	// The fan's own children, dimmed and lit.
	for _, act := range []floatingAction{{key: "on", enabled: true}, {key: "off", enabled: false}} {
		for _, hover := range []float32{0, 1} {
			if got := app.floatingFanFill(act, hover); got.A != 255 {
				t.Fatalf("fan %q at hover %v: alpha %d, want 255", act.key, hover, got.A)
			}
		}
	}
	// The hub's surface, which the title sits on.
	if got := app.floatingSurface(); got.A != 255 {
		t.Fatalf("the hub's fill has alpha %d, want 255", got.A)
	}
	// A disabled slice is still visibly dimmer than an enabled one: dim by
	// colour, not by alpha.
	on := app.sliceFill(st, acts[0], 0)
	off := app.sliceFill(st, acts[1], 0)
	if on == off {
		t.Fatal("a disabled slice is not dimmed at all")
	}
	if luma(off) >= luma(on) {
		t.Fatalf("a disabled slice is not darker: %v vs %v", off, on)
	}
}

// luma is a colour's brightness, for comparing two fills.
func luma(c ui.Color) float64 {
	return 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
}

// The window the control lives in carries the options that keep it invisible
// chrome: a Panel (the only thing that puts it out of Alt-Tab and the taskbar,
// and above every app), frameless, transparent, no system shadow, no taskbar
// button. This pins the options the app actually passes, so re-adding or
// dropping any of them fails here rather than in a screenshot.
func TestFloatingWindowOptions(t *testing.T) {
	app := New(Options{})
	opts := floatingWindowOptions(app)
	if !opts.Panel {
		t.Fatal("not a Panel: the window then shows in Alt-Tab and gets a taskbar button")
	}
	if opts.AlwaysOnTop {
		t.Fatal("AlwaysOnTop is redundant: Panel already makes it topmost")
	}
	if !opts.Frameless {
		t.Fatal("a title bar and borders would show around the pie")
	}
	if !opts.Transparent {
		t.Fatal("an opaque window paints the square the pie sits in")
	}
	if !opts.DisableShadow {
		t.Fatal("the system shadow hugs the square the window is, not the slices")
	}
	if !opts.SkipTaskbar {
		t.Fatal("the control gets a taskbar button")
	}
	if opts.Width != floatingSize || opts.Height != floatingSize {
		t.Fatalf("the window is %dx%d, want the pie's canvas %d", opts.Width, opts.Height, floatingSize)
	}
	if opts.Vibrancy != mygo.VibrancyMica {
		t.Fatal("no material at creation: Windows gives the window no per-pixel alpha")
	}
	if opts.Content == nil {
		t.Fatal("no view: the window shows nothing")
	}
}

// A pointer just past the rim still picks the slice under it: a pie menu that
// answers only inside its band feels broken.
func TestFloatingSelectsPastTheRim(t *testing.T) {
	app := New(Options{})
	tt := floatingTester(t, app)
	// The first slice is at the top: straight up from the middle, past the
	// ring's outer edge but within the selector's reach.
	angle := math.Pi / 2
	rad := float64(floatingPizzaOut) + floatingSelectOut*floatingRingWidth*0.6
	x := float32(float64(floatingSize)/2 + math.Cos(angle)*rad)
	y := float32(float64(floatingSize)/2 - math.Sin(angle)*rad)
	tt.Move(x, y)
	tt.Advance(floatingBloomTime)
	if !tt.HasText("Active sessions") {
		t.Fatalf("the slice past the rim doesn't answer: %q", tt.Texts())
	}
}

// The pie's slices are equal and its gaps real: seven shares, one pad, and a
// gap of transparency between two of them.
func TestFloatingPieGeometry(t *testing.T) {
	pie := floatingSlicesFor(floatingWedges)
	if len(pie) != floatingWedges {
		t.Fatalf("%d slices, want %d", len(pie), floatingWedges)
	}
	for i, s := range pie {
		if s.Pad != floatingPizzaPad {
			t.Fatalf("slice %d pads %v, want %v", i, s.Pad, floatingPizzaPad)
		}
		if d := s.End - s.Start; math.Abs(d-floatingStep) > 1e-6 {
			t.Fatalf("slice %d covers %.4f rad, want %.4f", i, d, floatingStep)
		}
	}
	// The first starts at twelve o'clock, the panel's own convention.
	if first := floatingMidOf(pie, 0); math.Abs(first-math.Pi/2) > 1e-6 {
		t.Fatalf("the first slice points %.4f rad, want the top", first)
	}
	// And the annulus has the ring's thickness, not a full disc.
	if got := floatingPizzaOut - floatingPizzaIn; got != floatingRingWidth {
		t.Fatalf("the ring is %d thick, want %d", got, floatingRingWidth)
	}
	// The gap is real width, not a hairline: chart turns the pad into
	// pad*sqrt(in²+out²) at the ring's edges, and the sector's own inset
	// keeps half of it each side.
	mid := float64(floatingIconAt)
	gap := floatingPizzaPad * mid
	if gap < 1 || gap > 2 {
		t.Fatalf("the gap is %0.1f DIP at the ring's middle, want 1 to 2", gap)
	}
	// The selector band sits inside the ring, and never covers a slice.
	if floatingSelectorIn+floatingSelectorWidth > floatingPizzaIn {
		t.Fatal("the selector band reaches into the ring")
	}
}
