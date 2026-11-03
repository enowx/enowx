package activityview

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// conversation is two turns: one that edits and runs, one that fails.
func conversation(v *View) {
	t0 := time.Now().Add(-10 * time.Minute)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	v.apply(proto.Transcript{Supported: true, Rev: 9, Entries: []proto.TranscriptEntry{
		{Index: 0, Kind: proto.TranscriptUser, Text: "Add search ranking", Time: at(0)},
		{Index: 1, Kind: proto.TranscriptTool, Text: "Read", Detail: "/work/app/core/search/find.go", Done: true, Time: at(5)},
		{Index: 2, Kind: proto.TranscriptTool, Text: "Edit", Detail: "/work/app/core/search/rank.go", Added: 12, Removed: 2, Done: true, Time: at(20)},
		{Index: 3, Kind: proto.TranscriptTool, Text: "Bash", Detail: "go test ./...", Done: true, Time: at(40)},
		{Index: 4, Kind: proto.TranscriptAssistant, Text: "Done.", Time: at(42)},
		{Index: 5, Kind: proto.TranscriptUser, Text: "Now the docs", Time: at(100)},
		{Index: 6, Kind: proto.TranscriptTool, Text: "Edit", Detail: "/work/app/core/search/rank.go", Added: 1, Done: true, Time: at(110)},
		{Index: 7, Kind: proto.TranscriptTool, Text: "Write", Detail: "/elsewhere/notes.md", Failed: true, Done: true, Time: at(115)},
	}}, nil, "/work/app")
}

func TestTurnsAndFilesAddUp(t *testing.T) {
	v := New()
	conversation(v)
	if len(v.turns) != 2 || v.turns[0].prompt != "Add search ranking" || len(v.turns[0].tools) != 3 || v.turns[0].edits != 1 {
		t.Fatalf("turns = %+v", v.turns)
	}
	if v.turns[1].failed != 1 || v.turns[1].edits != 1 {
		t.Fatalf("second turn = %+v", v.turns[1])
	}
	// A failed write isn't an edit; two edits of one file add up.
	if len(v.files) != 1 || v.files[0] != (edited{path: "core/search/rank.go", added: 13, removed: 2, edits: 2}) {
		t.Fatalf("files = %+v", v.files)
	}
	if tools, failed, edits := v.totals(); tools != 5 || failed != 1 || edits != 2 {
		t.Fatalf("totals = %d %d %d", tools, failed, edits)
	}
}

func TestTurnsUnfoldAndFilesOpen(t *testing.T) {
	v := New()
	conversation(v)
	var opened string
	o := Options{Session: proto.SessionInfo{Dir: "/work/app"}, OpenFile: func(p string) { opened = p }, OpenDiff: func(p string) { opened = "diff " + p }}
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 520, 600)
	tt.Frame()
	for _, want := range []string{"Turns", "Tool calls", "Files edited", "core/search/rank.go", "+13", "Now the docs", "Add search ranking"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("go test ./...") {
		t.Error("a folded turn shows its tools")
	}
	if err := tt.Click("Add search ranking"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("go test ./...") {
		t.Fatalf("an unfolded turn: %q", tt.Texts())
	}
	if err := tt.Click("core/search/rank.go"); err != nil {
		t.Fatal(err)
	}
	if opened != "core/search/rank.go" {
		t.Fatalf("opened %q", opened)
	}
	// What it read opens as a file, what it edited as its changes, what it ran
	// nothing.
	if err := tt.Click("Read core/search/find.go"); err != nil {
		t.Fatal(err)
	}
	if opened != "core/search/find.go" {
		t.Fatalf("opened %q", opened)
	}
	if err := tt.Click("Edit core/search/rank.go"); err != nil {
		t.Fatal(err)
	}
	if opened != "diff core/search/rank.go" {
		t.Fatalf("opened %q", opened)
	}
	opened = ""
	if err := tt.Click("Bash go test ./..."); err != nil {
		t.Fatal(err)
	}
	if opened != "" {
		t.Fatalf("opened %q", opened)
	}
}

func TestUnsupportedAgentSaysWhy(t *testing.T) {
	v := New()
	v.apply(proto.Transcript{Reason: "Codex has no chat view yet; it shows in the terminal."}, nil, "")
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, Options{}) }, 520, 300)
	tt.Frame()
	if !tt.HasText("Codex has no chat view yet; it shows in the terminal.") {
		t.Fatalf("%q", tt.Texts())
	}
}
