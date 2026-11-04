package ui

import (
	"testing"
	"time"

	mygoui "github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/platformview"
	"github.com/enowx/enowx/ui/theme"
)

// Sessions of Claude Platform open on the chat, from the start, and remember leaving
// it; others stay on the terminal until asked.
func TestPlatformSessionsOpenOnChat(t *testing.T) {
	app := New(Options{})
	own := proto.SessionInfo{ID: "own", Agent: "claude-platform", Created: time.Now()}
	other := proto.SessionInfo{ID: "other", Agent: "claude", Created: time.Now()}
	app.sessions = []proto.SessionInfo{own, other}
	if !app.showsChat("own") || app.currentView(own) != viewChat {
		t.Fatal("not on its chat from the start")
	}
	if app.showsChat("other") {
		t.Fatal("another agent on its chat")
	}
	if !canChat(own) || len(app.sessionViews(own)) != 2 {
		t.Fatalf("the terminal is no tab: %v", app.sessionViews(own))
	}
	app.showView(own, viewTerminal)
	if app.showsChat("own") || app.currentView(own) != viewTerminal {
		t.Fatal("leaving the chat isn't kept")
	}
	app.showView(own, viewChat)
	if !app.showsChat("own") {
		t.Fatal("coming back")
	}
	app.toggleChat(other)
	app.toggleChat(other)
	if _, set := app.state.Chats["other"]; set {
		t.Fatal("another agent's state changed form")
	}
}

// The key state follows the vault, and the sheet shows names, never values; its form is
// the vault's.
func TestPlatformKeysAndSheet(t *testing.T) {
	app := New(Options{})
	v := auditVault(t, app)
	app.page = ""
	if app.platformKeyState() != platformview.KeyMissing {
		t.Fatal("no vault is no key")
	}
	if err := app.vaultCreate("audit password", false); err != nil {
		t.Fatal(err)
	}
	if _, err := v.data.Add(vault.Credential{Platform: "github", Name: "gh", Type: vault.TypeToken, Fields: map[string]string{"token": "made-up-1"}}); err != nil {
		t.Fatal(err)
	}
	if app.platformKeyState() != platformview.KeyMissing {
		t.Fatal("another platform's key counted")
	}
	if _, err := v.data.Add(vault.Credential{Platform: "anthropic", Name: "work-key", Type: vault.TypeAPIKey, Fields: map[string]string{"api_key": "made-up-secret-value"}}); err != nil {
		t.Fatal(err)
	}
	if app.platformKeyState() != platformview.KeyHave {
		t.Fatal("the key isn't seen")
	}
	app.host.Name = "this Mac"
	app.openPlatform(platformview.TabKeys)
	tt := mygoui.NewTester(func(c *mygoui.Context) {
		c.SetTheme(theme.Window())
		mygoui.Column(c).Size(1000, 800).Background(theme.Background).TextColor(theme.Text).Children(func() {
			app.platformSheet(c)
			app.confirmDialog(c)
		})
	}, 1000, 800)
	for range 3 {
		tt.Frame()
	}
	if !tt.HasText("work-key") {
		t.Fatalf("no key: %q", tt.Texts())
	}
	for _, x := range tt.Texts() {
		if x == "made-up-secret-value" {
			t.Fatal("a value shows")
		}
	}
	if err := tt.Click("Remove work-key"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tt.Frame()
	}
	if app.confirming == nil {
		t.Fatalf("no confirmation: %q", tt.Texts())
	}
	if _, ok := tt.Find("Remove"); !ok {
		t.Fatalf("the confirmation doesn't show over the sheet: %q", tt.Texts())
	}
	app.confirming = nil
	v.view.OpenAdd("anthropic")
	for range 3 {
		tt.Frame()
	}
	if !tt.HasText("Add Anthropic") {
		t.Fatalf("the vault's form doesn't open: %q", tt.Texts())
	}
}
