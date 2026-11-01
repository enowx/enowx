package widget

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// A card takes no focus, and just shown it answers neither a click already on
// its way nor Enter; armed, its safe answer is Tab's first stop.
func TestAskCardWaitsForTheUser(t *testing.T) {
	typed, armed := "", false
	refused, allowed := 0, 0
	ask := &Ask{Key: "q", Title: "Let it?", Buttons: []AskButton{
		{Kind: Secondary, Label: "Refuse", Do: func() { refused++ }},
		{Kind: Primary, Label: "Allow", Do: func() { allowed++ }},
	}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Fill().Children(func() {
			ui.TextInput(c, &typed).Label("Field").Width(200)
			AskCard(c, ask, 0, armed)
		})
	}, 800, 600)
	tt.Frame()
	if err := tt.Click("Field"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !tt.Focused("Field") {
		t.Fatal("the field lost the keyboard to the card")
	}
	if err := tt.Click("Allow"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if allowed != 0 {
		t.Fatal("a click on a card just shown answered it")
	}
	armed = true
	tt.Frame()
	if err := tt.Click("Field"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if !tt.Focused("Refuse") {
		t.Fatal("Tab's first stop in the card isn't its safe answer")
	}
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if !tt.Focused("Allow") {
		t.Fatal("Tab doesn't reach the approving button")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if allowed != 1 || refused != 0 {
		t.Fatalf("Tab and Enter: allowed %d, refused %d", allowed, refused)
	}
}
