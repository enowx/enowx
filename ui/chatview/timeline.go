package chatview

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A turn reads as a conversation: the user's message a bubble at the right
// in the accent, the agent's words plain text in the column, what it did in
// quiet lines between, and what it does now in a line of its own at the end.

const (
	// bubbleWidth and answerWidth are as wide as a message and an answer run:
	// less than the column, so who says what shows in their edges.
	bubbleWidth = 560
	answerWidth = 640
	// readLeading is the reading text's line height, 24 on its 15.
	readLeading = 1.6
	// bubbleControls is the room kept beside a bubble for its time and pencil,
	// so that showing them under the pointer doesn't reflow the message.
	bubbleControls = 88
)

// item is something the timeline shows of a turn: an entry, a run of
// tool calls folded into one line, or (work) the line that opens the
// work of a turn done.
type item struct {
	entry *proto.TranscriptEntry
	tools []proto.TranscriptEntry
	work  bool
}

// isLine says the item is one of the quiet lines, which follow each other
// without a gap, rather than a block of its own.
func (ch *Chat) isLine(it item, latest bool) bool {
	if it.entry == nil {
		return true
	}
	switch it.entry.Kind {
	case proto.TranscriptUser, proto.TranscriptAssistant:
		return false
	}
	return !ch.asks(*it.entry, latest)
}

// turnTimeline draws turn t: the user's message, its work (step by step
// while under way; once done, a line that opens to it), the agent's
// answer, and while it runs the line of what it does now. Notices stand
// apart, small and centered.
func (ch *Chat) turnTimeline(c *ui.Context, t turn, running bool) {
	waiting := ch.ask.waiting
	work := t.work
	// The call under way is the line of what the agent does now, not a line
	// of what it did.
	var current *proto.TranscriptEntry
	if n := len(work); running && !waiting && n > 0 && len(t.answer) == 0 && work[n-1].Kind == proto.TranscriptTool && !work[n-1].Done {
		current, work = &work[n-1], work[:n-1]
	}
	var items []item
	if t.prompt != nil {
		items = append(items, item{entry: t.prompt})
	}
	open := ch.workOpen(t, running)
	if len(t.work) > 0 && !running {
		items = append(items, item{work: true})
	}
	if open {
		for _, st := range workSteps(work) {
			if st.entry != nil {
				items = append(items, item{entry: st.entry})
			} else if len(st.tools) >= groupFrom {
				items = append(items, item{tools: st.tools})
			} else {
				for i := range st.tools {
					items = append(items, item{entry: &st.tools[i]})
				}
			}
		}
	}
	var notices []proto.TranscriptEntry
	for i := range t.answer {
		if t.answer[i].Kind == proto.TranscriptNotice {
			notices = append(notices, t.answer[i])
		} else {
			items = append(items, item{entry: &t.answer[i]})
		}
	}
	thinking := running && len(t.work) == 0 && len(t.answer) == 0 && t.prompt != nil
	// Waiting on something no entry shows (a question of the agent's
	// own screen): a card of its own, its terminal answering.
	waitsElse := running && waiting && !thinking
	if waitsElse && len(items) > 0 {
		if l := items[len(items)-1]; l.entry != nil && ch.asks(*l.entry, true) {
			waitsElse = false
		}
	}
	isLatest := func(i int) bool { return running && i == len(items)-1 }
	ui.Column(c).FillWidth().Gap(theme.S4).Children(func() {
		for i := 0; i < len(items); {
			it := items[i]
			if !ch.isLine(it, isLatest(i)) {
				ch.blockItem(c.Key(i), *it.entry, running)
				i++
				continue
			}
			end := i
			for end < len(items) && ch.isLine(items[end], isLatest(end)) {
				end++
			}
			from := i
			ui.Column(c.Key(from)).FillWidth().Children(func() {
				for j := from; j < end; j++ {
					ch.lineItem(c.Key(j), t, items[j], open, isLatest(j))
				}
			})
			i = end
		}
		if waitsElse {
			ch.waitCard(c.Key("wait"))
		}
		if running && (!waiting || thinking) {
			ch.nowLine(c.Key("now"), t, current)
		}
		for i, e := range notices {
			ui.Row(c.Key(fmt.Sprint("notice", i))).FillWidth().Justify(ui.Center).Children(func() {
				ui.Text(c, e.Text).FontSize(theme.TextLabel).TextColor(theme.Muted)
			})
		}
	})
}

// blockItem is what stands apart in a turn: a message, words, or an ask.
func (ch *Chat) blockItem(c *ui.Context, e proto.TranscriptEntry, running bool) {
	switch {
	case e.Kind == proto.TranscriptUser:
		ch.userMessage(c, e, running)
	case e.Kind == proto.TranscriptAssistant:
		ui.Column(c).FillWidth().MaxWidth(answerWidth).FontSize(theme.TextRead).LineHeight(readLeading).Children(func() {
			widget.MarkdownLinks(c, e.Text, ch.links)
		})
	default:
		ch.askCardOf(c, e)
	}
}

// lineItem is one of the turn's lines.
func (ch *Chat) lineItem(c *ui.Context, t turn, it item, open, latest bool) {
	switch {
	case it.work:
		ch.workLine(c, t, open)
	case it.tools != nil:
		ch.groupLine(c, it.tools, latest)
	default:
		ch.entryLine(c, *it.entry, latest)
	}
}

// workOpen says turn t's work shows step by step: under way, or opened.
func (ch *Chat) workOpen(t turn, running bool) bool {
	if open, set := ch.open[t.start]; set {
		return open
	}
	return running
}

// userMessage is the user's message in its bubble: under the pointer, its
// time and the pencil that sends it again changed.
func (ch *Chat) userMessage(c *ui.Context, e proto.TranscriptEntry, running bool) {
	editing := ch.editing != nil && ch.editing.Index == e.Index
	text := e.Text
	if ch.showUser != nil {
		text = ch.showUser(text)
	}
	ui.Column(c).FillWidth().Gap(theme.S2).AlignItems(ui.End).Children(func() {
		row := ui.Row(c).FillWidth().Justify(ui.End).Gap(theme.S2).AlignItems(ui.Center).Label("Your message")
		row.Children(func() {
			ui.Row(c).Width(bubbleControls).Shrink(0).Gap(theme.S1).Justify(ui.End).AlignItems(ui.Center).Children(func() {
				if !row.Hovered() && !editing {
					return
				}
				if !e.Time.IsZero() {
					ui.Text(c, e.Time.Local().Format("15:04")).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
				}
				if ch.editable {
					tip := "Edit and send again"
					if running {
						tip = "Edit once the agent is done"
					}
					b := widget.IconButton(c, icon.Pencil, tip, false)
					if running {
						b.Opacity(0.45)
					}
					if b.Clicked() && !running {
						ch.edit(e)
					}
				}
			})
			bg := theme.Accent
			if editing {
				bg = theme.Fade(theme.Accent, 0.5)
			}
			ui.Column(c).MinWidth(0).Shrink(1).MaxWidth(bubbleWidth).Padding(theme.S2, theme.S4).
				Radius(theme.RadiusCard, theme.RadiusCard, theme.RadiusTail, theme.RadiusCard).Background(bg).Children(func() {
				ui.Text(c, text).FontSize(theme.TextRead).LineHeight(readLeading).TextColor(theme.OnAccent).Selectable()
			})
		})
		ch.pictureChips(c, e.Text)
	})
}

// workLine is a done turn's work in one line, opening it step by step
// (open) or folding it again: how long it took, its tool calls, the files
// it changed.
func (ch *Chat) workLine(c *ui.Context, t turn, open bool) {
	tools, edited, failed := 0, map[string]bool{}, 0
	for _, e := range t.work {
		if e.Kind == proto.TranscriptTool {
			tools++
			if e.Added+e.Removed > 0 {
				edited[e.Detail] = true
			}
			if e.Failed {
				failed++
			}
		}
	}
	label := "Worked"
	if d := t.elapsed().Round(time.Second); d > 0 {
		label += " for " + d.String()
	}
	var parts []string
	if tools > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", tools, plural(tools, "tool", "tools")))
	}
	if n := len(edited); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s edited", n, plural(n, "file", "files")))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	l := lineLook{svg: icon.Check, label: label, name: "Show the work", open: open, opens: true, chevron: true}
	if open {
		l.name = "Hide the work"
	}
	if failed > 0 {
		l.svg, l.iconColor = icon.Alert, theme.Del
	}
	if len(parts) > 0 {
		l.note = "· " + strings.Join(parts, " · ")
	}
	if drawLine(c, l).Clicked() {
		ch.open[t.start] = !open
	}
}

// entryLine is a thought, or one tool call.
func (ch *Chat) entryLine(c *ui.Context, e proto.TranscriptEntry, latest bool) {
	switch e.Kind {
	case proto.TranscriptThinking:
		label := "Thought"
		if e.DurationMs > 0 {
			label += " for " + (time.Duration(e.DurationMs) * time.Millisecond).Round(time.Second).String()
		}
		l := lineLook{svg: icon.Brain, label: label}
		if latest {
			l.color, l.iconColor = theme.Now, theme.Now
		}
		ch.stepLine(c, e.Index, l, func() {
			ui.Text(c, widget.LongWords(e.Text)).FontSize(theme.TextUI).TextColor(theme.TextSoft).Italic().Selectable()
		})
	case proto.TranscriptTool:
		ch.toolRow(c, e)
	}
}

// groupLine is a run of tool calls on one line, opening to each.
func (ch *Chat) groupLine(c *ui.Context, tools []proto.TranscriptEntry, latest bool) {
	key := -(tools[0].Index + 1)
	open, set := ch.open[key]
	if !set {
		open = latest
	}
	failed, added, removed := 0, 0, 0
	for _, e := range tools {
		if e.Failed {
			failed++
		}
		added, removed = added+e.Added, removed+e.Removed
	}
	svg, _ := toolLook(tools[len(tools)-1].Text)
	l := lineLook{svg: svg, label: summary(tools), open: open, opens: true}
	if failed > 0 {
		l.iconColor = theme.Del
	}
	l.right = func(bool) {
		figures(c, added, removed)
		if failed > 0 {
			ui.Text(c, fmt.Sprintf("%d failed", failed)).FontSize(theme.TextUI).TextColor(theme.Del).SingleLine()
		}
	}
	if drawLine(c, l).Clicked() {
		ch.open[key] = !open
	}
	if open {
		ui.Column(c).FillWidth().Padding(0, 0, 0, lineIndent).Transition(widget.PageMotion).Children(func() {
			for _, e := range tools {
				ch.toolRow(c.Key(e.Index), e)
			}
		})
	}
}

// stepLine is a step's line, opening to body.
func (ch *Chat) stepLine(c *ui.Context, index int, l lineLook, body func()) {
	l.open, l.opens = ch.open[index], true
	if l.name == "" {
		l.name = l.label
	}
	if drawLine(c, l).Clicked() {
		ch.open[index] = !l.open
	}
	if l.open {
		lineBody(c, body)
	}
}

// took is how long a step took, as short as reads: 0.4s, 12s, 3m 05s,
// 2h 10m.
func took(ms int64) string {
	switch d := time.Duration(ms) * time.Millisecond; {
	case d < 10*time.Second:
		return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
	case d < time.Minute:
		return strconv.Itoa(int(d.Round(time.Second).Seconds())) + "s"
	case d < time.Hour:
		d = d.Round(time.Second)
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		d = d.Round(time.Minute)
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
