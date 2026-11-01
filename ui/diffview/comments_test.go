package diffview

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCommentsGoToTheAgentAsOnePrompt(t *testing.T) {
	v := loaded()
	var sent, news string
	o := Options{Send: func(text string) { sent = text }, Notify: func(m string) { news = m }}
	tt := ui.NewTester(func(c *ui.Context) { v.DrawFile(c, o, "a.go") }, 900, 600)
	tt.Frame()

	// Click the added line, write a comment, add it.
	if err := tt.Click("var x = 2"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("Use a constant")
	tt.Frame()
	if err := tt.Click("Add comment"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// And one on the removed line.
	if err := tt.Click("var x = 1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("Why was this removed?")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText("Use a constant") || !tt.HasText("2 review comments") {
		t.Fatalf("after commenting: %q", tt.Texts())
	}

	if err := tt.Click("Send to agent"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, want := range []string{
		"Review comments on your changes on feat/x (compared with main)",
		"1. a.go:2 (`var x = 2`)\n   Use a constant",
		"2. a.go, removed line 2 (`var x = 1`)\n   Why was this removed?",
	} {
		if !strings.Contains(sent, want) {
			t.Errorf("prompt lacks %q:\n%s", want, sent)
		}
	}
	if news == "" || tt.HasText("Send to agent") {
		t.Fatalf("after sending: news %q, %q", news, tt.Texts())
	}
}
