package chatview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// chatState is a state the chat shows: its entries, the agent's, what it
// must read.
type chatState struct {
	name    string
	entries []proto.TranscriptEntry
	o       Options
	want    []string
}

var t0 = time.Date(2026, 10, 30, 14, 2, 0, 0, time.Local)

func chatStates() []chatState {
	ask := proto.TranscriptEntry{Index: 0, Kind: proto.TranscriptUser, Text: "Tambahin timeout di proxy installer", Time: t0}
	edit := proto.TranscriptEntry{Index: 2, Kind: proto.TranscriptTool, Text: "Edit", Detail: "/p/proxy/installer.go", Added: 3, Removed: 1, Done: true,
		DurationMs: 1100, Old: "resp, err := http.Get(url)", New: "ctx, cancel := context.WithTimeout(ctx, 30*time.Second)\ndefer cancel()\nresp, err := client.Do(req.WithContext(ctx))"}
	return []chatState{
		{name: "empty", o: Options{Agent: "claude", Fresh: true, Empty: func(c *ui.Context) { ui.Text(c, "Ask Claude Code anything") }},
			want: []string{"Ask Claude Code anything"}},
		{name: "writing", entries: []proto.TranscriptEntry{ask, {Index: 1, Kind: proto.TranscriptThinking, Text: "Find the call", Time: t0.Add(time.Second)}},
			o: Options{Agent: "claude", Working: true}, want: []string{"Working", "Thought"}},
		{name: "thinking", entries: []proto.TranscriptEntry{ask}, o: Options{Agent: "claude", Working: true}, want: []string{"Working"}},
		{name: "ready", entries: []proto.TranscriptEntry{{Index: 0, Kind: proto.TranscriptNotice, Text: "Session resumed", Time: t0}}, o: Options{Agent: "claude"}, want: []string{"Ready", "Session resumed"}},
		{name: "ended", entries: []proto.TranscriptEntry{ask, {Index: 1, Kind: proto.TranscriptAssistant, Text: "Done it.", Time: t0.Add(time.Minute)}}, o: Options{Agent: "claude", Ended: true}, want: []string{"Ended"}},
		{name: "tool", entries: []proto.TranscriptEntry{ask, {Index: 1, Kind: proto.TranscriptTool, Text: "Grep", Detail: "http.Get", Time: t0.Add(time.Second)}},
			o: Options{Agent: "claude", Working: true}, want: []string{"Searching", "http.Get"}},
		{name: "diff", entries: []proto.TranscriptEntry{ask, {Index: 1, Kind: proto.TranscriptTool, Text: "Read", Detail: "/p/proxy/installer.go", Done: true, DurationMs: 200}, edit,
			{Index: 3, Kind: proto.TranscriptAssistant, Text: "Saya pasang context **30 detik**."}},
			o: Options{Agent: "claude", OpenChanges: func(string) {}}, want: []string{"Done", "Worked", "Saya pasang context"}},
		{name: "permission", entries: []proto.TranscriptEntry{ask, {Index: 1, Kind: proto.TranscriptTool, Text: "Bash", Detail: "go test ./proxy/..."}},
			o: Options{Agent: "claude", Working: true, Waiting: true, Answer: func(string) {}}, want: []string{"Waiting for you", "The agent asks to run a command", "Allow"}},
		{name: "error", entries: []proto.TranscriptEntry{ask, {Index: 1, Kind: proto.TranscriptTool, Text: "Bash", Detail: "go test ./proxy/...", Done: true, Failed: true, Result: "FAIL proxy"},
			{Index: 2, Kind: proto.TranscriptAssistant, Text: "The tests fail; looking."}, {Index: 3, Kind: proto.TranscriptNotice, Text: "API error: overloaded"}},
			o: Options{Agent: "claude"}, want: []string{"1 failed", "API error: overloaded"}},
		{name: "long", entries: longChat(120), o: Options{Agent: "claude"}, want: []string{"Message 119"}},
	}
}

// longChat is n turns of the user's and the agent's.
func longChat(n int) []proto.TranscriptEntry {
	var out []proto.TranscriptEntry
	for i := range n {
		at := t0.Add(time.Duration(i) * time.Minute)
		out = append(out,
			proto.TranscriptEntry{Index: len(out), Kind: proto.TranscriptUser, Text: fmt.Sprintf("Message %d", i), Time: at},
			proto.TranscriptEntry{Index: len(out) + 1, Kind: proto.TranscriptTool, Text: "Read", Detail: "/p/a.go", Done: true, Time: at.Add(time.Second)},
			proto.TranscriptEntry{Index: len(out) + 2, Kind: proto.TranscriptAssistant, Text: strings.Repeat("An answer that runs a line or two. ", 3), Time: at.Add(2 * time.Second)},
		)
	}
	return out
}

// loaded is a chat showing entries, as if read from the agent.
func loaded(entries []proto.TranscriptEntry) *Chat {
	ch := New()
	ch.entries, ch.turns = entries, splitTurns(entries)
	ch.loaded, ch.supported = len(entries) > 0, true
	return ch
}

// Every state of the chat reads, wide and narrow, in every theme, light
// and dark.
func TestChatStates(t *testing.T) {
	defer theme.Apply(theme.Lookup("champagne", ui.Color{}))
	for _, st := range chatStates() {
		for _, size := range []struct {
			name string
			w    int
		}{{"wide", 1180}, {"narrow", 440}} {
			for _, p := range theme.Palettes {
				for _, light := range []bool{false, true} {
					theme.Apply(theme.LookupAppearance(p.ID, ui.Color{}, light))
					ch := loaded(st.entries)
					tt := ui.NewTester(func(c *ui.Context) {
						c.SetTheme(theme.Window())
						ui.Column(c).Width(float32(size.w)).Height(720).Background(theme.Background).TextColor(theme.Text).Children(func() { ch.View(c, st.o) })
					}, size.w, 720)
					tt.Frame()
					tt.Frame()
					for _, want := range st.want {
						if !hasTextLike(tt, want) {
							t.Fatalf("%s, %s, %s light %v: no %q in %q", st.name, size.name, p.ID, light, want, tt.Texts())
						}
					}
					if p.ID == "champagne" {
						mode := "dark"
						if light {
							mode = "light"
						}
						timelineShot(t, tt, fmt.Sprintf("chat-%s-%s-%s.png", st.name, size.name, mode))
					}
				}
			}
		}
	}
}

// hasTextLike says tt shows text containing s.
func hasTextLike(tt *ui.Tester, s string) bool {
	for _, x := range tt.Texts() {
		if strings.Contains(x, s) {
			return true
		}
	}
	return false
}

// Hundreds of messages draw a frame fast: the list draws what shows.
func TestLongChatDrawsFast(t *testing.T) {
	ch := loaded(longChat(400))
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1180).Height(720).Children(func() { ch.View(c, Options{Agent: "claude"}) })
	}, 1180, 720)
	tt.Frame()
	start := time.Now()
	for range 20 {
		ch.list.ScrollToEnd()
		tt.Frame()
	}
	if per := time.Since(start) / 20; per > 50*time.Millisecond {
		t.Fatalf("a frame of 400 turns takes %s", per)
	}
}

// The bar searches the chat, moving from match to match, and folds or
// opens every turn's work; the arrows bring the user's messages back.
func TestChatBarAndComposer(t *testing.T) {
	entries := chatStates()[3].entries
	entries = append(entries, longChat(3)...)
	for i := range entries {
		entries[i].Index = i
	}
	ch := loaded(entries)
	o := Options{Agent: "claude"}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1180).Height(720).Children(func() { ch.View(c, o) })
	}, 1180, 720)
	tt.Frame()
	if err := tt.Click("Search the chat"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	ch.find = "message"
	tt.Frame()
	if !tt.HasText("1 of 3") {
		t.Fatalf("matches: %q", tt.Texts())
	}
	if err := tt.Click("Next match (Enter)"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("2 of 3") {
		t.Fatalf("next: %q", tt.Texts())
	}
	ch.find = "installer.go"
	tt.Frame()
	if !ch.open[ch.turns[0].start] {
		t.Fatal("a match in a turn's work didn't open it")
	}
	if err := tt.Click("Fold every step"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if ch.anyOpen(ch.turns) {
		t.Fatal("not folded")
	}
	if got, ok := ch.recall("", true); !ok || got != "Message 2" {
		t.Fatalf("↑: %q %v", got, ok)
	}
	if got, ok := ch.recall("Message 2", true); !ok || got != "Message 1" {
		t.Fatalf("↑↑: %q %v", got, ok)
	}
	if got, ok := ch.recall("Message 1", false); !ok || got != "Message 2" {
		t.Fatalf("↓: %q %v", got, ok)
	}
	if _, ok := ch.recall("Message 2 changed", true); ok {
		t.Fatal("the arrows took a message being changed")
	}
}
