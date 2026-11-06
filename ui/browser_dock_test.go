package ui

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// The page shows where the user put it, docked in the side panel or in a tab
// of its own; the choice holds for every session, survives reopening the
// app, and previews asked for open there.
func TestBrowserPlaceIsKept(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	a := New(Options{StateFile: file})
	a.host = LocalHost()
	a.client = &client.Client{}
	s := proto.SessionInfo{ID: "s1", Project: "/code/app"}
	other := proto.SessionInfo{ID: "s2", Project: "/code/app"}
	a.sessions = []proto.SessionInfo{s, other}
	a.selected = s.ID
	if a.state.BrowserInPanel {
		t.Fatal("docked by default: a page opens in a tab of its own")
	}
	a.previewPort(s, proto.Port{Port: 5173, Addr: "127.0.0.1"})
	a.browserTabBehind(other)

	a.dockBrowser(s, true)
	if !a.panelShows(panelBrowser) || !a.browserDocked(s) || !a.browserDocked(other) {
		t.Fatalf("docking: panel %q, docked %v", a.panelTab(), a.state.BrowserInPanel)
	}
	for _, id := range []string{s.ID, other.ID} {
		if slices.Contains(a.centerOf(id).tabs, centerBrowser) {
			t.Fatalf("%s kept its page's tab: the page shows in one place", id)
		}
	}
	a.openPanel(panelChanges)
	a.previewPort(s, proto.Port{Port: 5173, Addr: "127.0.0.1"})
	a.browserTabBehind(other)
	if !a.panelShows(panelBrowser) || a.centerFront(s) != "" || len(a.centerOf(other.ID).tabs) != 0 {
		t.Fatalf("a preview docked: panel %q, front %q, other's tabs %v", a.panelTab(), a.centerFront(s), a.centerOf(other.ID).tabs)
	}
	a.saveState()
	if reopened := New(Options{StateFile: file}); !reopened.state.BrowserInPanel {
		t.Fatal("docking didn't survive reopening the app")
	}

	// As the session's view, the page is there, whatever the choice.
	a.state.BrowserViews = map[string]bool{s.ID: true}
	if a.browserDocked(s) {
		t.Fatal("the session shows the page as its view")
	}
	a.dockBrowser(s, false)
	if a.state.BrowserInPanel || a.state.BrowserViews[s.ID] || a.centerFront(s) != centerBrowser {
		t.Fatalf("popping out: docked %v, view %v, front %q", a.state.BrowserInPanel, a.state.BrowserViews[s.ID], a.centerFront(s))
	}
	a.saveState()
	if reopened := New(Options{StateFile: file}); reopened.state.BrowserInPanel {
		t.Fatal("popping out didn't survive reopening the app")
	}
}
