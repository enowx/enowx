package chatview

import (
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// A turn is a timeline: the user's message, the work folded once done
// into a line that opens to each step, the answer; the turn under way
// shows its steps, the latest lit.
func TestTurnTimeline(t *testing.T) {
	at := time.Date(2026, 10, 30, 14, 2, 0, 0, time.Local)
	entries := []proto.TranscriptEntry{
		{Index: 0, Kind: proto.TranscriptUser, Text: "Proxy installer-nya suka hang. Tambahin timeout ya.", Time: at},
		{Index: 1, Kind: proto.TranscriptThinking, Text: "hmm", DurationMs: 8000, Time: at.Add(8 * time.Second)},
		{Index: 2, Kind: proto.TranscriptTool, Text: "Read", Detail: "/p/proxy/installer.go", Done: true},
		{Index: 3, Kind: proto.TranscriptAssistant, Text: "Hang-nya dari `http.Get` tanpa batas waktu."},
		{Index: 4, Kind: proto.TranscriptTool, Text: "Edit", Detail: "/p/proxy/installer.go", Added: 18, Removed: 6, Done: true, Time: at.Add(14 * time.Second)},
		{Index: 5, Kind: proto.TranscriptAssistant, Text: "Saya pasang context 30 detik dan retry 3×."},
		{Index: 6, Kind: proto.TranscriptUser, Text: "Jalankan tesnya", Time: at.Add(time.Minute)},
		{Index: 7, Kind: proto.TranscriptTool, Text: "Bash", Detail: "go test ./proxy/..."},
	}
	ch := New()
	turns := splitTurns(entries)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(760).Height(700).Padding(20).Background(theme.Background).TextColor(theme.Text).Children(func() {
			ch.turnTimeline(c.Key(0), turns[0], false)
			ch.turnTimeline(c.Key(1), turns[1], true)
		})
	}, 760, 700)
	tt.Frame()
	for _, want := range []string{"You · 14:02", "Worked for 14s", "· 2 tools · 1 file edited", "Saya pasang context 30 detik dan retry 3×.", "Run", "go test ./proxy/..."} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	if tt.HasText("Thought for 8s") {
		t.Fatal("the done turn's work shows unfolded")
	}
	if err := tt.Click("Show the work"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Thought for 8s") || !tt.HasText("installer.go  /p/proxy") || !tt.HasText("+18") {
		t.Fatalf("the work opened: %q", tt.Texts())
	}
	timelineShot(t, tt, "chat-timeline.png")
	if err := tt.Click("Hide the work"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Thought for 8s") {
		t.Fatal("the work didn't fold again")
	}
}

// timelineShot saves what tt shows as name under $SHOTS, to look at.
func timelineShot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}

// Wide, the conversation has an outline of the user's messages, each a
// click away.
func TestOutline(t *testing.T) {
	ch := New()
	ch.turns = splitTurns([]proto.TranscriptEntry{
		{Index: 0, Kind: proto.TranscriptUser, Text: "Bikin endpoint health"},
		{Index: 1, Kind: proto.TranscriptAssistant, Text: "Done."},
		{Index: 2, Kind: proto.TranscriptUser, Text: "Ganti logger ke slog\nyang structured"},
		{Index: 3, Kind: proto.TranscriptAssistant, Text: "Done."},
	})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1280).Height(700).Background(theme.Background).TextColor(theme.Text).Children(func() {
			ch.conversation(c, Options{})
		})
	}, 1280, 700)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("Your messages") || !tt.HasText("Ganti logger ke slog") {
		t.Fatalf("the outline: %q", tt.Texts())
	}
	timelineShot(t, tt, "chat-outline.png")
	if err := tt.Click("Go to: Bikin endpoint health"); err != nil {
		t.Fatal(err)
	}
	narrow := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(700).Height(700).Children(func() { ch.conversation(c, Options{}) })
	}, 700, 700)
	narrow.Frame()
	narrow.Frame()
	if narrow.HasText("Your messages") {
		t.Fatal("an outline in a narrow conversation")
	}
}

// Durations read as short as they can.
func TestTook(t *testing.T) {
	for ms, want := range map[int64]string{400: "0.4s", 12_400: "12s", 185_000: "3m 05s", 7_560_000: "2h 06m"} {
		if got := took(ms); got != want {
			t.Errorf("%d: %q, want %q", ms, got, want)
		}
	}
}

// A message's attached pictures are found among its words, escaped as
// attachments write them.
func TestPicturePaths(t *testing.T) {
	got := picturePaths(`look at /tmp/up/Screen\ Shot\ 1.png and /tmp/a.go, then /tmp/b.WEBP`)
	if len(got) != 2 || got[0] != "/tmp/up/Screen Shot 1.png" || got[1] != "/tmp/b.WEBP" {
		t.Fatalf("%q", got)
	}
}

// The outline shows the newest messages: at its end when opened and as
// messages come, where the user left it once scrolled up, following again
// once back at its end. Each conversation starts at its end.
func TestOutlineStaysAtTheNewest(t *testing.T) {
	entries := func(n int) []proto.TranscriptEntry {
		var out []proto.TranscriptEntry
		for i := range n {
			out = append(out, proto.TranscriptEntry{Index: 2 * i, Kind: proto.TranscriptUser, Text: "Message number " + strconv.Itoa(i)},
				proto.TranscriptEntry{Index: 2*i + 1, Kind: proto.TranscriptAssistant, Text: "Done."})
		}
		return out
	}
	ch := New()
	ch.turns = splitTurns(entries(40))
	view := func(chat *Chat) *ui.Tester {
		return ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Width(1280).Height(700).Background(theme.Background).TextColor(theme.Text).Children(func() {
				chat.conversation(c, Options{})
			})
		}, 1280, 700)
	}
	tt := view(ch)
	frames := func(tt *ui.Tester) {
		for range 4 {
			tt.Frame()
		}
	}
	shows := func(tt *ui.Tester, n int) bool {
		b, ok := tt.Find("Go to: Message number " + strconv.Itoa(n))
		return ok && b.H > 0 // off the outline's rows, it isn't laid out
	}
	frames(tt)
	if ch.outlineScroll.MaxY <= 0 {
		t.Fatal("the outline doesn't overflow: nothing to test")
	}
	if !shows(tt, 39) {
		t.Fatalf("opened: not at its newest: at %v of %v", ch.outlineScroll.Y, ch.outlineScroll.MaxY)
	}
	// A new message keeps it at the end.
	ch.turns = splitTurns(entries(41))
	frames(tt)
	if !shows(tt, 40) || ch.outlineLeft {
		t.Fatalf("a new message: at %v of %v", ch.outlineScroll.Y, ch.outlineScroll.MaxY)
	}
	// Scrolled up, it stays, whatever comes and whatever is read.
	over := float32(1280 - 16 - outlineWidth/2)
	tt.Move(over, 200)
	tt.Frame()
	tt.Scroll(over, 200, 0, -200) // up: a wheel's sign
	frames(tt)
	if !ch.outlineLeft {
		t.Fatal("the wheel didn't move it")
	}
	left := ch.outlineScroll.Y
	ch.turns = splitTurns(entries(42))
	frames(tt)
	ch.list.ScrollTo(0, ui.Start)
	frames(tt)
	if ch.outlineScroll.Y != left || shows(tt, 41) {
		t.Fatalf("moved from where it was left: %v, was %v", ch.outlineScroll.Y, left)
	}
	// Back at its end, it follows again.
	ch.outlineScroll.Y = ch.outlineScroll.MaxY
	frames(tt)
	ch.turns = splitTurns(entries(43))
	frames(tt)
	if ch.outlineLeft || !shows(tt, 42) {
		t.Fatalf("back at its end: at %v of %v", ch.outlineScroll.Y, ch.outlineScroll.MaxY)
	}
	// Another session's starts at its end.
	other := New()
	other.turns = splitTurns(entries(30))
	ot := view(other)
	frames(ot)
	if !shows(ot, 29) {
		t.Fatalf("another conversation: at %v of %v", other.outlineScroll.Y, other.outlineScroll.MaxY)
	}
}
