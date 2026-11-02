package chatview

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// turn is what came of one message of the user's: the message, the work
// the agent did for it (thoughts, tool calls, what it said on the way),
// and its answer, the text it ended on.
type turn struct {
	start  int // index of the turn's first entry, which keys it
	prompt *proto.TranscriptEntry
	work   []proto.TranscriptEntry
	answer []proto.TranscriptEntry
	out    *outgoing // the message on its way this turn stands for
}

// splitTurns groups entries into turns. Notices stand as turns of their
// own, without a prompt.
func splitTurns(entries []proto.TranscriptEntry) []turn {
	var out []turn
	for i := range entries {
		e := entries[i]
		if e.Kind == proto.TranscriptUser || len(out) == 0 || e.Kind == proto.TranscriptNotice {
			out = append(out, turn{start: e.Index})
		}
		t := &out[len(out)-1]
		switch e.Kind {
		case proto.TranscriptUser:
			t.prompt = &entries[i]
		case proto.TranscriptNotice:
			t.answer = append(t.answer, e)
		default:
			t.work = append(t.work, e)
		}
	}
	// The answer is the text after the last thought or tool call.
	for i := range out {
		t := &out[i]
		last := len(t.work)
		for last > 0 && t.work[last-1].Kind == proto.TranscriptAssistant {
			last--
		}
		t.answer = append(t.answer, t.work[last:]...)
		t.work = t.work[:last]
	}
	return out
}

// elapsed is how long the turn's work took, from its prompt to its last
// entry.
func (t turn) elapsed() time.Duration {
	if t.prompt == nil || t.prompt.Time.IsZero() {
		return 0
	}
	end := t.prompt.Time
	for _, e := range append(append([]proto.TranscriptEntry{}, t.work...), t.answer...) {
		if e.Time.After(end) {
			end = e.Time
		}
	}
	return end.Sub(t.prompt.Time)
}

// turn draws one turn; animate makes it come in, for turns that arrive
// while the chat shows, not those it opens on.
func (ch *Chat) turn(c *ui.Context, t turn, running, animate bool) {
	col := ui.Column(c).FillWidth()
	if animate {
		col.Transition(widget.RowMotion(0))
	}
	col.Children(func() {
		if t.out == nil {
			ch.turnTimeline(c, t, running)
			return
		}
		editable := ch.editable
		ch.editable = false // nothing recorded to go back to yet
		ch.turnTimeline(c, t, running)
		ch.editable = editable
		ch.outgoingNote(c, t.out, running)
	})
}

// edit puts message e in the composer, to send in its place.
func (ch *Chat) edit(e proto.TranscriptEntry) {
	ch.editing, ch.editErr = &e, ""
	ch.input, ch.focus = e.Text, true
}

// editBack is how many of the user's messages the one edited is from the
// end, itself counted: those the agent goes back over.
func (ch *Chat) editBack() int {
	n := 0
	for _, e := range ch.entries {
		if e.Kind == proto.TranscriptUser && e.Index >= ch.editing.Index {
			n++
		}
	}
	return n
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// toolLook is the icon and verb a tool call shows with.
func toolLook(name string) (*ui.SVG, string) {
	switch name {
	case "Read", "NotebookRead":
		return icon.File, "Read"
	case "Edit", "MultiEdit", "NotebookEdit":
		return icon.Pencil, "Edit"
	case "Write":
		return icon.Pencil, "Write"
	case "Bash", "BashOutput", "KillShell":
		return icon.Terminal, "Run"
	case "Grep", "Glob", "LS":
		return icon.Search, "Search"
	case "WebFetch", "WebSearch":
		return icon.Globe, "Web"
	case "Task", "Agent":
		return icon.Bot, "Agent"
	case "TodoWrite":
		return icon.List, "Plan"
	}
	if strings.HasPrefix(name, "mcp__") {
		parts := strings.Split(name, "__")
		return icon.Bot, parts[len(parts)-1]
	}
	return icon.Terminal, name
}

// toolBody is what an open tool call shows: the change an edit made, with
// the file a click from the Git panel, else what the tool answered.
func (ch *Chat) toolBody(c *ui.Context, e proto.TranscriptEntry) {
	if e.Old != "" || e.New != "" {
		if ch.openChanges != nil && e.Detail != "" {
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if widget.Button(c.Key("git"), widget.Secondary, icon.Git, "Open in Git", "").Clicked() {
					ch.openChanges(e.Detail)
				}
			})
		}
		var b strings.Builder
		for _, l := range strings.Split(e.Old, "\n") {
			if e.Old != "" {
				b.WriteString("-" + l + "\n")
			}
		}
		for _, l := range strings.Split(e.New, "\n") {
			b.WriteString("+" + l + "\n")
		}
		widget.CodeBlock(c, "diff", strings.TrimSuffix(b.String(), "\n"))
		return
	}
	if e.Result == "" {
		ui.Text(c, "No output yet.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		return
	}
	widget.CodeBlockText(c, e.Result)
}
