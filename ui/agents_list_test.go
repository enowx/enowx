package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
)

// Settings → Agents lists every agent, installed ones first and the rest
// grouped, with search; one that can't have a config says why in place of the
// button.
func TestAgentsPageListsEveryAgent(t *testing.T) {
	h := newCanvasHarness(t)
	a := h.app
	a.agents = agent.List()
	for i := range a.agents {
		a.agents[i].Installed = a.agents[i].ID == "codex"
	}
	st := a.accountsOf()
	tt := ui.NewTester(func(c *ui.Context) { ui.Scroll(c).Fill().Children(func() { a.agentsSection(c) }) }, 1000, 900)
	frames := func() {
		for range 6 {
			tt.Frame()
			time.Sleep(10 * time.Millisecond)
			fns := h.later
			h.later = nil
			for _, fn := range fns {
				fn()
			}
		}
	}
	frames()
	if st.agent != "codex" {
		t.Fatalf("first shown: %q", st.agent)
	}
	if !tt.HasText("New config") {
		t.Fatalf("an installed agent has no button: %q", tt.Texts())
	}
	if !tt.HasText("Not installed") || !tt.HasText("Jcode") {
		t.Fatalf("the rest aren't listed: %q", tt.Texts())
	}
	if n := len(a.configAgents(st)); n != len(agent.Presets) {
		t.Fatalf("%d of %d agents", n, len(agent.Presets))
	}
	st.agent = "freebuff"
	frames()
	if !tt.HasText("It can't have a config.") || newConfigOpens(tt, st) {
		t.Fatalf("freebuff: %q", tt.Texts())
	}
	st.agent = "jcode"
	frames()
	if !tt.HasText("It isn't installed on this host: install it to give it a config.") || newConfigOpens(tt, st) {
		t.Fatalf("jcode: %q", tt.Texts())
	}
	st.find = "jco"
	frames()
	if tt.HasText("Codex") {
		t.Fatalf("search left others: %q", tt.Texts())
	}
}

// newConfigOpens clicks the header's New config, which stays in its place, and
// says whether that opened a form.
func newConfigOpens(tt *ui.Tester, st *accountsState) bool {
	tt.Click("New config") //nolint:errcheck // a disabled button may refuse the click: that's the point
	tt.Frame()
	opened := st.edit != nil
	st.edit = nil
	return opened
}
