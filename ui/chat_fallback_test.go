package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// A chat chosen while Claude Code ran a session isn't shown once OpenCode, with none,
// runs it: the terminal is, as the header's toggle back is hidden (issue #49).
func TestChatFallsBackToTheTerminal(t *testing.T) {
	claude := proto.SessionInfo{ID: "g", Agent: "claude"}
	opencode := proto.SessionInfo{ID: "g", Agent: "opencode"}
	for _, c := range []struct {
		name             string
		s                proto.SessionInfo
		known, on, chose bool
		want             bool
	}{
		{"chosen on an agent with a chat", claude, true, true, true, true},
		{"chosen, then the agent has none", opencode, true, true, true, false},
		{"left on an agent with a chat", claude, true, false, true, false},
		{"not chosen, an agent with its own chat", proto.SessionInfo{ID: "p", Agent: "claude-platform"}, true, false, false, true},
		{"not in the list yet, chosen", proto.SessionInfo{}, false, true, true, true},
	} {
		if got := chatShown(c.s, c.known, c.on, c.chose); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

// Switching a session to an agent without a chat forgets its chat, kept or not; to one
// with a chat keeps it. The body then draws the terminal's place, not an empty chat.
func TestSwitchToAnAgentWithoutAChatForgetsIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app := New(Options{})
	app.state.Chats = map[string]bool{"g": true, "h": true}
	app.upsert(proto.SessionInfo{ID: "g", Agent: "claude"})
	app.upsert(proto.SessionInfo{ID: "h", Agent: "claude"})
	app.upsert(proto.SessionInfo{ID: "g", Agent: "opencode"})
	app.upsert(proto.SessionInfo{ID: "h", Agent: "codex"})
	if _, kept := app.state.Chats["g"]; kept {
		t.Error("OpenCode's session kept the chat chosen on Claude Code")
	}
	if !app.state.Chats["h"] {
		t.Error("a switch to an agent with a chat lost it")
	}
	g, _ := app.sessionByID("g")
	if app.currentView(g) != viewTerminal {
		t.Fatalf("view: %s", app.currentView(g))
	}
	// Chosen again by something that doesn't know the agent changed: still the terminal.
	app.state.Chats["g"] = true
	tt := ui.NewTester(func(c *ui.Context) { app.sessionBody(c, g) }, 800, 500)
	tt.Frame()
	shot(t, tt, "chat-fallback-opencode.png")
	if !tt.HasText("Connecting…") {
		t.Errorf("not the terminal's place: %q", tt.Texts())
	}
	if app.showsChat("g") || app.currentView(g) != viewTerminal {
		t.Fatal("the chat shows on an agent without one")
	}
}
