package canvasview

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A moving design plays by the clock in its page (core/canvas/motion.js). In
// Interact mode a bar plays, pauses, restarts and moves through it. It isn't a
// timeline editor: what moves is the design's own code.

// motionBarRoom is what Interact mode leaves under a moving design for its bar.
const motionBarRoom = 44

// pageMotion is what a page says of its motion, in milliseconds: its length, the
// moment it is at, whether it plays.
type pageMotion struct {
	D, T float64
	P    bool
}

// motionControl is the bar's state. All of it is the main thread's; Evals run off
// it, one at a time, the latest order waiting.
type motionControl struct {
	motion   *pageMotion // nil until the page says; D 0 when it has nothing to move
	motionAt time.Time   // when motion was told
	scrub    float64     // the scrubber's value
	scrubbed bool        // the user holds it
	ordering bool        // an order is in the page
	nextEval string      // the order to send after it
}

// setMotion takes what the page said, keeping what the user is doing with it.
func (w *Web) setMotion(m *pageMotion) {
	if m != nil && w.scrubbed && w.motion != nil {
		m.T, m.P = w.motion.T, false
	}
	w.motion, w.motionAt = m, time.Now()
}

// motionTime is where the motion is now, carried on since the page last said.
func (w *Web) motionTime() float64 {
	m := w.motion
	if m == nil {
		return 0
	}
	if !m.P {
		return m.T
	}
	return min(m.T+float64(time.Since(w.motionAt).Milliseconds()), m.D)
}

// orderMotion runs code in the page. Only the last of what comes while one runs
// is sent: a scrub asks far faster than a page answers.
func (w *Web) orderMotion(code string) {
	if w.page == nil {
		return
	}
	if w.ordering {
		w.nextEval = code
		return
	}
	w.ordering = true
	page := w.page
	go func() {
		_, _ = page.Eval(code)
		w.Update(func() {
			w.ordering = false
			if next := w.nextEval; next != "" {
				w.nextEval = ""
				w.orderMotion(next)
			}
		})
	}()
}

func (w *Web) playMotion() {
	if t := w.motionTime(); t >= w.motion.D {
		w.motion.T = 0
	} else {
		w.motion.T = t
	}
	w.motion.P, w.motionAt = true, time.Now()
	w.orderMotion("enowx.motion.play()")
}

func (w *Web) pauseMotion() {
	w.motion.T, w.motion.P = w.motionTime(), false
	w.orderMotion("enowx.motion.pause()")
}

func (w *Web) restartMotion() {
	w.motion.T, w.motion.P, w.motionAt = 0, true, time.Now()
	w.orderMotion("enowx.motion.restart()")
}

// seekMotion moves to ms, paused: a picture held still while it is dragged.
func (w *Web) seekMotion(ms float64) {
	w.motion.T, w.motion.P = ms, false
	w.orderMotion(fmt.Sprintf("enowx.motion.pause();enowx.motion.seek(%g)", ms))
}

// motionBar draws the controls at the foot of a stage aw×ah, once the page says
// it has motion.
func (w *Web) motionBar(c *ui.Context, aw, ah float32) {
	m := w.motion
	if m == nil || m.D <= 0 {
		return
	}
	width := min(aw-32, 520)
	ui.Row(c.Key("motion-bar")).Absolute().Left((aw-width)/2).Top(ah-motionBarRoom+4).Width(width).Height(32).
		PaddingX(6).Gap(6).AlignItems(ui.Center).Background(theme.Panel).Radius(theme.RadiusControl).BorderWidth(1, 1, 1, 1).BorderColor(theme.Border).Children(func() {
		if m.P {
			if widget.IconButton(c.Key("motion-pause"), icon.Pause, "Pause", false).Clicked() {
				w.pauseMotion()
			}
		} else if widget.IconButton(c.Key("motion-play"), icon.Play, "Play", false).Clicked() {
			w.playMotion()
		}
		if widget.IconButton(c.Key("motion-restart"), icon.Undo, "Restart", false).Clicked() {
			w.restartMotion()
		}
		if !w.scrubbed {
			w.scrub = w.motionTime()
		}
		was := w.scrub
		slider := ui.Slider(c.Key("motion-scrub"), &w.scrub, 0, m.D).Grow(1).Label("Move through the animation")
		w.scrubbed = slider.Dragging()
		if w.scrub != was {
			w.seekMotion(w.scrub)
		}
		ui.Text(c.Key("motion-time"), fmt.Sprintf("%.1f / %.1f s", w.scrub/1000, m.D/1000)).FontSize(theme.TextLabel).TextColor(theme.Muted).Shrink(0)
	})
	if m.P {
		c.After(50 * time.Millisecond) // the scrubber moves on
	}
}
