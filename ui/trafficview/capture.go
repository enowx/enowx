package trafficview

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Pausing and stopping the recording: the toolbar's state and buttons
// (⌘⌥P pauses or resumes, ⌘⌥S stops or starts), the gaps in the list,
// and the preferences in the settings.

// captureState is how the host records, and the preferences for pausing.
type captureState struct {
	st          proto.TrafficCapture
	loaded      bool
	busy        bool
	menu        widget.ContextMenu
	prefs       proto.TrafficCapturePrefs
	prefsLoaded bool
	minutes     string
	count       string
}

// SetCapture records how the host records, told by an event.
func (v *View) SetCapture(st proto.TrafficCapture) { v.capture.st, v.capture.loaded = st, true }

// Capture is how the host records, as last told.
func (v *View) Capture() proto.TrafficCapture { return v.capture.st }

// loadCapture reads how the host records, and the preferences, once.
func (v *View) loadCapture(o Options) {
	cs := &v.capture
	if cs.loaded || cs.busy || o.Host == nil {
		return
	}
	cs.busy = true
	h := o.Host
	go func() {
		st, err := h.TrafficCapture()
		prefs, perr := h.TrafficCapturePrefs()
		o.Update(func() {
			cs.busy, cs.loaded = false, true
			if err != nil {
				cs.st = proto.TrafficCapture{State: proto.CaptureRecording} // a daemon of before
				return
			}
			cs.st = st
			if perr == nil {
				cs.setPrefs(prefs)
			}
		})
	}()
}

func (cs *captureState) setPrefs(p proto.TrafficCapturePrefs) {
	cs.prefs, cs.prefsLoaded = p, true
	cs.minutes, cs.count = numText(p.AutoMinutes), numText(p.AutoCount)
}

// setCapture asks the host to pause, resume, stop or start.
func (v *View) setCapture(o Options, p proto.TrafficCaptureParams) {
	cs := &v.capture
	if cs.busy || o.Host == nil {
		return
	}
	cs.busy = true
	h := o.Host
	go func() {
		st, err := h.SetTrafficCapture(p)
		o.Update(func() {
			cs.busy = false
			if err != nil {
				v.notice = client.Explain(err)
				return
			}
			cs.st = st
		})
	}()
}

// captureLabel is the state in a few words, and its color: recording
// red, paused amber, stopped grey.
func captureLabel(st proto.TrafficCapture) (string, ui.Color) {
	switch st.State {
	case proto.CapturePaused:
		label := "Paused"
		if len(st.Sources) > 0 || st.Filter != "" {
			label = "Partly paused"
		}
		if st.Skipped > 0 {
			label += fmt.Sprintf(" · %d not recorded", st.Skipped)
		}
		return label, theme.Warning
	case proto.CaptureStopped:
		label := "Stopped"
		if st.Skipped > 0 {
			label += fmt.Sprintf(" · %d not recorded", st.Skipped)
		}
		return label, theme.Muted
	}
	return "Recording", theme.Error
}

// captureBar is the toolbar's recording state, Pause or Resume (with a
// menu of what to pause), and Stop or Start.
func (v *View) captureBar(c *ui.Context, o Options) {
	v.loadCapture(o)
	cs := &v.capture
	if !cs.loaded {
		return
	}
	st := cs.st
	label, col := captureLabel(st)
	tip := "Recording what is attached"
	switch st.State {
	case proto.CapturePaused:
		tip = "Paused since " + st.Since.Local().Format("15:04") + ": traffic still flows, unrecorded"
		if st.Rules == "pause" {
			tip += ", rules paused too"
		}
		if st.Reason != "" {
			tip += " (" + st.Reason + ")"
		}
	case proto.CaptureStopped:
		tip = "Stopped since " + st.Since.Local().Format("15:04") + ": traffic goes through untouched"
	}
	ui.Row(c).Height(24).PaddingX(9).Gap(6).Radius(theme.RadiusSmall).Border(1, theme.Border).AlignItems(ui.Center).Tooltip(tip).Label(label).Children(func() {
		widget.Dot(c, col, st.State == proto.CaptureStopped, 7)
		ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
	})
	pauseShortcut := c.Shortcut(ui.Cmd|ui.Alt, ui.KeyP)
	stopShortcut := c.Shortcut(ui.Cmd|ui.Alt, ui.KeyS)
	switch st.State {
	case proto.CapturePaused:
		if widget.IconButton(c, icon.Play, "Resume recording (⌘⌥P)", false).Clicked() || pauseShortcut {
			v.setCapture(o, proto.TrafficCaptureParams{Action: proto.CaptureResume})
		}
	case proto.CaptureRecording, "":
		b := widget.IconButton(c, icon.Pause, "Pause recording (⌘⌥P)", false)
		cs.menu.OpenBelow(b, "capture", 28)
		cs.menu.Show(c, b, "capture", func(done func()) { v.pauseItems(c, o, done) })
		if pauseShortcut {
			v.setCapture(o, proto.TrafficCaptureParams{Action: proto.CapturePause})
		}
	}
	if st.State == proto.CaptureStopped {
		if widget.IconButton(c, icon.Play, "Start catching again (⌘⌥S)", false).Clicked() || stopShortcut {
			v.setCapture(o, proto.TrafficCaptureParams{Action: proto.CaptureStart})
		}
	} else if widget.IconButton(c, icon.Stop, "Stop catching: traffic goes through untouched (⌘⌥S)", false).Clicked() || stopShortcut {
		v.setCapture(o, proto.TrafficCaptureParams{Action: proto.CaptureStop})
	}
}

// pauseItems are what Pause offers: everything, or part of the traffic.
func (v *View) pauseItems(c *ui.Context, o Options, done func()) {
	pause := func(p proto.TrafficCaptureParams) {
		p.Action = proto.CapturePause
		v.setCapture(o, p)
		done()
	}
	if widget.MenuItem(c, icon.Pause, "Pause everything", false) {
		pause(proto.TrafficCaptureParams{})
	}
	if s := o.Session.ID; s != "" && widget.MenuItem(c, icon.Terminal, "Pause this session's", false) {
		pause(proto.TrafficCaptureParams{Sources: []string{s}})
	}
	if widget.MenuItem(c, icon.Browser, "Pause browsers'", false) {
		pause(proto.TrafficCaptureParams{Sources: []string{"browser"}})
	}
	if f := strings.TrimSpace(v.filter); f != "" && widget.MenuItemTip(c, icon.Search, "Pause matches", "Pause recording what the filter matches", false) {
		pause(proto.TrafficCaptureParams{Filter: f})
	}
	widget.MenuSeparator(c)
	if widget.MenuItem(c, icon.Pause, "Pause rules too", false) {
		pause(proto.TrafficCaptureParams{Rules: "pause"})
	}
}

// gapRows are the rows the list shows: the exchanges shown (their
// indexes in shown), with the pauses and stops between them (-1 minus
// their index in the gaps, the one going on last).
func (v *View) gapRows() []int {
	gaps := v.gaps()
	rows := make([]int, 0, len(v.shown)+len(gaps))
	g := 0
	for i, li := range v.shown {
		for g < len(gaps) && !gaps[g].To.IsZero() && !v.list[li].Started.Before(gaps[g].To) {
			rows = append(rows, -1-g)
			g++
		}
		rows = append(rows, i)
	}
	for ; g < len(gaps); g++ {
		rows = append(rows, -1-g)
	}
	return rows
}

// gaps are the pauses and stops to show: those before, and the one going
// on (To zero).
func (v *View) gaps() []proto.TrafficGap {
	st := v.capture.st
	gaps := st.Gaps
	if st.State == proto.CapturePaused || st.State == proto.CaptureStopped {
		gaps = append(gaps[:len(gaps):len(gaps)], proto.TrafficGap{State: st.State, From: st.Since, Skipped: st.Skipped, Partial: len(st.Sources) > 0 || st.Filter != ""})
	}
	return gaps
}

// gapRow is a pause or stop in the list.
func gapRow(c *ui.Context, g proto.TrafficGap) {
	what := g.State
	if g.Partial {
		what = "partly " + what
	}
	text := "— " + what + " " + g.From.Local().Format("15:04")
	if g.To.IsZero() {
		text += " until now"
	} else {
		text += "–" + g.To.Local().Format("15:04")
	}
	if g.Skipped > 0 {
		text += fmt.Sprintf(" · %d not recorded", g.Skipped)
	}
	text += " —"
	ui.Row(c).FillWidth().Height(26).Center().Children(func() {
		ui.Text(c, text).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
	})
}

// capturePrefsFields are the preferences for pausing, in the settings.
func (v *View) capturePrefsFields(c *ui.Context) {
	cs := &v.capture
	if !cs.prefsLoaded {
		return
	}
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Text(c, "Pausing").FontSize(theme.TextUI).FontWeight(600)
		keep := cs.prefs.Rules != "pause"
		check(c, &keep, "Rules (mocks, forwards, failures) keep running while recording is paused")
		cs.prefs.Rules = map[bool]string{true: "keep", false: "pause"}[keep]
		release := cs.prefs.Breakpoints != "drop"
		check(c, &release, "Let requests held at a breakpoint go on when pausing or stopping (else drop them)")
		cs.prefs.Breakpoints = map[bool]string{true: "release", false: "drop"}[release]
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			ui.Column(c).Grow(1).Children(func() {
				widget.Field(c, "Pause on its own after (minutes)", func() { widget.Input(c, &cs.minutes, "never").FillWidth() })
			})
			ui.Column(c).Grow(1).Children(func() {
				widget.Field(c, "or after (exchanges)", func() { widget.Input(c, &cs.count, "never").FillWidth() })
			})
		})
		ui.Text(c, "These are yours on every host; whether a host records now is its own.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	})
}

// saveCapturePrefs saves the preferences typed, with the settings.
func (v *View) saveCapturePrefs(o Options) error {
	cs := &v.capture
	if !cs.prefsLoaded {
		return nil
	}
	p := cs.prefs
	var err error
	if p.AutoMinutes, err = parseCount(cs.minutes, "Minutes"); err != nil {
		return err
	}
	if p.AutoCount, err = parseCount(cs.count, "Exchanges"); err != nil {
		return err
	}
	h := o.Host
	go func() {
		saved, err := h.SetTrafficCapturePrefs(p)
		o.Update(func() {
			if err != nil {
				v.notice = client.Explain(err)
				return
			}
			cs.setPrefs(saved)
		})
	}()
	return nil
}

func parseCount(s, what string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "never" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s: a whole number, or empty for never", what)
	}
	return n, nil
}
