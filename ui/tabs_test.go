package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// A session's tabs show in its strip, not in the session list, and
// picking one from elsewhere brings it to the front of its session.
func TestTabsShowInTheirSession(t *testing.T) {
	a := New(Options{})
	parent := proto.SessionInfo{ID: "s1", Project: "/code/app", Agent: "claude"}
	tab := proto.SessionInfo{ID: "s2", Project: "/code/app", Agent: "shell", Parent: "s1"}
	a.sessions = []proto.SessionInfo{parent, tab}
	a.agents = []proto.AgentInfo{{ID: "claude", Name: "Claude Code"}, {ID: "shell", Name: "Shell"}}

	if got := a.projectSessions("/code/app"); len(got) != 1 || got[0].ID != "s1" {
		t.Fatalf("project sessions %+v", got)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.tabStrip(c, parent) }, 600, 40)
	tt.Frame()
	if !tt.HasText("Claude Code") || !tt.HasText("Shell") {
		t.Fatalf("strip %q", tt.Texts())
	}
	if a.frontTab(parent).ID != "s1" {
		t.Fatal("a tab in front before one was picked")
	}
	a.selectSession(tab)
	if a.selected != "s1" || a.frontTab(parent).ID != "s2" {
		t.Fatalf("selected %q, front %q", a.selected, a.frontTab(parent).ID)
	}
	a.remove("s2")
	if a.frontTab(parent).ID != "s1" {
		t.Fatal("a removed tab stays in front")
	}
}

// Closing what is in front of a session closes that alone: with its session
// killed from elsewhere, nothing; a tab whose program runs only once asked; one
// that ended at once; and a killed session's tab leaves the strip and GOA's.
func TestClosingTheFrontTabOfAKilledSession(t *testing.T) {
	a := New(Options{})
	parent := proto.SessionInfo{ID: "s1", Project: "/code/app", Agent: "claude"}
	running := proto.SessionInfo{ID: "s2", Project: "/code/app", Agent: "shell", Parent: "s1"}
	ended := proto.SessionInfo{ID: "s3", Project: "/code/app", Agent: "shell", Parent: "s1", Exited: true}
	a.sessions = []proto.SessionInfo{parent, running, ended}
	a.agents = []proto.AgentInfo{{ID: "claude", Name: "Claude Code"}, {ID: "shell", Name: "Shell"}}
	a.selectSession(parent)

	a.closeFrontTab() // the session's own: nothing to close
	if a.confirming != nil || a.selected != "s1" {
		t.Fatalf("the session's own tab: %+v, selected %q", a.confirming, a.selected)
	}
	a.selectSession(running)
	a.closeFrontTab()
	if a.confirming == nil {
		t.Fatal("a running tab closed unasked")
	}
	a.confirming = nil
	a.selectSession(ended)
	a.closeFrontTab()
	if a.confirming != nil || a.frontTab(parent).ID == "s3" {
		t.Fatalf("an ended tab: asked %+v, front %q", a.confirming, a.frontTab(parent).ID)
	}

	a.remove("s2")
	tt := ui.NewTester(func(c *ui.Context) { a.tabStrip(c, parent) }, 600, 40)
	tt.Frame()
	if _, ok := tt.Find("a killed tab"); ok || len(a.tabsOf("s1")) != 1 {
		t.Fatalf("still shown: %q", tt.Texts())
	}

	a.remove("s1")
	if a.selected != "" {
		t.Fatalf("selected %q after its session was killed", a.selected)
	}
	a.closeFrontTab() // Cmd+W now: no tab, and the window stays, as nothing here closes it
	if a.confirming != nil || a.page != "" {
		t.Fatalf("with its session gone: %+v, page %q", a.confirming, a.page)
	}
}

// GOD of Agents' tab of a session goes when the session does.
func TestGOATabLeavesWithItsSession(t *testing.T) {
	a := New(Options{})
	a.goa.board.Session = "goa"
	key := "here\x00s9"
	a.goa.tabs = []*goaTab{{key: key, host: "here", id: "s9"}}
	tabs := a.centerOf("goa")
	tabs.tabs, tabs.front = []string{centerGOA + key}, centerGOA+key
	a.remove("s9")
	if len(a.goa.tabs) != 0 || len(a.centerOf("goa").tabs) != 0 || a.centerOf("goa").front != "" {
		t.Fatalf("still shown: %d, %+v", len(a.goa.tabs), a.centerOf("goa"))
	}
}
