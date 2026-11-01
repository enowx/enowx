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
	col := ui.Column(c).FillWidth().Gap(12)
	if animate {
		col.Transition(widget.RowMotion(0))
	}
	col.Children(func() { ch.turnTimeline(c, t, running) })
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

// row is a line of a turn's work that opens to show body.
func (ch *Chat) row(c *ui.Context, index int, svg *ui.SVG, label, detail string, status func(), body func()) {
	open := ch.open[index]
	ui.Column(c).FillWidth().Children(func() {
		if ch.lineHead(c, svg, label, detail, status).Clicked() {
			ch.open[index] = !open
		}
		if open {
			ui.Column(c).FillWidth().Padding(2, 10, 8, 32).Transition(widget.PageMotion).Children(body)
		}
	})
}

// lineHead is the clickable line of a step: an icon, what it is, a detail
// in the code font, and its status at the end.
func (ch *Chat) lineHead(c *ui.Context, svg *ui.SVG, label, detail string, status func()) ui.Element {
	head := ui.ButtonBase(c).FillWidth().Gap(9).Padding(5, 10).Radius(theme.RadiusControl).Cursor(ui.CursorPointer).
		Justify(ui.Start).Label(label)
	bg := theme.Fade(theme.Hover, 0)
	if head.Hovered() {
		bg = theme.Hover
	}
	head.Background(widget.Tint(head, "bg", bg)).Children(func() {
		ui.Icon(c, svg).FontSize(13).TextColor(theme.Muted)
		ui.Text(c, label).FontSize(theme.TextUI).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		if detail != "" {
			ui.Text(c, detail).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1).Grow(1)
		} else {
			ui.Spacer(c)
		}
		if status != nil {
			ui.Row(c).Gap(5).Shrink(0).Children(status)
		}
	})
	return head
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
				if widget.Button(c.Key("git"), widget.Secondary, icon.Git, "Open in Git", "").Height(26).Clicked() {
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
		ui.Text(c, "No output yet.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		return
	}
	ui.Text(c, e.Result).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Padding(8, 10).Radius(theme.RadiusControl).
		Background(theme.Sunken).Border(1, theme.Border).FillWidth().Selectable()
}
