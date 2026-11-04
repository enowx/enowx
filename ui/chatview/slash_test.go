package chatview

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

func TestSlashMatchesPrefixFirst(t *testing.T) {
	ch := New()
	ch.commands = []proto.AgentCommand{
		{Name: "compact"}, {Name: "model"}, {Name: "remote-model"}, {Name: "review", Description: "pick a model to review"},
	}
	ch.input = "/mo"
	got := ch.slashMatches()
	if len(got) != 3 || got[0].Name != "model" || got[1].Name != "remote-model" || got[2].Name != "review" {
		t.Fatalf("matches = %+v", got)
	}
	for _, in := range []string{"/model ", "hello", "/a\nb"} {
		ch.input = in
		if m := ch.slashMatches(); m != nil {
			t.Errorf("%q matched %+v", in, m)
		}
	}
	ch.input = "/mo"
	ch.slashClosed = "/mo"
	if ch.slashMatches() != nil {
		t.Error("a closed menu matched")
	}
}

// The menu goes with Escape and with a click elsewhere, and a click on it picks.
func TestSlashMenuClosesAndPicks(t *testing.T) {
	ch := New()
	ch.commandsRead = true
	ch.commands = []proto.AgentCommand{{Name: "compact"}, {Name: "model"}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(1100).Height(720).Background(theme.Frame).TextColor(theme.Text).Children(func() {
			ch.View(c, Options{Agent: "claude", AgentName: "Claude Code"})
		})
	}, 1100, 720)
	frames := func() {
		for range 3 {
			tt.Frame()
		}
	}
	frames()
	tt.Type("/mo")
	frames()
	if !tt.HasText("/model") {
		t.Fatalf("no menu: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	frames()
	if tt.HasText("/model") {
		t.Error("Escape left the menu open")
	}
	ch.slashClosed = ""
	frames()
	if !tt.HasText("/model") {
		t.Fatal("no menu again")
	}
	tt.ClickAt(550, 60)
	frames()
	if tt.HasText("/model") {
		t.Error("a click elsewhere left the menu open")
	}
	ch.slashClosed = ""
	frames()
	b, _ := tt.Find("/model")
	tt.Move(b.X+b.W/2, b.Y+b.H/2)
	tt.Frame()
	tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	frames()
	if ch.input != "/model " {
		t.Errorf("a click on the menu left the input %q", ch.input)
	}
}
