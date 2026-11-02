package chatview

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The chat's asks, menus and the composer's states read at the width of a
// session alone and beside the side panel, where the composer goes compact.
func TestChatCardsAndComposerStates(t *testing.T) {
	ask := proto.TranscriptEntry{Index: 0, Kind: proto.TranscriptUser, Text: "Tambahin timeout di proxy installer /tmp/one.png", Time: t0}
	waitOn := func(q ...proto.TranscriptQuestion) []proto.TranscriptEntry {
		return []proto.TranscriptEntry{ask, {Index: 1, Kind: proto.TranscriptTool, Text: "AskUserQuestion", Questions: q}}
	}
	short := proto.TranscriptQuestion{Header: "Scope", Question: "Which agents should I support first?", Options: []string{"Claude Code and Codex", "Every agent"}}
	long := proto.TranscriptQuestion{Question: "How should it go?", Options: []string{"Keep it", "Replace it"}, About: []string{"Nothing changes for anyone", "Everything moves over on the next start"}}
	multi := proto.TranscriptQuestion{Question: "Which ones?", Options: []string{"One", "Two"}, Multi: true}
	waiting := Options{Agent: "claude", AgentName: "Claude Code", Working: true, Waiting: true, Answer: func(string) {}, ShowTerminal: func() {}}
	base := Options{Agent: "claude", AgentName: "Claude Code", Answer: func(string) {}, Send: func(string, []string) {}, Interrupt: func() {}, AttachFiles: func() {}, Edit: func(int, string, string, []string, func(error)) {}}
	bash := []proto.TranscriptEntry{ask, {Index: 1, Kind: proto.TranscriptTool, Text: "Bash", Detail: "git push origin keys-vault"}}
	withWork := []proto.TranscriptEntry{ask,
		{Index: 1, Kind: proto.TranscriptTool, Text: "Read", Detail: "/p/a.go", Done: true},
		{Index: 2, Kind: proto.TranscriptTool, Text: "Grep", Detail: "http.Get", Done: true, Failed: true, Result: "no match"},
		{Index: 3, Kind: proto.TranscriptTool, Text: "Bash", Detail: "cat /p/b.go", Done: true},
		{Index: 4, Kind: proto.TranscriptAssistant, Text: "Done."}}
	cases := []struct {
		name    string
		entries []proto.TranscriptEntry
		o       Options
		set     func(ch *Chat)
		want    []string
	}{
		{name: "question", entries: waitOn(short), o: waiting, want: []string{"Which agents should I support first?", "Every agent", "Or answer in your own words", "Scope"}},
		{name: "question-rows", entries: waitOn(long, short), o: waiting, want: []string{"1 of 2", "Everything moves over on the next start"}},
		{name: "question-multi", entries: waitOn(multi), o: waiting, want: []string{"· One", "Answer in the terminal"}},
		{name: "question-answered", entries: waitOn(short), o: waiting, set: func(ch *Chat) { ch.answered[1] = 1 }, want: []string{"Answered", "If the agent asks you to confirm, its terminal shows it."}},
		{name: "permission", entries: bash, o: waiting, want: []string{"Claude Code asks to run a command", "git push origin keys-vault", "Allow", "Always allow", "Deny"}},
		{name: "permission-sent", entries: bash, o: waiting, set: func(ch *Chat) { ch.answered[1] = 1 }, want: []string{"Sent"}},
		{name: "permission-terminal", entries: bash, o: Options{Agent: "shell", AgentName: "Shell", Working: true, Waiting: true, ShowTerminal: func() {}}, want: []string{"Shell asks to run a command", "Answer in the terminal"}},
		{name: "wait", entries: []proto.TranscriptEntry{ask, {Index: 1, Kind: proto.TranscriptAssistant, Text: "Hm."}}, o: waiting, want: []string{"The agent waits for you", "Answer in the terminal"}},
		{name: "ended", entries: withWork, o: Options{Agent: "claude", AgentName: "Claude Code", Ended: true}, want: []string{"Ended"}},
		{name: "work-open", entries: withWork, o: base, set: func(ch *Chat) { ch.open[0] = true; ch.open[-2] = true },
			want: []string{"1 failed", "Read 2 files, searched once"}},
		{name: "slash-loading", entries: withWork, o: base, set: func(ch *Chat) { ch.input = "/" }, want: []string{"Reading the agent's commands…"}},
		{name: "slash-none", entries: withWork, o: base, set: func(ch *Chat) { ch.commandsRead, ch.input = true, "/zz" }, want: []string{"No command matches /zz"}},
		{name: "slash", entries: withWork, o: base, set: func(ch *Chat) {
			ch.commandsRead, ch.input = true, "/"
			ch.commands = []proto.AgentCommand{{Name: "compact", Description: "Compact the conversation"}, {Name: "review", Description: strings.Repeat("Reviews what changed. ", 8), Kind: "skill"}}
		}, want: []string{"/compact", "/review", "skill"}},
		{name: "attachments", entries: withWork, o: base, set: func(ch *Chat) {
			ch.Attach("a-file-with-a-rather-long-name-that-goes-on.txt", nil)
			ch.Attach("uploaded.txt", nil)("/tmp/uploaded.txt", nil)
		}, want: []string{"Send (Enter)"}},
		{name: "edit", entries: withWork, o: base, set: func(ch *Chat) { ch.editing, ch.input = &withWork[0], withWork[0].Text },
			want: []string{"Editing a message: sending it replaces it and what came after. Files stay as they are."}},
		{name: "edit-busy", entries: withWork, o: base, set: func(ch *Chat) { ch.editing, ch.editBusy = &withWork[0], true }, want: []string{"Going back to before this message…"}},
		{name: "edit-error", entries: withWork, o: base, set: func(ch *Chat) {
			ch.editing, ch.editErr = &withWork[0], "couldn't go back to that message: no such turn"
		},
			want: []string{"couldn't go back to that message: no such turn"}},
		{name: "menu", entries: withWork, o: base, set: func(ch *Chat) { ch.menu = "effort"; ch.effort = "high" }, want: []string{"Low effort", "High effort"}},
		{name: "search", entries: withWork, o: base, set: func(ch *Chat) { ch.finding, ch.find = true, "tambahin" }, want: []string{"1 of 1"}},
		{name: "search-none", entries: withWork, o: base, set: func(ch *Chat) { ch.finding, ch.find = true, "zzz" }, want: []string{"No match"}},
		{name: "context", entries: withWork, o: base, set: func(ch *Chat) {
			ch.contextUsed, ch.contextWindow, ch.added, ch.removed = 150_000, 200_000, 12044, 3211
		}, want: []string{"75%"}},
		{name: "context-unknown", entries: withWork, o: base, set: func(ch *Chat) { ch.contextUsed = 150_000 }, want: []string{"150k tokens"}},
		{name: "first-prompt", o: Options{Agent: "claude", AgentName: "Claude Code", FirstPrompt: "Start with this", Working: true}, want: []string{"Start with this", "Thinking"}},
		{name: "unsupported", o: Options{Agent: "aider"}, set: func(ch *Chat) { ch.loaded, ch.supported, ch.reason = true, false, "This agent has no chat view yet." }, want: []string{"This agent has no chat view yet."}},
		{name: "reading", o: Options{Agent: "claude"}, set: func(ch *Chat) { ch.loaded = false }, want: []string{"Reading the conversation…"}},
	}
	for _, st := range cases {
		for _, w := range []int{1100, 700, 440} {
			ch := loaded(st.entries)
			if st.set != nil {
				st.set(ch)
			}
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Width(float32(w)).Height(720).Background(theme.Panel).TextColor(theme.Text).Children(func() { ch.View(c, st.o) })
			}, w, 720)
			tt.Frame()
			tt.Frame()
			tt.Frame()
			for _, want := range st.want {
				if !hasTextLike(tt, want) {
					t.Errorf("%s at %d: no %q in %q", st.name, w, want, tt.Texts())
				}
			}
			if w != 440 {
				timelineShot(t, tt, "cards-"+st.name+"-"+map[int]string{1100: "wide", 700: "panel"}[w]+".png")
			}
		}
	}
}
