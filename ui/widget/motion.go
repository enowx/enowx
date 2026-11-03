package widget

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// The motion of the app. Colors ease out fast and land softly; things that
// move or resize ride a spring with a trace of bounce, so a change that
// interrupts another carries on from where it was instead of restarting.
// MyGo honors the desktop's Reduce Motion: springs and moves jump, colors
// still ease.
var (
	// Out is a quint ease-out: quick to start, long to settle.
	Out = ui.CubicBezier(0.22, 1, 0.36, 1)
	// Spring moves and resizes panels, pills and pop-ups.
	Spring = ui.Animation{Spring: ui.SpringOptions{Duration: 460 * time.Millisecond, Bounce: 0.12}}
	// Press is the quick spring of a pressed control.
	Press = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.2}}
)

// ease is how every control's colors change.
var ease = ui.Animation{Duration: 220 * time.Millisecond, Ease: Out}

// Pressable shrinks e a little while pressed and springs it back.
func Pressable(e ui.Element) ui.Element {
	scale := float32(1)
	if e.Pressed() && !e.IsDisabled() {
		scale = 0.965
	}
	// Fade 0 is opaque: a Pose fades from 0 to 1, gone.
	return e.AnimatePose(ui.Pose{Scale: scale}, Press)
}

// pill tells apart the selection pills of different lists.
type pill struct{ group string }

// SelectionPill marks the selected row. The pills of one group are the
// same element to MyGo: selecting another row slides the pill there. An
// absolute element draws above its siblings, so the pill is a see-through
// tint the row's text shows through, and lets the pointer pass.
func SelectionPill(c *ui.Context, row ui.Element, group string, radius float32) {
	if theme.Picked.A == 255 {
		// What is absolute lies above its siblings: over the row's words, a pill that
		// can't be seen through hides them. A solid one is the row's own ground.
		row.Background(theme.Picked)
		return
	}
	ui.Box(c.Key("pill")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Radius(radius).PassThrough().
		Background(theme.Picked).
		Transition(ui.ElementTransition{Animation: &Spring, Match: pill{group}})
}

// PanelMotion is a side panel's: it slides in from its edge and folds
// away, while what is beside it moves with LayoutMotion.
func PanelMotion(fromX float32) ui.ElementTransition {
	m := &ui.Motion{X: fromX, Opacity: 0, Collapse: true}
	return ui.ElementTransition{Animation: &Spring, Enter: m, Exit: m}
}

// QuickMotion moves and resizes a small control at once, near enough:
// for what the user clicked, which should answer without a glide.
var QuickMotion = ui.ElementTransition{Animation: &ui.Animation{Duration: 140 * time.Millisecond, Ease: Out}, Position: true, Size: true}

// quick eases a small control's colors as fast.
var quick = ui.Animation{Duration: 120 * time.Millisecond, Ease: Out}

// QuickTint eases e's color named key toward target at QuickMotion's pace.
func QuickTint(e ui.Element, key string, target ui.Color) ui.Color {
	return e.AnimateColor(key, target, quick)
}

// LayoutMotion moves and resizes an element in step with its neighbours.
var LayoutMotion = ui.ElementTransition{Animation: &Spring, Position: true, Size: true}

// PageMotion is a view's as it replaces another: it rises a few DIPs as
// it fades in.
var PageMotion = ui.ElementTransition{Duration: 340 * time.Millisecond, Ease: Out, Enter: &ui.Motion{Y: 10, Opacity: 0}}

// OverlayMotion is a pop-up's or dialog's: it grows from a little smaller
// as it fades in, and fades out a touch smaller.
var OverlayMotion = ui.ElementTransition{
	Animation: &Spring,
	Enter:     &ui.Motion{Scale: 0.95, Y: -4, Opacity: 0},
	Exit:      &ui.Motion{Scale: 0.97, Opacity: 0},
}

// RowMotion is a list row's as it appears: it slides in a little later
// than the one above it.
func RowMotion(i int) ui.ElementTransition {
	delay := time.Duration(min(i, 12)) * 18 * time.Millisecond
	return ui.ElementTransition{
		Animation: &ui.Animation{Duration: 320 * time.Millisecond, Ease: Out, Delay: delay},
		Enter:     &ui.Motion{Y: 6, Opacity: 0},
		Position:  true,
	}
}
