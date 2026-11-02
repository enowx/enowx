// Package checkview is the side panel's Checks tool: the commands that
// tell whether a session's work holds up (tests, linters, build), run in
// the session's folder by its daemon, with how each last went and its
// output.
package checkview

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// How often a shown view asks how the checks go: often while one runs.
const (
	pollRunning = time.Second
	pollIdle    = 5 * time.Second
	// outputLines is how much of a check's output shows: its end.
	outputLines = 300
)

// Options connect a view to the app.
type Options struct {
	Client  *client.Client
	Session proto.SessionInfo
	// Update runs a function on the main thread; Wanted, called there,
	// reports whether the view still shows, else its poller stops.
	Update func(func())
	Wanted func() bool
	// OpenOutput shows a check's output, by its name, in the center; Current
	// is the check shown there, "" for none.
	OpenOutput func(name string)
	Current    func() string
}

// View is one session's checks. It belongs to the main thread; its poller
// reports through Options.Update.
type View struct {
	info    proto.ChecksInfo
	loaded  bool
	err     string
	polling bool
	poke    chan struct{}
	edit    *editForm
}

type editForm struct {
	open bool
	rows []proto.Check
}

// New returns a view with nothing read.
func New() *View { return &View{poke: make(chan struct{}, 1)} }

// Poke asks the view's poller to ask now: a check started or ended.
func (v *View) Poke() {
	select {
	case v.poke <- struct{}{}:
	default:
	}
}

func (v *View) running() bool {
	for _, r := range v.info.Runs {
		if r.State == proto.CheckRunning || r.State == proto.CheckQueued {
			return true
		}
	}
	return false
}

func (v *View) follow(o Options) {
	if v.polling || o.Client == nil {
		return
	}
	v.polling = true
	cl, id := o.Client, o.Session.ID
	go func() {
		for {
			info, err := cl.Checks(id)
			keep := make(chan time.Duration, 1)
			o.Update(func() {
				if !o.Wanted() {
					v.polling = false
					keep <- 0
					return
				}
				v.loaded = true
				if err != nil {
					v.err = client.Explain(err)
				} else {
					v.err, v.info = "", info
				}
				wait := pollIdle
				if v.running() {
					wait = pollRunning
				}
				keep <- wait
			})
			wait := <-keep
			if wait == 0 {
				return
			}
			select {
			case <-time.After(wait):
			case <-v.poke:
			}
		}
	}()
}

// do runs a call off the main thread, then asks again.
func (v *View) do(o Options, call func(cl *client.Client) error) {
	cl := o.Client
	if cl == nil {
		return
	}
	go func() {
		err := call(cl)
		o.Update(func() {
			if err != nil {
				v.err = client.Explain(err)
			}
			v.Poke()
		})
	}()
}

// run returns the last run of check name, if it ran.
func (v *View) run(name string) (proto.CheckRun, bool) {
	for _, r := range v.info.Runs {
		if r.Name == name {
			return r, true
		}
	}
	return proto.CheckRun{}, false
}

// Draw shows the checks, each with how it last went, for the panel.
func (v *View) Draw(c *ui.Context, o Options) {
	v.follow(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(0, theme.S3, theme.S3, theme.S3).Gap(theme.S2).Children(func() {
		v.toolbar(c, o)
		if v.err != "" {
			widget.Notice(c, nil, theme.Del, v.err, "", nil)
		}
		switch {
		case !v.loaded:
			note(c, "Looking for checks…")
		case len(v.info.Checks) == 0:
			note(c, "No checks found in this project. Edit them to add the commands that test it.")
		default:
			current := ""
			if o.Current != nil {
				current = o.Current()
			}
			widget.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					for _, ch := range v.info.Checks {
						v.checkRow(c.Key(ch.Name), o, ch, ch.Name == current)
					}
				})
			})
		}
	})
	v.editDialog(c, o)
}

func note(c *ui.Context, text string) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(theme.TextSoft).TextAlign(ui.Center)
	})
}

// toolbar runs or stops the checks, says where they come from, and edits them.
func (v *View) toolbar(c *ui.Context, o Options) {
	widget.ToolHeader(c, func() {
		switch {
		case !v.loaded || len(v.info.Checks) == 0:
		case v.running():
			if widget.Button(c, widget.Secondary, icon.Stop, "Stop", "").Clicked() {
				v.do(o, func(cl *client.Client) error { return cl.StopChecks(o.Session.ID) })
			}
		default:
			if widget.Button(c, widget.Primary, icon.Play, "Run all", "").Clicked() {
				v.do(o, func(cl *client.Client) error { _, err := cl.RunChecks(o.Session.ID); return err })
			}
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			if v.loaded && len(v.info.Checks) > 0 {
				from := "Found in the project's files"
				if v.info.Custom {
					from = "Set for this project"
				}
				widget.Running(c, from, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			}
		})
		if v.loaded && widget.Button(c, widget.Secondary, nil, "Edit", "").Clicked() {
			v.edit = &editForm{open: true, rows: append([]proto.Check(nil), v.info.Checks...)}
		}
	})
}

// checkRow is a check in the panel: how it went, its command, which runs when
// too long, and the facts of its last run. It opens its output in the center.
func (v *View) checkRow(c *ui.Context, o Options, ch proto.Check, on bool) {
	r, ran := v.run(ch.Name)
	ui.Column(c).FillWidth().Children(func() {
		b := ui.ButtonBase(c).FillWidth().MinHeight(36).Padding(theme.S1, theme.S3).Gap(theme.S1).Radius(theme.RadiusCard).Justify(ui.Start).
			Cursor(ui.CursorPointer).Label(ch.Name)
		bg := theme.Fade(theme.Selected, 0)
		switch {
		case on:
			bg = theme.Selected
		case b.Hovered():
			bg = theme.Hover
		}
		b.Background(widget.QuickTint(b, "bg", bg)).Children(func() {
			// As wide as the spinner, which takes more than its size.
			ui.Row(c).Width(16).Shrink(0).Justify(ui.Center).Children(func() { stateMark(c, r.State, ran) })
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, commandOf(ch), widget.RunStyle{Mono: true, Always: on})
				widget.Running(c.Key("facts"), facts(r, ran), widget.RunStyle{Size: theme.TextLabel, Color: theme.SoftOn(on), Always: on})
			})
		})
		if b.Clicked() && o.OpenOutput != nil {
			o.OpenOutput(ch.Name)
		}
		if ran && r.State == proto.CheckFailed {
			v.judgedLine(c, o, r, 32)
		}
	})
}

// commandOf is what a check runs, or its name when it doesn't say.
func commandOf(ch proto.Check) string {
	if ch.Command != "" {
		return ch.Command
	}
	return ch.Name
}

// stateColor is the color of how a check went, nothing for one that isn't over
// or didn't run.
func stateColor(state string, ran bool) ui.Color {
	switch {
	case !ran:
		return theme.Idle
	case state == proto.CheckPassed:
		return theme.Done
	case state == proto.CheckFailed:
		return theme.Error
	case state == proto.CheckRunning:
		return theme.Working
	}
	return theme.Idle
}

// Color is the color of how check name went, for its tab's dot.
func (v *View) Color(name string) ui.Color {
	r, ran := v.run(name)
	return stateColor(r.State, ran)
}

// stateMark is a dot or spinner for how a check went: hollow while it has no
// result (not run, queued), dim when it was stopped.
func stateMark(c *ui.Context, state string, ran bool) {
	switch {
	case !ran || state == proto.CheckQueued:
		widget.Dot(c, theme.Idle, true, 8)
	case state == proto.CheckRunning:
		widget.Spinner(c, 12)
	default:
		widget.Dot(c, stateColor(state, ran), false, 8)
	}
}

// facts is how a check's last run went, in a few words.
func facts(r proto.CheckRun, ran bool) string {
	switch {
	case !ran:
		return "not run"
	case r.State == proto.CheckQueued:
		return "queued"
	case r.State == proto.CheckRunning:
		return "running · " + time.Since(r.Started).Round(time.Second).String()
	case r.State == proto.CheckStopped:
		return "stopped"
	case r.State == proto.CheckFailed:
		return fmt.Sprintf("failed (exit %d) · %s", r.ExitCode, r.Duration.Round(100*time.Millisecond))
	}
	return "passed · " + r.Duration.Round(100*time.Millisecond).String()
}

// lastLines is the last n lines of s, as plain text: colors dropped and
// tabs set to columns of 8.
func lastLines(s string, n int) string {
	s = ansi.ReplaceAllString(strings.TrimRight(s, "\n"), "")
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = append([]string{"…"}, lines[len(lines)-n:]...)
	}
	for i, l := range lines {
		lines[i] = expandTabs(strings.TrimSuffix(l, "\r"))
	}
	return strings.Join(lines, "\n")
}

// ansi matches terminal escape sequences, which tools print for colors
// though NO_COLOR asks them not to.
var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func expandTabs(line string) string {
	if !strings.Contains(line, "\t") {
		return line
	}
	var b strings.Builder
	col := 0
	for _, r := range line {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// editDialog sets the project's checks: a name and a command each.
func (v *View) editDialog(c *ui.Context, o Options) {
	f := v.edit
	if f == nil {
		return
	}
	save, reset := false, false
	widget.Dialog(c, &f.open, 560, theme.S3, func() {
		if widget.DialogHeader(c, "Checks of this project",
			"Commands run in the session's folder, one after the other. They are kept by the daemon on this host, for every session of the project, so the project stays untouched.", true) {
			f.open = false
		}
		ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
			for i := range f.rows {
				ui.Row(c.Key(i)).FillWidth().Gap(theme.S2).Children(func() {
					widget.Input(c, &f.rows[i].Name, "Name").Width(120).Shrink(0)
					widget.Input(c, &f.rows[i].Command, "Command, as typed in a shell").Grow(1).MinWidth(0).Font(theme.MonoFont)
					if widget.IconButton(c, icon.Close, "Remove", false).Clicked() {
						f.rows = append(f.rows[:i:i], f.rows[i+1:]...)
					}
				})
			}
			if widget.Button(c, widget.Secondary, icon.Plus, "Add a check", "").Clicked() {
				f.rows = append(f.rows, proto.Check{})
			}
		})
		widget.DialogFooter(c, "", func() {
			if v.info.Custom && widget.Button(c, widget.Secondary, nil, "Use those found", "").Clicked() {
				reset = true
			}
			if widget.Button(c, widget.Secondary, nil, "Cancel", "").Clicked() {
				f.open = false
			}
			if widget.PrimaryButton(c, nil, "Save", "").Clicked() {
				save = true
			}
		})
	})
	if save || reset {
		rows := f.rows
		if reset {
			rows = nil
		}
		f.open = false
		v.do(o, func(cl *client.Client) error { _, err := cl.SetChecks(o.Session.ID, rows); return err })
	}
	if !f.open {
		v.edit = nil
	}
}
