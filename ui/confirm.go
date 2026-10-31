package ui

import "github.com/egoist/mygo/ui"

// A destructive action (signing out, removing someone, leaving) asks
// first: confirm sets what to ask, confirmDialog asks it over the window.

type confirmation struct {
	title, msg, action string
	do                 func()
	cancel             func() // when the user says no; may be nil
}

// confirm asks title and msg, running do when the user clicks action.
func (a *App) confirm(title, msg, action string, do func()) {
	a.confirmOr(title, msg, action, do, nil)
}

// confirmOr is confirm, running cancel when the user says no, or when
// another question takes this one's place.
func (a *App) confirmOr(title, msg, action string, do, cancel func()) {
	if q := a.confirming; q != nil && q.cancel != nil {
		q.cancel()
	}
	a.confirming = &confirmation{title: title, msg: msg, action: action, do: do, cancel: cancel}
}

func (a *App) confirmDialog(c *ui.Context) {
	q := a.confirming
	if q == nil {
		return
	}
	open := true
	switch ui.AlertDialog(c, &open, q.title, q.msg, "Cancel", q.action) {
	case 1:
		a.confirming = nil
		q.do()
	case 0, ui.AlertDismissed:
		a.confirming = nil
		if q.cancel != nil {
			q.cancel()
		}
	}
	if !open && a.confirming == q {
		a.confirming = nil
		if q.cancel != nil {
			q.cancel()
		}
	}
}
