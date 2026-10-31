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
}

// View is one session's checks. It belongs to the main thread; its poller
// reports through Options.Update.
type View struct {
	info    proto.ChecksInfo
	loaded  bool
	err     string
	polling bool
	poke    chan struct{}
	open    map[string]bool // checks whose output shows, by name
	edit    *editForm
}

type editForm struct {
	open bool
	rows []proto.Check
}

// New returns a view with nothing read.
func New() *View { return &View{poke: make(chan struct{}, 1), open: map[string]bool{}} }

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

// Draw shows the checks, each with how it last went.
func (v *View) Draw(c *ui.Context, o Options) {
	v.follow(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		v.toolbar(c, o)
		if v.err != "" {
			ui.Row(c).FillWidth().Padding(8, 14).Background(theme.Fade(theme.Error, 0.1)).Children(func() {
				ui.Text(c, v.err).FontSize(12).TextColor(theme.Error).Shrink(1)
			})
		}
		switch {
		case !v.loaded:
			note(c, "Looking for checks…")
		case len(v.info.Checks) == 0:
			note(c, "No checks found in this project. Edit them to add the commands that test it.")
		default:
			ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					for _, ch := range v.info.Checks {
						v.checkRow(c.Key(ch.Name), o, ch)
					}
				})
			})
		}
	})
	v.editDialog(c, o)
}

func note(c *ui.Context, text string) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Children(func() {
		ui.Text(c, text).FontSize(12.5).TextColor(theme.Muted).TextAlign(ui.Center)
	})
}

// toolbar says where the checks come from, and runs or stops them.
func (v *View) toolbar(c *ui.Context, o Options) {
	ui.Row(c).Height(42).Shrink(0).FillWidth().Padding(0, 10, 0, 14).Gap(8).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		from := "Found in the project's files"
		if v.info.Custom {
			from = "Set for this project"
		}
		if v.loaded && len(v.info.Checks) > 0 {
			ui.Text(c, from).FontSize(11).TextColor(theme.Muted).SingleLine().Shrink(1)
		}
		ui.Spacer(c)
		if v.loaded && widget.GhostButton(c, "Edit").Clicked() {
			v.edit = &editForm{open: true, rows: append([]proto.Check(nil), v.info.Checks...)}
		}
		switch {
		case !v.loaded || len(v.info.Checks) == 0:
		case v.running():
			if widget.Button(c, widget.Secondary, icon.Stop, "Stop", "").Clicked() {
				v.do(o, func(cl *client.Client) error { return cl.StopChecks(o.Session.ID) })
			}
		default:
			if widget.Button(c, widget.Primary, icon.Check, "Run all", "").Clicked() {
				v.do(o, func(cl *client.Client) error { _, err := cl.RunChecks(o.Session.ID); return err })
			}
		}
	})
}

// checkRow is a check: how it went, its command, how long, and its
// output when unfolded.
func (v *View) checkRow(c *ui.Context, o Options, ch proto.Check) {
	r, ran := v.run(ch.Name)
	open := v.open[ch.Name]
	runOne := false
	ui.Column(c).FillWidth().BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		b := ui.ButtonBase(c).FillWidth().Padding(8, 10, 8, 14).Gap(10).Justify(ui.Start).Cursor(ui.CursorPointer).Label(ch.Name)
		if b.Hovered() {
			b.Background(theme.Hover)
		}
		b.Children(func() {
			stateMark(c, r.State, ran)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, ch.Name).FontSize(12.5).SingleLine()
				ui.Text(c, ch.Command).Font(theme.MonoFont).FontSize(11).TextColor(theme.Muted).SingleLine().Shrink(1)
			})
			ui.Text(c, facts(r, ran)).FontSize(11).TextColor(theme.Muted).SingleLine().Shrink(0)
			if !v.running() && widget.IconButton(c, icon.ChevronRight, "Run "+ch.Name, false).Clicked() {
				runOne = true
				v.do(o, func(cl *client.Client) error { _, err := cl.RunChecks(o.Session.ID, ch.Name); return err })
			}
		})
		if b.Clicked() && !runOne {
			v.open[ch.Name] = !open
		}
		if ran && r.State == proto.CheckFailed {
			v.judgedLine(c, o, r)
		}
		if open && ran {
			out := lastLines(r.Output, outputLines)
			if out == "" {
				out = "(no output)"
			}
			ui.Scroll(c).FillWidth().MaxHeight(280).Background(theme.Sunken).Children(func() {
				ui.Text(c, out).Font(theme.MonoFont).FontSize(11).TextColor(theme.TextSoft).Padding(8, 14).FillWidth()
			})
		}
	})
}

// stateMark is a dot, mark or spinner for how a check went.
func stateMark(c *ui.Context, state string, ran bool) {
	switch {
	case !ran:
		widget.Dot(c, theme.Idle, true, 8)
	case state == proto.CheckRunning:
		widget.Spinner(c, 12)
	case state == proto.CheckPassed:
		ui.Icon(c, icon.Check).FontSize(13).TextColor(theme.Done)
	case state == proto.CheckFailed:
		ui.Icon(c, icon.Close).FontSize(13).TextColor(theme.Error)
	default: // queued, stopped
		widget.Dot(c, theme.Idle, true, 8)
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
		return "running " + time.Since(r.Started).Round(time.Second).String()
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
	widget.Dialog(c, &f.open, 560, 14, func() {
		widget.DialogHeader(c, "Checks of this project",
			"Commands run in the session's folder, one after the other. They are kept on this host, for every session of the project.", false)
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			for i := range f.rows {
				ui.Row(c.Key(i)).FillWidth().Gap(8).Children(func() {
					widget.Input(c, &f.rows[i].Name, "name").Width(110).Shrink(0)
					widget.Input(c, &f.rows[i].Command, "command, as typed in a shell").Grow(1).MinWidth(0)
					if widget.IconButton(c, icon.Close, "Remove", false).Clicked() {
						f.rows = append(f.rows[:i:i], f.rows[i+1:]...)
					}
				})
			}
			if widget.GhostButton(c, "Add a check").Clicked() {
				f.rows = append(f.rows, proto.Check{})
			}
		})
		widget.DialogFooter(c, "", func() {
			if v.info.Custom && widget.GhostButton(c, "Use those found").Clicked() {
				reset = true
			}
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			if widget.PrimaryButton(c, icon.Check, "Save", "").Clicked() {
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
