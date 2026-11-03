package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
	"github.com/enowx/enowx/ui/wrapcheck"
)

// Every simple confirm of the app is confirmDialog with other words, so these
// stand for them all: the shortest and longest it holds, one that destroys and
// one that doesn't. What is asked shows whole, inside the dialog, and only the
// action does it.
func TestConfirmDialogHoldsWhatItAsks(t *testing.T) {
	cases := []struct{ name, title, msg, action string }{
		{"confirm-destroys", "Remove this hook?", "Before each tool call. It stops running on every agent of this Mac.", "Remove"},
		{"confirm-plain", "Sign “MacBook Pro” in to your account?", "A desktop app, started 2 minutes ago from 203.0.113.9 (Jakarta). It started on another network: approve it only if you started it.", "Approve"},
		{"confirm-long", "Set up “Every night, on a project with a long name” from the team's kit?", strings.TrimSpace(strings.Repeat("Claude Code runs it every day at 02:00, with this prompt: review what changed and say what to fix. ", 4)), "Set it up"},
		{"confirm-warning", "Let Design's members start sessions on this Mac?", teamSessionsWarning, "Let them"},
	}
	for _, st := range cases {
		t.Run(st.name, func(t *testing.T) {
			a := New(Options{})
			done := 0
			a.confirm(st.title, st.msg, st.action, func() { done++ })
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme.Window())
				ui.Column(c).Width(1200).Height(720).Background(theme.Frame).TextColor(theme.Text).Children(func() { a.confirmDialog(c) })
			}, 1200, 720)
			tt.Frame()
			tt.Frame()
			for _, want := range []string{st.title, st.msg, "Cancel", st.action} {
				if !tt.HasText(want) {
					t.Errorf("no %q in %q", want, tt.Texts())
				}
			}
			if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
				t.Errorf("out of their parents:\n%s", strings.Join(out, "\n"))
			}
			shot(t, tt, st.name+".png")
			if err := tt.Click("Cancel"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if done != 0 || a.confirming != nil {
				t.Fatalf("Cancel: did it %d times, still asks: %v", done, a.confirming != nil)
			}
			a.confirm(st.title, st.msg, st.action, func() { done++ })
			tt.Frame()
			if err := tt.Click(st.action); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if done != 1 {
				t.Fatalf("%s did it %d times", st.action, done)
			}
		})
	}
}

// What can't be done yet says why, with OK alone (removing a project with
// running sessions).
func TestAlertWithOKAlone(t *testing.T) {
	open, chosen := true, -1
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(1200).Height(720).Background(theme.Frame).TextColor(theme.Text).Children(func() {
			if open {
				if i := widget.AlertDialog(c, &open, "Can't remove “antares” yet", "2 sessions are running in it. Stop them first.", "OK"); i >= 0 {
					chosen, open = i, false
				}
			}
		})
	}, 1200, 720)
	tt.Frame()
	tt.Frame()
	shot(t, tt, "confirm-ok-alone.png")
	if tt.HasText("Cancel") {
		t.Errorf("it offers Cancel: %q", tt.Texts())
	}
	if err := tt.Click("OK"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if chosen != 0 {
		t.Fatalf("chose %d", chosen)
	}
}
