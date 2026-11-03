package ui

import (
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The panel's square canvas, the ring the wedges are cut from, the hub, and
// the fans the wedges open. Each is a tunable the view reads every frame, so
// a change shows at once — except floatingSize, which is also the window's
// size (setUpFloating).
const (
	// floatingPizzaOut is the ring's outer radius, and floatingRingWidth its
	// thickness: the slices live in the annulus between the two, not on a
	// circle of buttons.
	floatingPizzaOut  = 150
	floatingRingWidth = 92
	// floatingPizzaIn is the annulus's inner radius, the ring's hole.
	floatingPizzaIn = floatingPizzaOut - floatingRingWidth
	// floatingPizzaPad is the gap between two slices, in radians: chart
	// turns it into a gap of constant width, about 2 DIP here.
	floatingPizzaPad = 0.0125
	// floatingCoverage and floatingCenterAngle are how much of the circle
	// the pie covers and where its first slice starts: a full pie from
	// twelve o'clock, which seven actions fill.
	floatingCoverage    = 1
	floatingCenterAngle = -math.Pi / 2
	// floatingHubSize is the diameter of the centre circle, the pie's
	// middle, which holds the hovered slice's title.
	floatingHubSize = 100
	// floatingIconAt is how far out a slice's icon sits: the middle of the
	// ring, which is the segment's centroid along its middle angle.
	floatingIconAt = (floatingPizzaIn + floatingPizzaOut) / 2
	// floatingSelectorIn and floatingSelectorWidth are the selector band,
	// just inside the ring: it grows out of the ring's inner edge as a slice
	// lights.
	floatingSelectorIn    = floatingPizzaIn - 6
	floatingSelectorWidth = 4
	// floatingDecoratorGap is how far past the ring the thin decorator ring
	// sits, and floatingDecoratorWidth its width.
	floatingDecoratorGap   = 1
	floatingDecoratorWidth = 1
	// floatingSelectOut is how far past the ring the pointer still selects a
	// slice, in ring widths: a pie menu that answers only inside its band
	// feels broken.
	floatingSelectOut = 0.8
	// floatingSelectIn is how far inside the ring the pointer still selects,
	// in ring widths.
	floatingSelectIn = 0.5
	// floatingSliceTarget is the box of the button over a slice's icon: the
	// aim. Seven wedges leave 90 DIP between two icons, so this never
	// overlaps its neighbour.
	floatingSliceTarget = 70
	// floatingFanGap is the air between the pie's rim and a fan's inner edge:
	// the children are pulled out of the pie, and must not touch it.
	floatingFanGap = 3
	// A fan child keeps enough depth for an icon and caption without
	// enlarging the transparent window more than its targets need.
	floatingFanWidth = 46
	// floatingFanIn and floatingFanOut are the fan's annulus: the pie's own
	// shape, pulled out past the rim so the children sit above the pie and
	// never over it. It slides floatingFanLift further out as the fan blooms.
	floatingFanIn  = floatingPizzaOut + floatingFanGap
	floatingFanOut = floatingFanIn + floatingFanWidth
	// floatingFanLift is how far out the children slide as they bloom, in
	// DIPs, on top of the gap: they grow away from the pie rather than
	// appearing at their place.
	floatingFanLift = 2
	// floatingFanPad is the gap between two of a fan's sub-slices, in
	// radians: the pie's own gap width, in DIPs, at the fan's radius. chart
	// turns a pad into a gap of pad*sqrt(in²+out²)/2 each side, so the
	// angle that keeps the width is the pie's own pad times the ratio of
	// the two hypotenuses — the same cut, further out.
	floatingFanPad = floatingPizzaPad * 160.8 / 251.0
	// floatingFanTarget is the box of the button over a sub-slice's icon:
	// the aim, as floatingSliceTarget is a slice's.
	floatingFanTarget = 40
	// floatingMaxSessions is how many sessions the pie shows: one a wedge,
	// and six wedges at most, so a busy source does not crowd the ring.
	floatingMaxSessions = 6
	// floatingBloomTime is how long the slices take to reach the ring.
	floatingBloomTime = 320 * time.Millisecond
	// Submenus answer a quick second click without waiting for a panel spring.
	floatingFanTime = 120 * time.Millisecond
	// floatingSweep is how far the whole pie turns as it opens, in degrees,
	// and floatingOpenScale how small it starts: the sweep is what makes it
	// read as a pie menu rather than a fade.
	floatingSweep     = 18
	floatingOpenScale = 0.86
	// floatingSpin is how far the pie turns as its slices swap for another
	// set: enough to read as a turn, short of a dizzy one.
	floatingSpin = 24
	// floatingShortcut is the global shortcut that summons the panel:
	// Command+Shift+Space on macOS, Control+Shift+Space elsewhere. Bound
	// over every app, it must be one no app needs: Find (Command+F) is
	// every app's.
	floatingShortcut = "CmdOrCtrl+Shift+Space"
	// floatingWedges is how many actions ring the pie: Active sessions,
	// New session, Open folder, Panels, Command palette, Settings,
	// Appearance.
	floatingWedges = 7
	// floatingStep is the angle between two slices: a seventh of the
	// circle, about 51.4°.
	floatingStep = 2 * math.Pi / floatingWedges
)

// Keep the canvas within eight DIPs of the fully open fan. Transparent
// corners still receive clicks on Windows, so surplus canvas blocks the
// desktop even though nothing is painted there.
const floatingSize = 2 * (floatingFanOut + floatingFanLift + 8)

// The menu's levels: actions, servers, projects and sessions.
const (
	floatingActionsMode = iota
	floatingSessionsMode
	floatingHostsMode
	floatingProjectsMode
)

// What the hub says for a source with an empty ring: one word per case, so
// "nothing ever ran here" never reads as "it all ended".
const (
	floatingEmptyNone  = "No active sessions"
	floatingEmptyEnded = "All sessions ended"
)

// floatingNameBudget is how many characters of a session's name the pie
// keeps: enough to tell two sessions apart, short enough for the hub.
// The hub holds two lines; longer names remain available in tooltips.
const (
	floatingNameBudget = 24
	floatingHubLines   = 2
	floatingHubBudget  = 14
)

// The sources the Active sessions fan offers, in its order.
const (
	floatingThisPC = iota
	floatingVPS
)

// The wedges whose click opens a fan, by key: one fan is open at a time.
const (
	floatingActiveFan = "active"
	floatingPanelsFan = "panels"
)

// floatingState is what the panel keeps from frame to frame: which pie
// shows, the source picked (-1 until one is), and which fan is open ("" for
// none). It is a choice for one showing, not a setting.
type floatingState struct {
	mode    int
	source  int
	hostID  string
	project string
	page    int
	history []floatingLocation
	changed bool
	// fan is the key of the wedge whose children are out, so two fans can
	// never be open at once.
	fan string
	// title is the centre circle's word, the hovered slice's, and fade how
	// far it has come over the mark: one value, so the two cross-fade
	// instead of fighting over the same channel.
	title string
	fade  float32
}

// sessions reports whether the pie shows a source's sessions rather than the
// actions.
func (s floatingState) sessions() bool { return s.mode == floatingSessionsMode && s.source >= 0 }

// floatingAction is one wedge of the pie.
type floatingAction struct {
	key, label string
	// short names it in the hub, where the full label would not fit.
	short   string
	svg     *ui.SVG
	enabled bool
	// fan, when not "", is the key of the children this wedge opens instead
	// of running: clicking it unfolds or folds them.
	fan string
	// status, for a session's wedge, is its dot, which colours its fill.
	status   *sessionStatus
	run      func()
	navigate func(*floatingState)
}

// The global shortcut, behind variables so a test can stand in for the
// platform, which needs a running app.
var (
	floatingShortcutsReady     = func() bool { return mygo.App.IsReady() }
	floatingShortcutBound      = func(acc string) bool { return mygo.GlobalShortcut.IsRegistered(acc) }
	registerFloatingShortcut   = func(acc string, fn func()) error { return mygo.GlobalShortcut.Register(acc, fn) }
	unregisterFloatingShortcut = func(acc string) { mygo.GlobalShortcut.Unregister(acc) }
)

// floatingKey is floatingShortcut as the app shows its shortcuts.
var floatingKey = modKey + "⇧Space"
