package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// A session's notice closes, and stays closed until it says something else.
func TestSessionBannerCloses(t *testing.T) {
	a := New(Options{})
	s := proto.SessionInfo{ID: "s1", SkipPermissions: true}
	tt := ui.NewTester(func(c *ui.Context) { a.sessionBanner(c, s) }, 800, 200)
	tt.Frame()
	if !tt.HasText("Skips permission prompts") {
		t.Fatalf("%q", tt.Texts())
	}
	if err := tt.Click("Close this notice"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Skips permission prompts") {
		t.Fatal("still shown")
	}
	s.ExitWhy = "It ended on a hang-up"
	s.Exited = true
	tt.Frame()
	if !tt.HasText("It ended on a hang-up") {
		t.Fatalf("what is new isn't shown: %q", tt.Texts())
	}
}
