//go:build !windows

package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The dock picks the panel's tool, and the tool's button folds it again; what
// doesn't fit in the dock is in its menu.
func TestDockPicksThePanelsTool(t *testing.T) {
	app := New(Options{})
	app.client = &client.Client{}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(600).Height(500).Background(theme.Background).TextColor(theme.Text).Children(func() { app.toolsDock(c) })
	}, 600, 500)
	tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%v: %q", err, tt.Texts())
		}
		tt.Frame()
	}
	click("Files")
	if !app.state.Panel.Open || app.state.Panel.Tab != panelFiles {
		t.Fatalf("Files: %+v", app.state.Panel)
	}
	click("Files")
	if app.state.Panel.Open {
		t.Fatal("Files again doesn't fold the panel")
	}
	click("More tools")
	click("Ports")
	if !app.state.Panel.Open || app.state.Panel.Tab != panelPorts {
		t.Fatalf("Ports: %+v", app.state.Panel)
	}
}

// Team chat has its own button above More tools, while the host is in a team,
// and is in the menu no more; the team's other tools stay there.
func TestDockHasTeamChatAtItsFoot(t *testing.T) {
	app := New(Options{})
	app.client = &client.Client{}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(600).Height(500).Background(theme.Background).TextColor(theme.Text).Children(func() { app.toolsDock(c) })
	}, 600, 500)
	tt.Frame()
	if _, ok := tt.Find("Team chat"); ok {
		t.Fatal("a chat with no team")
	}
	app.spaceList = &hostSpaces{host: app.host.ID, list: []proto.Space{{Team: "t1", Name: "enowX"}}}
	tt.Frame()
	chat, ok := tt.Find("Team chat")
	more, okMore := tt.Find("More tools")
	if !ok || !okMore || chat.Y >= more.Y || chat.W != more.W || chat.H != more.H {
		t.Fatalf("chat %+v %v, more %+v %v", chat, ok, more, okMore)
	}
	if err := tt.Click("More tools"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("Team projects"); !ok {
		t.Fatalf("the team's other tools left the menu: %q", tt.Texts())
	}
	if n := strings.Count(strings.Join(tt.Texts(), "\n"), "Team chat"); n > 1 {
		t.Fatalf("team chat %d times: %q", n, tt.Texts())
	}
	app.menu = widget.ContextMenu{}
	tt.Frame()
	if err := tt.Click("Team chat"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.state.Panel.Open || app.state.Panel.Tab != panelChat {
		t.Fatalf("after its button: %+v", app.state.Panel)
	}
}
