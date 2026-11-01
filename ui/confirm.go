package ui

import (
	"github.com/egoist/mygo/ui"
	"github.com/enowx/enowx/ui/widget"
)

// A destructive action the user just asked for (signing out, removing someone,
// leaving) asks first: confirm sets what to ask, confirmDialog asks it over
// the window. What an agent asks waits as a card instead (asks.go).

type confirmation struct {
	title, msg, action string
	do                 func()
}

// confirm asks title and msg, running do when the user clicks action.
func (a *App) confirm(title, msg, action string, do func()) {
	a.confirming = &confirmation{title: title, msg: msg, action: action, do: do}
}

func (a *App) confirmDialog(c *ui.Context) {
	q := a.confirming
	if q == nil {
		return
	}
	open := true
	switch widget.AlertDialog(c, &open, q.title, q.msg, "Cancel", q.action) {
	case 1:
		a.confirming = nil
		q.do()
	case 0, ui.AlertDismissed:
		a.confirming = nil
	}
	if !open && a.confirming == q {
		a.confirming = nil
	}
}
