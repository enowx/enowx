package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The stall limits' dialog opens from both its ways in, Settings → New
// sessions' Change… and GOD of Agents' Stall limits…, and what is saved in
// it is what the host answers.
func TestStallLimitsFromSettingsAndGOA(t *testing.T) {
	app, posted := laterApp(t)
	app.host.ID, app.host.Name = "local", "this Mac"
	app.settingsSection = sectionSessions
	app.agents = app.agents[:2] // every preset's row would push the stalled tasks' card out of the window
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).FillWidth().FillHeight().Background(theme.Background).Children(func() {
			app.goaHeader(c, proto.SessionInfo{}, proto.SessionInfo{})
			app.settingsPage(c)
			app.goaStallDialog(c)
		})
	}, 1400, 1400)
	frames := func(what string, until func() bool) {
		t.Helper()
		drain(t, tt, posted, func() bool {
			tt.Advance(50 * time.Millisecond)
			return until()
		}, what)
	}
	loaded := func() bool {
		return app.goa.stall != nil && app.goa.stall.loaded && tt.HasText("When a task counts as stalled")
	}
	saved := func(what string, want proto.GOAStall) {
		t.Helper()
		if err := tt.Click("Save"); err != nil {
			t.Fatalf("%s: %v (%q)", what, err, tt.Texts())
		}
		frames(what+" saved", func() bool { return app.goa.stall == nil })
		var got proto.GOAStall
		if err := app.client.Call(proto.MethodGOAStall, nil, &got); err != nil || got != want {
			t.Fatalf("%s: goa.stall answers %+v, want %+v (%v)", what, got, want, err)
		}
	}

	// Two cards of the section say Change…: this one is the stalled tasks'.
	frames("the section", func() bool { return tt.HasText("Stalled tasks on this Mac") })
	tt.Frame() // the lowest cards take the frame's height from the frame before
	tt.Frame()
	if err := tt.Click("Change the stall limits"); err != nil {
		t.Fatalf("the card: %v (%q)", err, tt.Texts())
	}
	frames("the dialog from Settings", loaded)
	app.goa.stall.idle = "5m"
	tt.Frame()
	saved("from Settings", proto.GOAStall{Blank: 60, Idle: 300, Stuck: 600})

	if err := tt.Click("More for GOD of Agents"); err != nil {
		t.Fatal(err)
	}
	frames("its menu", func() bool { return tt.HasText("Stall limits…") })
	if err := tt.Click("Stall limits…"); err != nil {
		t.Fatal(err)
	}
	frames("the dialog from GOD of Agents", loaded)
	if f := app.goa.stall; f.idle != "5m" {
		t.Fatalf("the limit saved isn't read back: %+v", f)
	}
	app.goa.stall.stuck = "20m"
	tt.Frame()
	saved("from GOD of Agents", proto.GOAStall{Blank: 60, Idle: 300, Stuck: 1200})
}
