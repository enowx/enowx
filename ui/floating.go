package ui

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// floatingView is the panel: the pie of actions or of one source's sessions,
// the fans its wedges open, and the centre circle in the middle.
func (a *App) floatingView(c *ui.Context) {
	c.SetTheme(a.floatingTheme())
	// Unconditional: the window carries no material to show through, so
	// anything painted here would be painted over the desktop.
	c.Root().Background(ui.Transparent)
	scale := a.floatingScale()
	panel := ui.Box(c.Key("floating")).Size(floatingSize, floatingSize).TransformOrigin(0, 0).Scale(scale, scale)
	// The pie's state outlives a frame on the panel element, and a test
	// reads it by what it shows.
	st := ui.Local[floatingState](panel, "pie", func() floatingState {
		return floatingState{source: -1}
	})
	// The sources are read fresh every frame: a session that ended or
	// started shows at once, and nothing is cached in the view.
	if c.Shortcut(0, ui.KeyEscape) {
		switch {
		case st.mode != floatingActionsMode:
			st.back()
			a.restartPie(panel)
		case st.fan != "":
			st.fan = "" // the children fold into their wedge first
		default:
			a.hideFloating()
		}
	}
	if a.floatingFresh {
		panel.Jump("open", 0) // sweep out of the centre, rather than appear at once
		// A summon starts on the actions, no source picked, no fan open.
		*st = floatingState{source: -1}
		a.restartPie(panel)
		a.floatingFresh = false
	}
	if st.changed {
		a.restartPie(panel)
		st.changed = false
	}
	target := float32(0)
	if a.floatingShown {
		target = 1
	}
	// A summon springs open and a fold runs straight through: a fold that
	// bounced would read as hesitation.
	open := widget.Spring
	if target == 0 {
		open = ui.Animation{Duration: floatingBloomTime * 7 / 10, Ease: widget.Out}
	}
	t := a.floatingAnimate(panel, "open", target, open)
	// The sweep: the whole pie turns and grows into place, which is what
	// makes it read as a pie menu rather than a fade.
	sweep := a.floatingAnimate(panel, "sweep", target, open)
	fanTarget := float32(0)
	if st.fan != "" && a.floatingShown && st.mode == floatingActionsMode {
		fanTarget = 1
	}
	// One value keeps fan changes continuous; a short ease avoids making
	// repeated menu choices wait for the whole panel's spring to settle.
	f := a.floatingAnimate(panel, "fan", fanTarget, ui.Animation{Duration: floatingFanTime, Ease: widget.Out})
	// A mode change spins the pie into its new slices rather than cutting.
	spin := min(1, a.floatingAnimate(panel, "spin", 1, widget.Spring))
	var acts []floatingAction
	more, empty := 0, ""
	if st.sessions() {
		// The ring shows the sessions still running, but the source's own
		// count decides what an empty ring means: nothing there at all, or
		// everything there ended.
		all := a.floatingLocationSessions(st)
		list := floatingActiveSessions(all)
		if len(list) > floatingMaxSessions {
			more = len(list) - floatingMaxSessions
			list = list[:floatingMaxSessions]
		}
		if len(list) == 0 {
			empty = floatingEmptyNone
			if len(all) > 0 {
				empty = floatingEmptyEnded
			}
		}
		acts = a.floatingSessionWedges(list)
	} else if st.mode == floatingHostsMode || st.mode == floatingProjectsMode {
		acts = a.floatingChoices(st)
		if len(acts) == 0 {
			empty = "No projects"
		}
	} else {
		acts = a.floatingActions()
	}
	panel.Children(func() {
		hovered := a.floatingPizza(c, panel, st, acts, t, sweep, spin)
		fanTitle := ""
		if f > 0.001 {
			_, fanTitle = a.floatingFan(c, panel, st, f, t)
		}
		// One name in the hub, whichever level it comes from: a fan child
		// first, then the slice under the pointer, then what an empty
		// source has to say. A fan child never gets a bubble of its own,
		// so two of them can never collide over the pie.
		name := ""
		switch {
		case fanTitle != "":
			name = fanTitle
		case hovered >= 0 && hovered < len(acts):
			name = acts[hovered].short
		case empty != "":
			name = empty
		}
		a.floatingTitle(panel, st, name)
		a.floatingHubButton(c, panel, st, t, more)
	})
	if !a.floatingShown && t == 0 && a.floating != nil && a.floating.IsVisible() {
		a.floating.Hide() // folded: the window waits, hidden, for the next summon
	}
}

// restartPie starts the pie's spin again, so a mode change sweeps its new
// slices in instead of cutting to them.
func (a *App) restartPie(panel ui.Element) {
	panel.Jump("spin", 0)
	panel.Jump("title", 0)
}

// float01 is b as the 0 or 1 an animation moves between.
func float01(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// floatingHubButton is the centre circle: enowX's mark with the hovered
// slice's name over it, which folds the panel away — or, on a source's
// sessions, the way back to the actions.
func (a *App) floatingHubButton(c *ui.Context, panel ui.Element, st *floatingState, t float32, more int) {
	label, tip, svg := "Hide the floating control", "Hide the floating control", icon.Logo
	if st.mode != floatingActionsMode {
		label, tip, svg = "Back", "Back", icon.ArrowLeft
		if more > 0 {
			// The pie holds six: say what it left out.
			tip += " · " + strconv.Itoa(more) + " more"
		}
	}
	b := widget.Float(ui.ButtonBase(c.Key("hub")).Absolute().
		Left(floatingSize/2-floatingHubSize/2).Top(floatingSize/2-floatingHubSize/2).
		Size(floatingHubSize, floatingHubSize).Radius(floatingHubSize/2).Center().
		Background(a.floatingSurface()).Border(1, theme.BorderStrong).TextColor(theme.TextStrong).
		Opacity(t).Label(label).Tooltip(tip).Cursor(ui.CursorPointer))
	b.Children(func() {
		// The mark, which the title fades over: nothing hovered leaves it
		// showing, as the pie's own face.
		mark := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().PassThrough()
		mark.Children(func() {
			ui.Icon(c, svg).FontSize(28).TextColor(a.floatingAccent()).Opacity(1 - st.fade)
		})
		if st.title != "" {
			// A fixed box inside the circle, not a fill: MaxWidth and
			// MaxLines need a width to resolve against, and an absolute
			// fill has none.
			const box = floatingHubSize - 8
			title := ui.Box(c).Absolute().
				Left(floatingHubSize/2-box/2).Top(floatingHubSize/2-box/2).
				Size(box, box).Center().PassThrough().
				Opacity(st.fade)
			title.Children(func() {
				// A session's name is clipped to
				// what fits before it gets here (floatingClip), and the
				// text is held to two lines with an ellipsis inside a box
				// narrower than the circle, so neither a long name nor a
				// source's message can spill out of it.
				ui.Text(c, floatingClip(st.title, floatingHubBudget*floatingHubLines)).
					FontSize(11).TextColor(theme.TextStrong).
					MaxWidth(box - 4).MaxLines(floatingHubLines).
					TextAlign(ui.Center)
			})
		}
	})
	if b.Clicked() {
		if st.mode != floatingActionsMode {
			st.back()
			a.restartPie(panel) // the actions sweep back in
			return
		}
		a.hideFloating()
	}
}
