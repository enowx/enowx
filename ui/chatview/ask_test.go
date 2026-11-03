package chatview

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// While the agent waits, the tool it asks to use shows to allow or deny,
// and its question with its options: each answer the keys its terminal
// takes.
func TestAsksAreAnsweredFromTheChat(t *testing.T) {
	var sent []string
	o := Options{Agent: "claude", Waiting: true, Answer: func(keys string) { sent = append(sent, keys) }}
	ch := New()
	run := splitTurns([]proto.TranscriptEntry{
		{Index: 0, Kind: proto.TranscriptUser, Text: "Jalankan tesnya"},
		{Index: 1, Kind: proto.TranscriptTool, Text: "Bash", Detail: "go test ./proxy/..."},
	})[0]
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(760).Height(600).Padding(20).Background(theme.Background).TextColor(theme.Text).Children(func() {
			ch.View(c.Key("view"), o)
			ch.turnTimeline(c.Key("t"), run, true)
		})
	}, 760, 600)
	tt.Frame()
	if !tt.HasText("The agent asks to run a command") || !tt.HasText("go test ./proxy/...") {
		t.Fatalf("no permission card: %q", tt.Texts())
	}
	timelineShot(t, tt, "chat-permission.png")
	if err := tt.Click("Allow"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(sent) != 1 || sent[0] != "1" || !tt.HasText("Sent") {
		t.Fatalf("allowed: %q, %q", sent, tt.Texts())
	}

	run = splitTurns([]proto.TranscriptEntry{
		{Index: 2, Kind: proto.TranscriptUser, Text: "Bikin database"},
		{Index: 3, Kind: proto.TranscriptTool, Text: "AskUserQuestion", Detail: "Which database?", Questions: []proto.TranscriptQuestion{
			{Header: "Database", Question: "Which database?", Options: []string{"Postgres", "SQLite"}, About: []string{"What the project runs", "No server"}},
			{Question: "Migrations?", Options: []string{"Yes", "No"}},
		}},
	})[0]
	tt.Frame()
	if !tt.HasText("Which database?") || !tt.HasText("No server") || !tt.HasText("1 of 2") {
		t.Fatalf("no question card: %q", tt.Texts())
	}
	timelineShot(t, tt, "chat-question.png")
	if err := tt.Click("SQLite"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if sent[len(sent)-1] != "2" || !tt.HasText("Migrations?") {
		t.Fatalf("picked: %q, %q", sent, tt.Texts())
	}
	ch.ownAnswer = "Nanti saja"
	tt.Frame()
	if err := tt.Click("Send"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if sent[len(sent)-1] != "3Nanti saja\r" || !tt.HasText("Answered") {
		t.Fatalf("own answer: %q, %q", sent, tt.Texts())
	}

	// An agent whose keys aren't known is answered in its terminal.
	o.Agent = "codex"
	shown := false
	o.ShowTerminal = func() { shown = true }
	ch2 := New()
	tt2 := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(760).Height(400).Children(func() {
			ch2.View(c.Key("view"), o)
			ch2.turnTimeline(c.Key("t"), splitTurns([]proto.TranscriptEntry{{Index: 1, Kind: proto.TranscriptTool, Text: "Bash", Detail: "rm -rf build"}})[0], true)
		})
	}, 760, 400)
	tt2.Frame()
	if tt2.HasText("Allow") {
		t.Fatal("allowing an agent whose keys aren't known")
	}
	if err := tt2.Click("Answer in the terminal"); err != nil {
		t.Fatal(err)
	}
	if !shown {
		t.Fatal("the terminal wasn't shown")
	}
}
