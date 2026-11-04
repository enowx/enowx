package terminal

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/core/platformagent"
	"github.com/enowx/enowx/core/platformagent/record"
)

// What the agent asks the user. The keys are what enowX's chat and `answer` press
// (core/agent's Preset.Approve, ui/chatview/ask.go), the same as a user would:
//
//	permission: "1" allow, "2" always, "3" or Esc deny, each without Enter
//	question:   N its option, without Enter; own words: the number after the last
//	            option, then the text, then CR
//	several picks: numbers with spaces or commas, then CR
//
// A message on its way can't answer by accident: a key answers a permission only when
// it came alone and nothing is typed on the line, otherwise it goes to the line.

// asker is one thing asked of the user, on Run's goroutine.
type asker interface {
	// open shows it. key takes a key; burst says it came with others in one read, more
	// that others follow it. noAnswer ends it: no answer is coming.
	open(t *Terminal)
	key(t *Terminal, k Key, burst, more bool)
	noAnswer(t *Terminal)
}

// await asks a, until it answers on ch or ctx ends.
func await[T any](ctx context.Context, t *Terminal, a asker, ch <-chan T) (T, error) {
	var zero T
	t.post(func() { t.pushAsk(a) })
	select {
	case v := <-ch:
		return v, nil
	case <-ctx.Done():
		t.post(func() { t.dropAsk(a) })
		return zero, ctx.Err()
	case <-t.closed:
		return zero, ErrClosed
	}
}

func (t *Terminal) pushAsk(a asker) {
	if t.eof {
		a.noAnswer(t)
		return
	}
	t.asks = append(t.asks, a)
	if len(t.asks) == 1 {
		a.open(t)
	}
}

// dropAsk takes a away, answered or not, and opens the next.
func (t *Terminal) dropAsk(a asker) {
	for i, x := range t.asks {
		if x != a {
			continue
		}
		if q, ok := a.(*questionAsk); ok {
			q.giveLineBack(t)
		}
		t.asks = append(t.asks[:i], t.asks[i+1:]...)
		if i == 0 && len(t.asks) > 0 {
			t.asks[0].open(t)
		}
		return
	}
}

// Permission implements platformagent.Front.
func (t *Terminal) Permission(ctx context.Context, req platformagent.PermissionRequest) (platformagent.Decision, error) {
	ch := make(chan platformagent.Decision, 1)
	d, err := await(ctx, t, asker(&permissionAsk{req: req, answer: func(d platformagent.Decision) { ch <- d }}), ch)
	if err != nil {
		return platformagent.Deny, err
	}
	return d, nil
}

// OverBudget implements platformagent.Front.
func (t *Terminal) OverBudget(ctx context.Context, spent, budget float64) (bool, error) {
	ch := make(chan bool, 1)
	ask := fmt.Sprintf("Spent $%.2f of a $%.2f budget. Do you want to continue? (y/n)", spent, budget)
	return await(ctx, t, asker(&yesNoAsk{text: ask, answer: func(yes bool) { ch <- yes }}), ch)
}

// Questions implements platformagent.Front.
func (t *Terminal) Questions(ctx context.Context, qs []record.Question) ([]string, error) {
	type result struct {
		answers []string
		err     error
	}
	ch := make(chan result, 1)
	a := &questionAsk{qs: qs, answer: func(answers []string, err error) { ch <- result{answers, err} }}
	r, err := await(ctx, t, asker(a), ch)
	if err != nil {
		return nil, err
	}
	return r.answers, r.err
}

// permissionAsk asks to allow a tool call.
type permissionAsk struct {
	req    platformagent.PermissionRequest
	answer func(platformagent.Decision)
}

func (a *permissionAsk) open(t *Terminal) {
	t.sc.Line(t.sc.paint(styleYellow, "Do you want to allow "+oneLine(a.req.Tool)+"?"))
	detail := strings.Split(cleanText(a.req.Detail), "\n")
	const maxLines = 20
	if len(detail) > maxLines {
		detail = append(detail[:maxLines], fmt.Sprintf("… %d more lines", len(detail)-maxLines))
	}
	if d := strings.TrimSpace(strings.Join(detail, "\n")); d != "" {
		t.sc.Line(d)
	}
	if rule := oneLine(a.req.Rule); rule != "" {
		t.sc.Line(t.sc.paint(styleDim, "Always allow keeps: "+rule))
	}
	t.sc.Line(t.sc.paint(styleYellow, "1 allow  2 always allow  3 deny"))
}

func (a *permissionAsk) key(t *Terminal, k Key, burst, more bool) {
	if burst || !t.ed.empty() {
		if k.Kind == KeyEsc && !t.ed.empty() {
			t.ed.clear()
			return
		}
		a.toLine(t, k)
		return
	}
	done := func(d platformagent.Decision, said string) {
		t.dropAsk(a)
		t.sc.Line(t.sc.paint(styleDim, said))
		a.answer(d)
	}
	switch {
	case k.Kind == KeyRune && k.Rune == '1':
		done(platformagent.AllowOnce, "Allowed")
	case k.Kind == KeyRune && k.Rune == '2':
		done(platformagent.AllowAlways, "Always allowed")
	case k.Kind == KeyRune && k.Rune == '3', k.Kind == KeyEsc:
		done(platformagent.Deny, "Denied")
	default:
		a.toLine(t, k)
	}
}

// toLine is a key that doesn't answer: typing a message for after the turn.
func (a *permissionAsk) toLine(t *Terminal, k Key) {
	if k.Kind == KeyEsc {
		return // a burst's Esc isn't a deny, nor an interrupt
	}
	t.lineKey(k)
}

func (a *permissionAsk) noAnswer(t *Terminal) { a.answer(platformagent.Deny) }

// yesNoAsk asks y or n, for spending over budget and /clear.
type yesNoAsk struct {
	text   string
	answer func(bool)
}

func (a *yesNoAsk) open(t *Terminal) { t.sc.Line(t.sc.paint(styleYellow, a.text)) }

func (a *yesNoAsk) key(t *Terminal, k Key, burst, more bool) {
	if burst || !t.ed.empty() {
		if k.Kind != KeyEsc {
			t.lineKey(k)
		} else if !t.ed.empty() {
			t.ed.clear()
		}
		return
	}
	switch {
	case k.Kind == KeyRune && (k.Rune == 'y' || k.Rune == 'Y'):
		t.dropAsk(a)
		a.answer(true)
	case k.Kind == KeyRune && (k.Rune == 'n' || k.Rune == 'N'), k.Kind == KeyEsc:
		t.dropAsk(a)
		a.answer(false)
	case k.Kind == KeyCtrlC:
		t.ctrlC()
	}
}

func (a *yesNoAsk) noAnswer(t *Terminal) { a.answer(false) }

// questionAsk asks questions one after another; while it does, the line is its answer,
// and what had been typed waits in stash.
type questionAsk struct {
	qs      []record.Question
	answer  func([]string, error)
	at      int
	answers []string
	opened  bool
	free    bool   // the user's own words go on the line
	number  string // digits waiting for one more
}

func (a *questionAsk) open(t *Terminal) {
	a.opened = true
	t.stash = append([]rune(nil), t.ed.text...)
	t.ed.clear()
	a.showQuestion(t)
}

func (a *questionAsk) showQuestion(t *Terminal) {
	q := a.qs[a.at]
	head := oneLine(q.Question)
	if h := oneLine(q.Header); h != "" {
		head = "[" + h + "] " + head
	}
	if len(a.qs) > 1 {
		head += fmt.Sprintf(" (%d of %d)", a.at+1, len(a.qs))
	}
	t.sc.Line(t.sc.paint(styleYellow, head))
	for i, o := range q.Options {
		line := fmt.Sprintf("  %d. %s", i+1, oneLine(o))
		if i < len(q.About) && q.About[i] != "" {
			line += t.sc.paint(styleDim, ": "+oneLine(q.About[i]))
		}
		t.sc.Line(line)
	}
	if q.Multi {
		t.sc.Line(t.sc.paint(styleDim, "  Numbers with spaces or commas, or your own words, then Enter"))
	} else {
		t.sc.Line(fmt.Sprintf("  %d. Your own words, then Enter", len(q.Options)+1))
	}
}

func (a *questionAsk) key(t *Terminal, k Key, burst, more bool) {
	q := a.qs[a.at]
	switch k.Kind {
	case KeyCtrlC:
		t.ctrlC()
		return
	case KeyShiftTab:
		t.cycleMode()
		return
	}
	if q.Multi || a.free || !t.ed.empty() {
		a.lineKey(t, k)
		return
	}
	switch {
	case k.Kind == KeyRune && k.Rune >= '0' && k.Rune <= '9':
		a.digit(t, k.Rune, more)
	case k.Kind == KeyEnter:
		if a.number != "" {
			a.commit(t, false)
		}
	case k.Kind == KeyEsc:
		if a.number != "" {
			a.number = ""
			return
		}
		a.finish(t, nil, ErrDeclined)
	case k.Kind == KeyRune || k.Kind == KeyPaste:
		a.typeNumber(t)
		a.lineKey(t, k)
	}
}

// digit takes a digit of an option's number. With fewer than ten choices, it is whole.
func (a *questionAsk) digit(t *Terminal, r rune, more bool) {
	a.number += string(r)
	v, _ := strconv.Atoi(a.number)
	own := len(a.qs[a.at].Options) + 1
	if v < 1 || v > own {
		a.typeNumber(t)
		return
	}
	if v*10 <= own {
		if !more {
			t.numberC = time.After(t.cfg.NumberWait)
		}
		return
	}
	a.commit(t, more)
}

// timeUp is no second digit coming.
func (a *questionAsk) timeUp(t *Terminal) {
	if a.number != "" {
		a.commit(t, false)
	}
}

// commit acts on the number typed. An option's number with more text behind it is the
// start of the user's words, and not a pick: only the own-words option takes text.
func (a *questionAsk) commit(t *Terminal, more bool) {
	v, _ := strconv.Atoi(a.number)
	q := a.qs[a.at]
	t.numberC = nil
	if v == len(q.Options)+1 {
		a.number = ""
		a.free = true
		return
	}
	if more {
		a.typeNumber(t)
		return
	}
	a.number = ""
	a.next(t, q.Options[v-1])
}

// typeNumber puts the digits waiting on the line as words.
func (a *questionAsk) typeNumber(t *Terminal) {
	t.ed.insert(a.number)
	a.number = ""
	a.free = true
}

// lineKey is a key while the answer is typed.
func (a *questionAsk) lineKey(t *Terminal, k Key) {
	switch k.Kind {
	case KeyEnter:
		text := strings.TrimSpace(t.ed.String())
		if text == "" {
			return
		}
		q := a.qs[a.at]
		if q.Multi {
			if picks, ok := multiPicks(text, q.Options); ok {
				text = strings.Join(picks, ", ")
			}
		}
		t.ed.clear()
		a.next(t, text)
	case KeyEsc:
		if !t.ed.empty() {
			t.ed.clear()
			return
		}
		a.finish(t, nil, ErrDeclined)
	default:
		t.ed.apply(k)
	}
}

// multiPicks reads numbers with spaces or commas as options.
func multiPicks(text string, options []string) ([]string, bool) {
	fields := strings.FieldsFunc(text, func(r rune) bool { return r == ' ' || r == ',' })
	var picks []string
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(options) {
			return nil, false
		}
		picks = append(picks, options[n-1])
	}
	return picks, len(picks) > 0
}

// next keeps an answer and goes on to the next question.
func (a *questionAsk) next(t *Terminal, answer string) {
	t.sc.Line(t.sc.paint(styleDim, "  → "+oneLine(answer)))
	a.answers = append(a.answers, answer)
	a.free, a.number = false, ""
	a.at++
	if a.at < len(a.qs) {
		a.showQuestion(t)
		return
	}
	a.finish(t, a.answers, nil)
}

// giveLineBack puts back what was typed before it opened.
func (a *questionAsk) giveLineBack(t *Terminal) {
	if a.opened {
		a.opened = false
		t.ed.set(t.stash)
		t.stash, t.numberC = nil, nil
	}
}

func (a *questionAsk) finish(t *Terminal, answers []string, err error) {
	t.dropAsk(a)
	a.answer(answers, err)
}

func (a *questionAsk) noAnswer(t *Terminal) { a.answer(nil, ErrDeclined) }
