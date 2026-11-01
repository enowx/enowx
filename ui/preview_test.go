package ui

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

func TestAlwaysPreviewedPortsOpenTheBrowser(t *testing.T) {
	if !webViewAvailable() {
		t.Skip("no web view here")
	}
	a := New(Options{})
	a.client = &client.Client{}
	s := proto.SessionInfo{ID: "s1", Project: "/code/app"}
	other := proto.SessionInfo{ID: "s2", Project: "/code/app"}
	a.sessions = []proto.SessionInfo{s, other}
	a.selected = "s1"
	a.setAlwaysPreview(s, 5173, true)

	a.portsChanged(proto.PortsChange{Session: "s1", Opened: []proto.Port{{Port: 5173}}, Ports: []proto.Port{{Port: 5173}}})
	if len(a.portOffers) != 0 || a.panelShown() || !a.browserNews {
		t.Fatalf("offers %+v, panel shown %v, news %v", a.portOffers, a.panelShown(), a.browserNews)
	}
	// Another port, or the port in a session not shown, is offered.
	a.portsChanged(proto.PortsChange{Session: "s1", Opened: []proto.Port{{Port: 8080}}, Ports: []proto.Port{{Port: 5173}, {Port: 8080}}})
	a.portsChanged(proto.PortsChange{Session: "s2", Opened: []proto.Port{{Port: 5174}}, Ports: []proto.Port{{Port: 5174}}})
	if len(a.portOffers) != 2 {
		t.Fatalf("offers %+v", a.portOffers)
	}
	a.setAlwaysPreview(s, 5173, false)
	if a.alwaysPreviews(s, 5173) {
		t.Fatal("still always previewed")
	}
}

func TestBrowserIsASessionView(t *testing.T) {
	if !webViewAvailable() {
		t.Skip("no web view here")
	}
	a := New(Options{})
	a.client = &client.Client{}
	s := proto.SessionInfo{ID: "s1", Agent: "claude"}
	a.sessions = []proto.SessionInfo{s}
	a.selected = "s1"
	if views := a.sessionViews(s); !slices.Contains(views, viewBrowser) {
		t.Fatalf("views %q", views)
	}
	a.showView(s, viewBrowser)
	if a.currentView(s) != viewBrowser {
		t.Fatal("not shown")
	}
	// The panel's Browser says where the page is instead of showing it
	// twice.
	a.openPanel(panelBrowser)
	tt := ui.NewTester(func(c *ui.Context) { a.sidePanelView(c) }, 500, 400)
	tt.Frame()
	if !tt.HasText("The preview shows as “claude”'s view. Switch it to Terminal or Chat to have it here.") {
		t.Fatalf("%q", tt.Texts())
	}
	a.showView(s, viewTerminal)
	if a.currentView(s) != viewTerminal {
		t.Fatal("still the browser")
	}
}
