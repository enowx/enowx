package ui

import (
	"net"
	"slices"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// A page opened by itself (an agent's, a port always previewed) doesn't
// take the panel from the tool the user is at: the Browser's button gets
// a dot instead, until looked at; nor does it leave a page shown.
func TestBrowserDoesntTakeThePanel(t *testing.T) {
	app := New(Options{})
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a, b := net.Pipe()
	go srv.ServeConn(a)
	cl, err := client.New(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	app.client = cl
	app.openPanel(panelChat)
	app.showBrowserSoon()
	if app.panelTab() != panelChat || !app.browserNews {
		t.Fatalf("tab %s, news %v", app.panelTab(), app.browserNews)
	}
	app.page = pageNotes
	app.state.Panel.Open = false
	app.showBrowserSoon()
	if app.page != pageNotes || app.state.Panel.Open {
		t.Fatal("a page shown was left for the Browser")
	}
	app.page, app.browserNews = "", false
	app.showBrowserSoon()
	if app.panelShown() || !app.browserNews {
		t.Fatal("an agent's page opened the panel: it only marks the Browser")
	}
}

// The page shows in a center tab: a preview asked for comes to the front and
// leaves the panel as it was; one opened by itself waits behind what is in front.
func TestPageOpensInACenterTab(t *testing.T) {
	a := New(Options{})
	a.client = &client.Client{}
	s := proto.SessionInfo{ID: "s1", Project: "/code/app"}
	a.sessions = []proto.SessionInfo{s}
	a.selected = s.ID
	a.openPanel(panelChanges)

	a.browserTabBehind(s)
	if a.centerFront(s) != "" || !slices.Contains(a.centerOf(s.ID).tabs, centerBrowser) {
		t.Fatalf("behind: front %q, tabs %v", a.centerFront(s), a.centerOf(s.ID).tabs)
	}
	a.previewPort(s, proto.Port{Port: 5173, Addr: "127.0.0.1"})
	if a.centerFront(s) != centerBrowser || a.panelTab() != panelChanges || len(a.centerOf(s.ID).tabs) != 1 {
		t.Fatalf("asked for: front %q, panel %q, tabs %v", a.centerFront(s), a.panelTab(), a.centerOf(s.ID).tabs)
	}
}
