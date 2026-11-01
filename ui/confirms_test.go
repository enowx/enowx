package ui

import (
	"strings"
	"testing"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
)

// What can't be taken back asks first, saying what goes, and nothing happens
// until the user says yes.
func TestLastingActionsAskFirst(t *testing.T) {
	app := New(Options{})
	asks := func(what string, ask func(), says string) {
		t.Helper()
		app.confirming = nil
		ask()
		if app.confirming == nil {
			t.Fatalf("%s didn't ask", what)
		}
		if !strings.Contains(app.confirming.msg, says) {
			t.Fatalf("%s says %q, nothing of %q", what, app.confirming.msg, says)
		}
	}
	link := proto.ProjectLink{ID: "l1", Source: proto.ProjectEnd{Host: "this Mac"}, Target: proto.ProjectEnd{Host: "enowxlabs"}}
	app.links.links = map[string]proto.ProjectLinkState{"l1": {ProjectLink: link}}
	asks("unlinking", func() { app.askUnlink(&linksForm{}, link) }, "Both copies stay")
	if _, kept := app.links.links["l1"]; !kept {
		t.Fatal("unlinked before the answer")
	}
	asks("removing a host", func() { app.askRemoveHost(hosts.Host{ID: "h1", Name: "enowxlabs"}) }, "Nothing on the host changes")
	asks("removing an automation", func() { app.askRemoveAutomation(proto.Automation{ID: "a1", Name: "Nightly"}) }, "stops running")
	a, b := proto.SessionInfo{ID: "a", Name: "A"}, proto.SessionInfo{ID: "b", Name: "B"}
	asks("keeping one of a Compare", func() { app.askKeep(a, []proto.SessionInfo{a, b}) }, "discarded for good")
}
