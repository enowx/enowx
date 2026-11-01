// Package creditview shows what the agent accounts signed in on the
// connected host have left of their plans, on the Credits page.
package creditview

import (
	"fmt"
	"strconv"
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
	force  chan struct{} // asks the poller for a fresh reading
}

// Host is what the views need of the connected host.
type Host struct {
	Name string
	// AgentName names an agent preset ID.
	AgentName func(id string) string
	// SignIn, when set, offers to sign an agent in again.
	SignIn func(agent string)
}

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

// minCardHeight is the least a row of the page's cards gets: two limits and
// the warning fit.
const minCardHeight = 224

// accounts is a card per account, or says why there are none.
func (t *Tracker) accounts(c *ui.Context, h Host) {
	switch {
	case t.err != "":
		widget.PageBody(c, false, func() { widget.EmptyState(c, icon.Alert, "The accounts can't be read", t.err, nil) })
		return
	case !t.loaded || len(t.report.Accounts) == 0 && t.report.Checking:
		widget.PageBody(c, false, func() {
			ui.Row(c).FillWidth().Gap(theme.S2).Justify(ui.Center).Children(func() {
				widget.Spinner(c, 14)
				ui.Text(c, "Reading the accounts on "+h.Name+"…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			})
		})
		return
	case len(t.report.Accounts) == 0:
		widget.PageBody(c, false, func() {
			widget.EmptyState(c, icon.Credits, "No agent is signed in on "+h.Name, "Claude Code, Codex and Kiro CLI show here once they are.", nil)
		})
		return
	}
	widget.FillGrid(c, 2, len(t.report.Accounts), minCardHeight, func() {
		for i, a := range t.report.Accounts {
			accountCard(c.Key(i), h, a)
		}
	})
}

// accountTitle is what an account's card is called: its plan, else its agent.
func accountTitle(h Host, a proto.CreditAccount) string {
	if a.Plan != "" {
		return a.Plan
	}
	return h.AgentName(a.Agent)
}

// accountWho says whose it is: the agent, the enowX account, who is signed in.
func accountWho(h Host, a proto.CreditAccount) string {
	parts := []string{h.AgentName(a.Agent), "Default"}
	if a.AccountName != "" {
		parts[1] = a.AccountName
	}
	if a.Account != "" {
		parts = append(parts, a.Account)
	}
	return strings.Join(parts, " · ")
}

// window is one limit: its name, how much is used and when it starts over,
// and a meter.
func window(c *ui.Context, w proto.CreditWindow) {
	ui.Column(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c.Key("name"), w.Label, widget.RunStyle{}) })
			used := fmt.Sprintf("%.0f%%", w.Used)
			if !w.ResetsAt.IsZero() {
				used += " · Resets in " + textfmt.Until(w.ResetsAt, time.Now())
			}
			ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
				widget.Running(c.Key("used"), used, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		})
		widget.Meter(c, widget.Segment{Fraction: w.Used / 100, Color: level(w.Used)})
		if w.Detail != "" {
			widget.Running(c.Key("detail"), w.Detail, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		}
	})
}

// accountCard is an account on the Credits page.
func accountCard(c *ui.Context, h Host, a proto.CreditAccount) {
	widget.GridCard(c).Children(func() {
		widget.CardTitle(c, nil, accountTitle(h, a), nil)
		ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			widget.Avatar(c, icon.Agent(a.Agent), 32, theme.Text)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, accountWho(h, a), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		})
		problem(c, h, a)
		for i, w := range a.Windows {
			window(c.Key(i), w)
		}
		if w, ok := a.Tightest(); ok && w.Used >= 90 {
			widget.Banner(c, theme.Warning, "Almost used up. Start new sessions on this host with another agent until it resets.", nil)
		}
		if a.Note != "" {
			widget.CardNote(c, a.Note, 2)
		}
	})
}

// problem says why an account has no reading, with a way to sign in again.
func problem(c *ui.Context, h Host, a proto.CreditAccount) {
	if a.Error == "" {
		return
	}
	ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Text(c, a.Error).FontSize(theme.TextLabel).TextColor(theme.Warning).Grow(1).Basis(0).MinWidth(0)
		if h.SignIn != nil && widget.GhostButton(c, "Sign in").Clicked() {
			h.SignIn(a.Agent)
		}
	})
}

// Page is the Credits page under its header: its figures, then a card per
// account.
func (t *Tracker) Page(c *ui.Context, h Host) {
	closest, tones := "–", map[int]ui.Color{}
	if acc, w, ok := t.tightest(); ok {
		closest = fmt.Sprintf("%s, %.0f%%", accountTitle(h, acc), w.Used)
		if w.Used >= 75 {
			tones[1] = level(w.Used)
		}
	}
	widget.FigsBandToned(c, []widget.Fig{
		{Label: "Accounts", Value: strconv.Itoa(len(t.report.Accounts))},
		{Label: "Closest to its limit", Value: closest},
		{Label: "Next reset", Value: t.nextReset()},
	}, tones)
	t.accounts(c, h)
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

// Show puts a reading in without asking the host, for a development build's
// sample data.
func (t *Tracker) Show(r proto.CreditsReport) { t.report, t.loaded, t.err = r, true, "" }
