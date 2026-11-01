// Package creditview shows what the agent accounts signed in on the
// connected host have left of their plans: a chip for the title bar, its
// popover, and the Credits page.
package creditview

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The poller asks the daemon often while it reads, so the reading shows
// as soon as it is done, and rarely once it has one: the daemon reads
// again by itself when its reading gets old.
const (
	whileChecking = 3 * time.Second
	settled       = time.Minute
)

// Tracker holds the latest reading. It belongs to the main thread; the
// poller started by Follow fills it.
type Tracker struct {
	report proto.CreditsReport
	err    string
	loaded bool
	open   bool          // the chip's popover shows
	force  chan struct{} // asks the poller for a fresh reading
}

// Host is what the views need of the connected host.
type Host struct {
	Name string
	// AgentName names an agent preset ID.
	AgentName func(id string) string
	// SignIn, when set, offers to sign an agent in again.
	SignIn func(agent string)
	// OpenPage shows the Credits page.
	OpenPage func()
	// Focus is the agent of the session in view, whose account the chip
	// shows; "" or an agent with no reading lets the chip take turns.
	Focus string
}

// rotateEvery is how long the chip shows each account while it takes
// turns.
const rotateEvery = 4 * time.Second

// Follow reads cl's host while wanted (called on the main thread) reports
// true and cl is open. update runs a function on the main thread.
func (t *Tracker) Follow(cl *client.Client, update func(func()), wanted func() bool) {
	*t = Tracker{force: make(chan struct{}, 1)}
	force := t.force
	go func() {
		refresh := false
		for {
			r, err := cl.Credits(refresh)
			refresh = false
			stop := make(chan bool, 1)
			update(func() {
				if !wanted() {
					stop <- true
					return
				}
				stop <- false
				t.loaded = true
				t.err = ""
				if err != nil {
					t.err = describe(err)
					return
				}
				t.report = r
			})
			if <-stop {
				return
			}
			wait := settled
			if err == nil && r.Checking {
				wait = whileChecking
			}
			select {
			case <-cl.Done():
				return
			case <-force:
				refresh = true
			case <-time.After(wait):
			}
		}
	}()
}

// Refresh asks the daemon to read the accounts again.
func (t *Tracker) Refresh() {
	select {
	case t.force <- struct{}{}:
	default:
	}
}

func describe(err error) string {
	if strings.Contains(err.Error(), "unknown method") {
		return "This host runs an older enowX. Reconnect to update it."
	}
	return err.Error()
}

// tightest is the account closest to a limit, with that limit.
func (t *Tracker) tightest() (proto.CreditAccount, proto.CreditWindow, bool) {
	var acc proto.CreditAccount
	var win proto.CreditWindow
	found := false
	for _, a := range t.report.Accounts {
		if w, ok := a.Tightest(); ok && (!found || w.Used > win.Used) {
			acc, win, found = a, w, true
		}
	}
	return acc, win, found
}

// level colors a used percentage.
func level(used float64) ui.Color { return widget.Level(used / 100) }

// levelText colors a percentage in the chip: plain while far from the
// limit, the level's color once close to it.
func levelText(used float64) ui.Color {
	if used < 70 {
		return theme.Text
	}
	return level(used)
}

// Summary is a line for the status bar: each account's limits, short.
func (t *Tracker) Summary(h Host) string {
	var parts []string
	for _, a := range t.report.Accounts {
		if len(a.Windows) == 0 {
			continue
		}
		windows := make([]string, len(a.Windows))
		for i, w := range a.Windows {
			windows[i] = fmt.Sprintf("%s %.0f%%", w.Short(), w.Used)
		}
		parts = append(parts, h.AgentName(a.Agent)+" "+strings.Join(windows, " · "))
	}
	return strings.Join(parts, "  ·  ")
}

// shown is the account the chip shows: the one of the agent in view, else
// each account with a reading in turn. It reports which of them it is, for
// telling a change apart.
func (t *Tracker) shown(c *ui.Context, h Host) (proto.CreditAccount, proto.CreditWindow, int, bool) {
	var read []proto.CreditAccount
	for _, a := range t.report.Accounts {
		if _, ok := a.Tightest(); ok {
			read = append(read, a)
		}
	}
	if len(read) == 0 {
		return proto.CreditAccount{}, proto.CreditWindow{}, -1, false
	}
	for i, a := range read {
		if a.Agent == h.Focus {
			w, _ := a.Tightest()
			return a, w, i, true
		}
	}
	i := 0
	if len(read) > 1 {
		now := c.Now()
		i = int(now.UnixNano()/int64(rotateEvery)) % len(read)
		next := now.Truncate(rotateEvery).Add(rotateEvery)
		c.After(next.Sub(now))
	}
	w, _ := read[i].Tightest()
	return read[i], w, i, true
}

// Chip is the title bar's credits button: the account of the agent in
// view, or each in turn, colored by how close it is to its limit. It
// opens the popover.
func (t *Tracker) Chip(c *ui.Context, h Host) {
	acc, win, which, ok := t.shown(c, h)
	label := "Credits"
	col := theme.Idle
	if ok {
		label = h.AgentName(acc.Agent)
		col = level(win.Used)
	}
	b := ui.ButtonBase(c.Key("credits-chip")).Height(26).PaddingX(9).Gap(6).Radius(theme.RadiusControl).Shrink(0).
		Background(theme.Sunken).Label("Credits").Tooltip("Credits on " + h.Name).Cursor(ui.CursorPointer).TextColor(theme.TextSoft)
	border := theme.BorderStrong
	if b.Hovered() || t.open {
		border = theme.AccentLine
	}
	b.Border(1, widget.Tint(b, "border", border)).Clip().Children(func() {
		// Each account slides up into the chip as it takes its turn.
		ui.Row(c.Key(which)).Gap(6).Transition(ui.ElementTransition{
			Duration: 320 * time.Millisecond, Ease: widget.Out, Enter: &ui.Motion{Y: 8, Opacity: 0},
		}).Children(func() {
			widget.Dot(c, col, !ok, 6)
			ui.Text(c, label).FontSize(theme.TextLabel).SingleLine()
			// Every limit, short: "5h 8% · Wk 6% · Fable 0%".
			for i, w := range acc.Windows {
				if i > 0 {
					ui.Text(c, "·").FontSize(theme.TextLabel).TextColor(theme.Muted)
				}
				ui.Row(c).Gap(3).Children(func() {
					ui.Text(c, w.Short()).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
					ui.Text(c, fmt.Sprintf("%.0f%%", w.Used)).FontSize(theme.TextLabel).TextColor(levelText(w.Used)).SingleLine()
				})
			}
		})
	})
	if b.Clicked() {
		t.open = !t.open
	}
	if !t.open {
		return
	}
	ui.PopoverBase(c, b, &t.open, func(panel ui.Element) {
		widget.Popup(panel.AttachTo(b, ui.AnchorBottomRight, ui.AnchorTopRight).Margin(6, 0, 0, 0).Width(320).Padding(10).Gap(8))
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, "Credits").FontSize(theme.TextUI).Grow(1)
			ui.Text(c, t.checkedLabel()).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			if widget.IconButton(c, icon.Sync, "Read again", false).Size(26, 26).Clicked() {
				t.Refresh()
			}
		})
		t.accounts(c, h, false)
		if widget.GhostButton(c, "Open the Credits page").FillWidth().Justify(ui.Center).Clicked() {
			t.open = false
			if h.OpenPage != nil {
				h.OpenPage()
			}
		}
	})
}

// checkedLabel says how fresh the reading is.
func (t *Tracker) checkedLabel() string {
	switch {
	case t.report.Checking && t.report.Checked.IsZero():
		return "Reading…"
	case t.report.Checking:
		return "Updating…"
	case t.report.Checked.IsZero():
		return ""
	}
	return textfmt.Age(t.report.Checked, time.Now()) + " ago"
}

// accounts lists the accounts, as rows in the popover or cards on the
// page, or says why there are none.
func (t *Tracker) accounts(c *ui.Context, h Host, cards bool) {
	switch {
	case t.err != "":
		ui.Text(c, t.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		return
	case !t.loaded || len(t.report.Accounts) == 0 && t.report.Checking:
		ui.Row(c).Gap(8).Children(func() {
			widget.Spinner(c, 14)
			ui.Text(c, "Reading the accounts on "+h.Name+"…").FontSize(theme.TextLabel).TextColor(theme.Muted)
		})
		return
	case len(t.report.Accounts) == 0:
		ui.Text(c, "No agent is signed in on "+h.Name+". Claude Code, Codex and Kiro CLI show here once they are.").
			FontSize(12).TextColor(theme.Muted)
		return
	}
	if !cards {
		for i, a := range t.report.Accounts {
			accountRow(c.Key(i), h, a)
		}
		return
	}
	widget.FitGrid(c, 200, 2).Gap(10).Children(func() {
		for i, a := range t.report.Accounts {
			accountCard(c.Key(i), h, a)
		}
	})
}

// accountHead is an account's agent, plan and who it is signed in as.
func accountHead(c *ui.Context, h Host, a proto.CreditAccount, size float32) {
	ui.Row(c).FillWidth().Gap(9).Children(func() {
		widget.Avatar(c, icon.Agent(a.Agent), size, theme.Text)
		ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).Gap(7).MinWidth(0).Children(func() {
				ui.Text(c, h.AgentName(a.Agent)).FontSize(theme.TextUI).SingleLine()
				if a.Plan != "" {
					widget.Tag(c, a.Plan, theme.TextSoft)
				}
			})
			if a.Account != "" {
				ui.Text(c, a.Account).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
			}
		})
	})
}

// window is one limit: its name, a meter, how much is used and when it
// starts over.
func window(c *ui.Context, w proto.CreditWindow) {
	ui.Column(c).FillWidth().Gap(5).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, w.Label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("%.0f%%", w.Used)).FontSize(theme.TextLabel).TextColor(level(w.Used)).SingleLine()
			if !w.ResetsAt.IsZero() {
				ui.Text(c, "resets in "+textfmt.Until(w.ResetsAt, time.Now())).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			}
		})
		widget.Meter(c, widget.Segment{Fraction: w.Used / 100, Color: level(w.Used)})
		if w.Detail != "" {
			ui.Text(c, w.Detail).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		}
	})
}

// accountRow is an account in the popover.
func accountRow(c *ui.Context, h Host, a proto.CreditAccount) {
	ui.Column(c).FillWidth().Gap(8).Padding(9).Radius(theme.RadiusCard).Background(theme.Raised).
		Border(1, theme.Border).Children(func() {
		accountHead(c, h, a, 24)
		problem(c, h, a, false)
		for i, w := range a.Windows {
			window(c.Key(i), w)
		}
	})
}

// accountCard is an account on the Credits page.
func accountCard(c *ui.Context, h Host, a proto.CreditAccount) {
	widget.Card(c).Gap(12).Children(func() {
		accountHead(c, h, a, 30)
		problem(c, h, a, true)
		for i, w := range a.Windows {
			window(c.Key(i), w)
		}
		if w, ok := a.Tightest(); ok && w.Used >= 90 {
			ui.Text(c, "Almost used up. Start new sessions on this host with another agent until it resets.").
				FontSize(11.5).TextColor(theme.Error)
		}
		if a.Note != "" {
			ui.Text(c, a.Note).FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
	})
}

// problem says why an account has no reading, with a way to sign in again.
func problem(c *ui.Context, h Host, a proto.CreditAccount, offerSignIn bool) {
	if a.Error == "" {
		return
	}
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		ui.Text(c, a.Error).FontSize(theme.TextLabel).TextColor(theme.Warning).Grow(1).Shrink(1)
		if offerSignIn && h.SignIn != nil && widget.GhostButton(c, "Sign in").Clicked() {
			h.SignIn(a.Agent)
		}
	})
}

// Page is the Credits page's body: a summary, then a card per account.
func (t *Tracker) Page(c *ui.Context, h Host) {
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		widget.FitGrid(c, 220, 3).Gap(10).Children(func() {
			n := len(t.report.Accounts)
			summaryCard(c, "Accounts", fmt.Sprintf("%d on %s", n, h.Name), theme.Text)
			if acc, w, ok := t.tightest(); ok {
				summaryCard(c, "Closest to its limit", fmt.Sprintf("%s · %.0f%%", h.AgentName(acc.Agent), w.Used), level(w.Used))
			} else {
				summaryCard(c, "Closest to its limit", "–", theme.Muted)
			}
			summaryCard(c, "Next reset", t.nextReset(), theme.Text)
		})
		t.accounts(c, h, true)
		ui.Text(c, "enowX reads these with each agent's own sign-in on "+h.Name+", from the agent's vendor only, every few minutes while connected.").
			FontSize(11.5).TextColor(theme.Muted)
	})
}

func summaryCard(c *ui.Context, label, value string, col ui.Color) {
	widget.Card(c).Gap(4).Children(func() {
		ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		ui.Text(c, value).FontSize(theme.TextRead).TextColor(col).SingleLine()
	})
}

// nextReset is how soon the first limit starts over.
func (t *Tracker) nextReset() string {
	var next time.Time
	for _, a := range t.report.Accounts {
		for _, w := range a.Windows {
			if !w.ResetsAt.IsZero() && w.ResetsAt.After(time.Now()) && (next.IsZero() || w.ResetsAt.Before(next)) {
				next = w.ResetsAt
			}
		}
	}
	if next.IsZero() {
		return "–"
	}
	return "in " + textfmt.Until(next, time.Now())
}

// CheckedLabel says how fresh the reading is, for the page header.
func (t *Tracker) CheckedLabel() string { return t.checkedLabel() }
