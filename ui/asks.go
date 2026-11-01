package ui

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/vaultview"
	"github.com/enowx/enowx/ui/widget"
)

// What is asked of the user without their doing anything (an agent, a daemon,
// a connection the app started by itself) comes as a card over the window,
// never as a dialog: it doesn't take the keyboard from what they're typing,
// and nothing is answered but by their click, or Tab and Enter
// (widget.AskCard). One shows at a time, the oldest first, so what they read
// doesn't change under them; behind a dialog they opened, it waits.

const (
	// How long the daemons wait for the answer, less a margin: an answer
	// nobody waits for any more must not act.
	hostAskLife  = 5 * time.Minute // confirmTimeout, hibernating, features.request
	vaultAskLife = 2 * time.Minute // vaultTimeout
	askMargin    = 5 * time.Second
)

// askArmDelay is how long a card shown ignores its answers: long enough for a
// click already on its way, too short to notice.
var askArmDelay = 500 * time.Millisecond

// askUntil is when an ask made now stops counting. On the wall clock: a machine
// asleep may outlive a daemon on another one.
func askUntil(life time.Duration) time.Time { return time.Now().Round(0).Add(life - askMargin) }

// askLine is the cards waiting, in the order they came. It belongs to the main
// thread.
type askLine struct {
	keys    []string
	shown   string
	shownAt time.Time
}

// order keeps the keys waiting in the order they came, new ones last.
func (l *askLine) order(waiting []*widget.Ask) {
	l.keys = slices.DeleteFunc(l.keys, func(k string) bool {
		return !slices.ContainsFunc(waiting, func(q *widget.Ask) bool { return q.Key == k })
	})
	for _, q := range waiting {
		if !slices.Contains(l.keys, q.Key) {
			l.keys = append(l.keys, q.Key)
		}
	}
}

// asksView shows the oldest card waiting. It goes before the dialogs in the
// view: a dialog the user opened stays on top, with the keyboard.
func (a *App) asksView(c *ui.Context) {
	if until, ok := a.dropLateAsks(); ok {
		c.After(time.Until(until) + 50*time.Millisecond)
	}
	var waiting []*widget.Ask
	for _, source := range []func(*ui.Context) *widget.Ask{
		a.hostControlAsk, a.takeoverAsk, a.vaultAsk, a.featureAskCard, a.hibernateAsk, a.decisionAsk, a.accountAsk, a.ragAsk,
		a.trustAsk, a.runAsAsk, a.passphraseAsk, a.signInAsk,
	} {
		if q := source(c); q != nil {
			waiting = append(waiting, q)
		}
	}
	l := &a.asks
	l.order(waiting)
	if len(l.keys) == 0 {
		l.shown = ""
		return
	}
	if l.shown != l.keys[0] {
		l.shown, l.shownAt = l.keys[0], time.Now()
	}
	i := slices.IndexFunc(waiting, func(q *widget.Ask) bool { return q.Key == l.shown })
	widget.AskCard(c, waiting[i], a.asksBehind(len(waiting)), time.Since(l.shownAt) >= askArmDelay)
}

// asksBehind counts what waits after the card shown, given how many kinds show
// their first.
func (a *App) asksBehind(shown int) int {
	more := shown - 1 + max(len(a.hostAsks)-1, 0) + max(len(a.featAsks)-1, 0) + max(len(a.hibernateAsks)-1, 0) +
		max(len(a.decisions)-1, 0) + max(len(a.acctAsks)-1, 0) + max(len(a.ragOffers)-1, 0)
	if a.presence != nil {
		more += max(len(a.presence.asks)-1, 0)
	}
	if a.vlt != nil {
		more += max(len(a.vlt.asks)-1, 0)
	}
	return more
}

// dropLateAsks refuses what is asked past the daemon's wait, so a late yes runs
// nothing and the question goes. It runs before any card's buttons are read. ok
// is whether an ask still has a deadline, the next at until.
func (a *App) dropLateAsks() (until time.Time, ok bool) {
	now := time.Now()
	late := func(at time.Time, what string) bool {
		if at.IsZero() {
			return false // nothing waits on it
		}
		if now.Before(at) {
			if !ok || at.Before(until) {
				until, ok = at, true
			}
			return false
		}
		a.notify(what + ": it wasn't answered in time")
		return true
	}
	for _, q := range slices.Clone(a.hostAsks) {
		if late(q.until, "A session's request") {
			a.hostAsks = slices.DeleteFunc(a.hostAsks, func(x *hostAsk) bool { return x == q })
			q.refuse()
		}
	}
	for _, q := range slices.Clone(a.hibernateAsks) {
		if late(q.until, "“"+q.asker+"” asking to hibernate") {
			a.hibernateAsks = slices.DeleteFunc(a.hibernateAsks, func(x *hibernateForm) bool { return x == q })
			q.reply(false)
		}
	}
	for _, q := range slices.Clone(a.featAsks) {
		if late(q.until, "An agent asking to turn on "+q.req.ID) {
			a.answerFeatureAsk(q, false, "")
		}
	}
	if v := a.vlt; v != nil {
		for _, q := range slices.Clone(v.asks) {
			if late(q.until, q.ask.Title) {
				v.asks = slices.DeleteFunc(v.asks, func(x *vaultRequestAsk) bool { return x == q })
				a.vaultAnswer(q, vaultview.AnswerDeny)
			}
		}
	}
	return until, ok
}
