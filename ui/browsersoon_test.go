package ui

import (
	"net"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
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
