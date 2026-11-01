package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The sidebar folds to icons and back: folded, each project is a group of
// its sessions, every session still there by its full name, a click
// selects it, and New sits in the current project's group.
func TestSidebarFoldsToIcons(t *testing.T) {
	app := New(Options{})
	app.state.RailWide = true
	app.projects = []proto.ProjectInfo{{Name: "cryptgo", Path: "/p/cryptgo"}, {Name: "enxapi", Path: "/p/enxapi"}}
	app.sessions = []proto.SessionInfo{
		{ID: "s1", Name: "Refactor the payment webhooks", Agent: "claude", Project: "/p/cryptgo", Branch: "main"},
		{ID: "s2", Name: "ai", Agent: "codex", Project: "/p/cryptgo", Exited: true},
	}
	app.project = "/p/cryptgo"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(600).Height(600).Background(theme.Ground).TextColor(theme.Text).Children(func() {
			if app.navShown() {
				app.navSidebar(c)
			} else {
				app.navSidebarNarrow(c)
			}
		})
	}, 600, 600)
	tt.Frame()
	if err := tt.Click("Collapse the sidebar (" + modKey + "B)"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second) // its width done moving
	tt.Frame()
	shot(t, tt, "sidebar-folded.png")
	if app.state.RailWide || tt.HasText("Sessions only") {
		t.Fatalf("not folded: %q", tt.Texts())
	}
	r, ok := tt.Find("Refactor the payment webhooks")
	if !ok || r.X+r.W > navNarrow {
		t.Fatalf("the session isn't there folded, by its name: %v %v", ok, r)
	}
	if err := tt.Click("Refactor the payment webhooks"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.selected != "s1" {
		t.Fatalf("selected %q", app.selected)
	}
	if err := tt.Click("Expand the sidebar (" + modKey + "B)"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.state.RailWide || !tt.HasText("Sessions only") {
		t.Fatalf("not unfolded: %q", tt.Texts())
	}
}

// A project's initials: two words' first letters, else its first two.
func TestProjectInitials(t *testing.T) {
	for name, want := range map[string]string{"enxapi": "EN", "Fondasi dashboard": "FD", "my-app": "MA", "x": "X"} {
		if got := projectInitials(name); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
}

// The list by state groups every project's sessions, naming their project; by
// project it nests them; the choice is kept with the app's state.
func TestSessionListModes(t *testing.T) {
	app := New(Options{})
	app.state.RailWide = true
	app.projects = []proto.ProjectInfo{{Path: "/a", Name: "alpha"}, {Path: "/b", Name: "beta"}}
	app.sessions = []proto.SessionInfo{
		{ID: "s1", Name: "One", Project: "/a", State: proto.StateWaiting},
		{ID: "s2", Name: "Two", Project: "/b", State: proto.StateWorking},
		{ID: "s3", Name: "Three", Project: "/a", Exited: true, ExitCode: 3},
		{ID: "s4", Name: "Four", Project: "/b", Exited: true},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(600).Height(600).Children(func() { app.navSidebar(c) })
	}, 600, 600)
	tt.Frame()
	for _, want := range []string{"Waiting on you", "Working", "Failed", "Done", "alpha", "beta"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q by state: %q", want, tt.Texts())
		}
	}
	if err := tt.Click("By project"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.state.SessionsByProject || tt.HasText("Waiting on you") {
		t.Fatalf("by project: %q", tt.Texts())
	}
	for _, want := range []string{"One", "Two", "Three", "Four"} {
		if !tt.HasText(want) {
			t.Fatalf("no session %q by project: %q", want, tt.Texts())
		}
	}
	// A folded project still says what needs the user.
	app.collapsed["/a"] = true
	tt.Frame()
	if !tt.HasText("1 waiting") || !tt.HasText("1 failed") {
		t.Fatalf("folded: %q", tt.Texts())
	}
}
