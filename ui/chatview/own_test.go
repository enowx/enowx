package chatview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/wrapcheck"
)

const ownAgent = "claude-platform"

// ownBase is a turn with an edit, so the row holds a diff chip as well.
func ownBase() []proto.TranscriptEntry {
	return []proto.TranscriptEntry{
		{Index: 0, Kind: proto.TranscriptUser, Text: "Add a timeout", Time: t0},
		{Index: 1, Kind: proto.TranscriptTool, Text: "Edit", Detail: "/p/a.go", Added: 3, Removed: 1, Done: true},
		{Index: 2, Kind: proto.TranscriptAssistant, Text: "Done."},
	}
}

// ownTester draws a chat in a frame, styled, wide as w.
func ownTester(ch *Chat, o *Options, w int) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(float32(w)).Height(720).Background(theme.Background).TextColor(theme.Text).Children(func() { ch.View(c, *o) })
	}, w, 720)
}

func frames(tt *ui.Tester, n int) {
	for range n {
		tt.Frame()
	}
}

// The chip says the spend and the budget and fits the row at every width and in every
// style, however long the figures; nothing sticks out of its parent.
func TestCostChipFits(t *testing.T) {
	oldStyle, palette := theme.CurrentStyle(), theme.Current()
	defer func() {
		theme.UseStyle(oldStyle)
		theme.Apply(palette)
	}()
	long := "claude-opus-5-5-with-an-enormous-model-name-that-never-fits-20261105"
	for _, s := range theme.Styles {
		theme.UseStyle(s)
		theme.Apply(palette)
		for _, w := range []int{1180, 620, 440, 300} {
			for _, st := range []struct {
				name         string
				cost, budget float64
			}{{"plain", 1.24, 10}, {"near", 8.5, 10}, {"over", 12, 10}, {"huge", 123456789012.37, 987654321098}, {"tiny", 0.001, 0}} {
				ch := loaded(ownBase())
				ch.added, ch.removed, ch.model = 3, 1, long
				ch.contextUsed, ch.contextWindow = 141_000, 1_000_000
				ch.cost, ch.budget = st.cost, st.budget
				o := Options{Agent: "claude", AgentName: "Claude Platform", Send: func(string, []string) {}, AttachFiles: func() {}, Interrupt: func() {}, Working: true}
				tt := ownTester(ch, &o, w)
				frames(tt, 4)
				if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
					t.Errorf("%s, %d wide, %s: out of their parents:\n%s", s.ID, w, st.name, strings.Join(out, "\n"))
				}
				timelineShot(t, tt, fmt.Sprintf("own-chip-%s-%d-%s.png", s.ID, w, st.name))
				if r, ok := tt.Find("This session's cost: " + ch.costLabel()); ok && (r.X < 0 || r.X+r.W > float32(w)) {
					t.Errorf("%s, %d wide, %s: the chip at %+v", s.ID, w, st.name, r)
				}
			}
		}
	}
}

// Wide, the chip says budget and spend; near the budget it turns amber, past it red; it
// opens to what they mean, how to change them and where the credit shows.
func TestCostChipSays(t *testing.T) {
	ch := loaded(ownBase())
	ch.cost, ch.budget = 1.24, 10
	consoles := 0
	o := Options{Agent: ownAgent, AgentName: "Claude Platform", Send: func(string, []string) {}, OpenConsole: func() { consoles++ }}
	tt := ownTester(ch, &o, 1180)
	frames(tt, 3)
	if !tt.HasText("$1.24 of $10") {
		t.Fatalf("no chip: %q", tt.Texts())
	}
	if err := tt.Click("This session's cost: $1.24 of $10"); err != nil {
		t.Fatal(err)
	}
	frames(tt, 3)
	for _, want := range []string{"Budget: $10", "/budget", "Remaining credit shows in the Claude Console: an ordinary API key can't read it."} {
		if !hasTextLike(tt, want) {
			t.Fatalf("no %q in %q", want, tt.Texts())
		}
	}
	if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
		t.Errorf("popover: %s", strings.Join(out, "\n"))
	}
	timelineShot(t, tt, "own-cost-popover.png")
	if err := tt.Click("Open the Claude Console"); err != nil {
		t.Fatal(err)
	}
	if frames(tt, 2); consoles != 1 {
		t.Fatalf("console opened %d times", consoles)
	}
	if costLevel(8.5, 10) != theme.Warning || costLevel(10, 10) != theme.Error || costLevel(1, 10) != theme.TextSoft || costLevel(100, 0) != theme.TextSoft {
		t.Fatal("the chip's colors don't follow the budget")
	}
	if money(0.001) != "<$0.01" || budgetMoney(12.5) != "$12.50" {
		t.Fatalf("figures: %s %s", money(0.001), budgetMoney(12.5))
	}
	// An agent that says no cost shows none.
	other := loaded(ownBase())
	oo := Options{Agent: "claude", Spent: "12k tokens", Send: func(string, []string) {}}
	ot := ownTester(other, &oo, 1180)
	frames(ot, 3)
	if hasTextLike(ot, "$") {
		t.Fatalf("a chip for no cost: %q", ot.Texts())
	}
}

// Cost counts as news, or a quiet poller would never tell it.
func TestCostIsNews(t *testing.T) {
	if !news(proto.Transcript{}, proto.Transcript{CostUSD: 1}) || !news(proto.Transcript{}, proto.Transcript{BudgetUSD: 1}) {
		t.Fatal("cost isn't news")
	}
	ch := New()
	ch.apply(proto.Transcript{Supported: true, CostUSD: 2.5, BudgetUSD: 10})
	if model, _, budget := ch.Facts(); ch.cost != 2.5 || budget != 10 || model != "" {
		t.Fatalf("applied: %v %v", ch.cost, budget)
	}
}

// The poller hurries only while this agent works or a message of the user's is on its
// way, is woken when it starts, and the answers of the turn stream only then.
func TestPollerFollowsTheWork(t *testing.T) {
	ch := New()
	ch.followSpeed(Options{Agent: ownAgent})
	if ch.streaming.Load() {
		t.Fatal("quick while idle")
	}
	ch.followSpeed(Options{Agent: "claude", Working: true})
	if ch.streaming.Load() {
		t.Fatal("quick for an agent that doesn't stream")
	}
	ch.followSpeed(Options{Agent: ownAgent, Working: true})
	if !ch.streaming.Load() || len(ch.hurry) != 1 {
		t.Fatalf("not quick while working: %v, %d pokes", ch.streaming.Load(), len(ch.hurry))
	}
	if ch.liveAnswer(proto.TranscriptEntry{Index: 3}, true) == nil || ch.liveAnswer(proto.TranscriptEntry{Index: 1}, false) != nil {
		t.Fatal("which answers stream")
	}
	ch.followSpeed(Options{Agent: ownAgent, Working: true, Ended: true})
	if ch.streaming.Load() || len(ch.live) != 0 {
		t.Fatal("still quick once ended")
	}
}

// An answer changing revision after revision keeps showing as it grows, in the same
// frame, and no frame past the change costs a parse.
func TestStreamingAnswerShows(t *testing.T) {
	entries := ownBase()[:1]
	entries = append(entries, proto.TranscriptEntry{Index: 1, Kind: proto.TranscriptAssistant, Text: "First **part**\n\nSec"})
	ch := loaded(entries)
	o := Options{Agent: ownAgent, AgentName: "Claude Platform", Working: true, Send: func(string, []string) {}}
	tt := ownTester(ch, &o, 800)
	frames(tt, 3)
	if !hasTextLike(tt, "Sec") {
		t.Fatalf("no text: %q", tt.Texts())
	}
	ch.apply(proto.Transcript{Supported: true, Rev: 9, Entries: []proto.TranscriptEntry{{Index: 1, Rev: 9, Kind: proto.TranscriptAssistant, Text: "First **part**\n\nSecond part, grown"}}})
	frames(tt, 2)
	if !hasTextLike(tt, "Second part, grown") {
		t.Fatalf("the change doesn't show: %q", tt.Texts())
	}
	if s := ch.live[1]; s == nil {
		t.Fatal("not drawn through a stream")
	}
	// Done, it draws whole as any answer.
	o.Working = false
	frames(tt, 2)
	if !hasTextLike(tt, "Second part, grown") || len(ch.live) != 0 {
		t.Fatalf("done: %q, %d streams", tt.Texts(), len(ch.live))
	}
}

// The chat's questions fit a program reading a key at a time: a pick is a digit, an own
// answer the digit after the last option, the text and Enter; several picks, or more
// options than digits, go to the terminal.
func TestQuestionKeysFitSingleKeys(t *testing.T) {
	var sent []string
	ask := func(q proto.TranscriptQuestion) (*Chat, *ui.Tester) {
		e := []proto.TranscriptEntry{{Index: 0, Kind: proto.TranscriptUser, Text: "Go"}, {Index: 1, Kind: proto.TranscriptTool, Text: "AskUserQuestion", Questions: []proto.TranscriptQuestion{q}}}
		ch := loaded(e)
		o := Options{Agent: "claude", AgentName: "Claude Platform", Working: true, Waiting: true, Answer: func(k string) { sent = append(sent, k) }, ShowTerminal: func() {}}
		tt := ownTester(ch, &o, 800)
		frames(tt, 3)
		return ch, tt
	}
	ch, tt := ask(proto.TranscriptQuestion{Question: "Which?", Options: []string{"A", "B"}})
	if err := tt.Click("B"); err != nil {
		t.Fatalf("%v %q", err, tt.Texts())
	}
	frames(tt, 2)
	if len(sent) != 1 || sent[0] != "2" {
		t.Fatalf("a pick sent %q", sent)
	}
	_ = ch
	sent = nil
	ch, tt = ask(proto.TranscriptQuestion{Question: "Which?", Options: []string{"A", "B"}})
	ch.ownAnswer = "  neither  "
	frames(tt, 1)
	if err := tt.Click("Send"); err != nil {
		t.Fatalf("%v %q", err, tt.Texts())
	}
	frames(tt, 2)
	if len(sent) != 1 || sent[0] != "3neither\r" {
		t.Fatalf("an own answer sent %q", sent)
	}
	_, tt = ask(proto.TranscriptQuestion{Question: "Which?", Options: []string{"A", "B"}, Multi: true})
	if !tt.HasText("Answer in the terminal") {
		t.Fatalf("several picks: %q", tt.Texts())
	}
	many := make([]string, 10)
	for i := range many {
		many[i] = "option"
	}
	_, tt = ask(proto.TranscriptQuestion{Question: "Which?", Options: many})
	if !tt.HasText("Answer in the terminal") {
		t.Fatalf("ten options: %q", tt.Texts())
	}
	_, tt = ask(proto.TranscriptQuestion{Question: "Which?", Options: many[:9]})
	if hasTextLike(tt, "Answer in the terminal") || hasTextLike(tt, "own words") {
		t.Fatalf("nine options keep their keys, no own answer: %q", tt.Texts())
	}
}

// A notice the app knows what to do about offers it, whatever its length.
func TestNoticeOffers(t *testing.T) {
	entries := append(ownBase(), proto.TranscriptEntry{Index: 3, Kind: proto.TranscriptNotice, Text: "API error 401: invalid x-api-key " + strings.Repeat("and so on ", 40)})
	opened := 0
	for _, w := range []int{1180, 440} {
		ch := loaded(entries)
		ch.ScrollToEnd()
		o := Options{Agent: ownAgent, AgentName: "Claude Platform", Send: func(string, []string) {},
			NoticeAction: func(n string) (string, func()) { return "API keys", func() { opened++ } }}
		tt := ownTester(ch, &o, w)
		frames(tt, 4)
		if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
			t.Errorf("%d wide:\n%s", w, strings.Join(out, "\n"))
		}
		if err := tt.Click("API keys"); err != nil {
			t.Fatalf("%v %q", err, tt.Texts())
		}
		frames(tt, 2)
	}
	if opened != 2 {
		t.Fatalf("the notice's button ran %d times", opened)
	}
}

// Long tool details stay in their parents, and a subagent under way says so.
func TestLongDetailAndSubagent(t *testing.T) {
	entries := append(ownBase(),
		proto.TranscriptEntry{Index: 3, Kind: proto.TranscriptUser, Text: "Again"},
		proto.TranscriptEntry{Index: 4, Kind: proto.TranscriptTool, Text: "Bash", Detail: strings.Repeat("a-very-long-command ", 60), Done: true, Failed: true, Result: strings.Repeat("why ", 200)},
		proto.TranscriptEntry{Index: 5, Kind: proto.TranscriptTool, Text: "Task", Detail: "Look for callers"},
	)
	for _, w := range []int{1180, 440} {
		ch := loaded(entries)
		ch.open[1], ch.open[4] = true, true
		ch.ScrollToEnd()
		o := Options{Agent: ownAgent, AgentName: "Claude Platform", Working: true, Send: func(string, []string) {}}
		tt := ownTester(ch, &o, w)
		frames(tt, 4)
		if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
			t.Errorf("%d wide:\n%s", w, strings.Join(out, "\n"))
		}
		if !hasTextLike(tt, "Delegating to a subagent") {
			t.Errorf("the subagent's line: %q", tt.Texts())
		}
	}
}
