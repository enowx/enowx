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

// A turn as a timeline: each thing in it (the user's message, a thought, a
// tool call, the agent's words) a line along a rule, marked by an icon of
// what it is. Color is kept for what needs the eye: the user's own
// messages, the step under way, a failure.

// tone is how a timeline mark is colored.
type tone int

const (
	toneStep  tone = iota // a step done: quiet
	toneUser              // the user's message
	toneAgent             // the agent's own words
	toneRun               // the step under way
	toneFail              // a step that failed
	toneAsk               // what the agent waits on the user for
)

// markSize is a mark's diameter; the rule runs through their centers.
const markSize = 26

// node is one line of the timeline: its mark on the rule, then body; last
// stops the rule at the mark.
func node(c *ui.Context, svg *ui.SVG, t tone, last bool, body func()) {
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(markSize).Shrink(0).AlignItems(ui.Center).Children(func() {
			mark(c, svg, t)
			if !last {
				ui.Box(c).Width(1).Grow(1).MinHeight(6).Background(theme.BorderStrong)
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Padding(3, 0, 12, 0).Gap(6).Children(body)
	})
}

// mark is a node's icon in its circle.
func mark(c *ui.Context, svg *ui.SVG, t tone) {
	bg, border, fg := theme.Panel, theme.BorderStrong, theme.TextSoft
	switch t {
	case toneUser:
		bg, border, fg = theme.Accent, theme.Accent, theme.OnAccent
	case toneAgent:
		fg = theme.Accent
	case toneRun:
		border, fg = theme.Working, theme.Working
	case toneFail:
		border, fg = theme.Error, theme.Error
	case toneAsk:
		bg, border, fg = theme.Fade(theme.Warning, 0.15), theme.Warning, theme.Warning
	}
	ui.Box(c).Size(markSize, markSize).Shrink(0).Radius(markSize/2).Center().Background(bg).Border(1, border).Children(func() {
		ui.Icon(c, svg).FontSize(13).TextColor(fg)
	})
}

// item is something the timeline shows of a turn: an entry, a run of
// tool calls folded into one line, or (work) the line that opens the
// work of a turn done.
type item struct {
	entry *proto.TranscriptEntry
	tools []proto.TranscriptEntry
	work  bool
}

// turnTimeline draws turn t: the user's message, its work (step by step
// while under way; once done, a line that opens to it), the agent's
// answer. Notices stand apart, small and centered.
func (ch *Chat) turnTimeline(c *ui.Context, t turn, running bool) {
	var items []item
	if t.prompt != nil {
		items = append(items, item{entry: t.prompt})
	}
	open := ch.workOpen(t, running)
	if len(t.work) > 0 && !running {
		items = append(items, item{work: true})
	}
	if open {
		for _, st := range workSteps(t.work) {
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
	waitsElse := running && ch.ask.waiting && !thinking
	if waitsElse && len(items) > 0 {
		if l := items[len(items)-1]; l.entry != nil && ch.asks(*l.entry, true) {
			waitsElse = false
		}
	}
	ui.Column(c).FillWidth().Children(func() {
		for i, it := range items {
			last := i == len(items)-1 && !thinking
			latest := running && i == len(items)-1
			switch {
			case it.work:
				ch.workNode(c.Key("work"), t, open, last)
			case it.tools != nil:
				ch.groupNode(c.Key(it.tools[0].Index), it.tools, latest, last)
			case it.entry.Kind == proto.TranscriptUser:
				ch.userNode(c.Key(it.entry.Index), *it.entry, running, last)
			case ch.asks(*it.entry, latest):
				ch.askNode(c.Key(it.entry.Index), *it.entry, last && !waitsElse)
			default:
				ch.entryNode(c.Key(it.entry.Index), *it.entry, latest, last && !waitsElse)
			}
		}
		if waitsElse {
			ch.waitNode(c.Key("wait"))
		}
		if thinking {
			node(c.Key("thinking"), icon.Brain, toneRun, true, func() {
				ui.Row(c).Height(20).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Thinking").FontSize(theme.TextUI).TextColor(theme.TextSoft).Italic()
					widget.Spinner(c, 11)
				})
			})
		}
		for i, e := range notices {
			ui.Row(c.Key(fmt.Sprint("notice", i))).FillWidth().Justify(ui.Center).Padding(4, 0).Children(func() {
				ui.Text(c, e.Text).FontSize(theme.TextLabel).TextColor(theme.Muted)
			})
		}
	})
}

// workOpen says turn t's work shows step by step: under way, or opened.
func (ch *Chat) workOpen(t turn, running bool) bool {
	if open, set := ch.open[t.start]; set {
		return open
	}
	return running
}

// userNode is the user's message, with the pencil that sends it again
// changed while the pointer is over it.
func (ch *Chat) userNode(c *ui.Context, e proto.TranscriptEntry, running, last bool) {
	editing := ch.editing != nil && ch.editing.Index == e.Index
	text := e.Text
	if ch.showUser != nil {
		text = ch.showUser(text)
	}
	node(c, icon.Account, toneUser, last, func() {
		row := ui.Row(c).FillWidth().Height(20).Gap(6).AlignItems(ui.Center)
		row.Children(func() {
			who := "You"
			if !e.Time.IsZero() {
				who += " · " + e.Time.Local().Format("15:04")
			}
			ui.Text(c, who).FontSize(theme.TextLabel).TextColor(theme.Accent).SingleLine()
			ui.Spacer(c)
			if ch.editable && (row.Hovered() || editing) {
				tip := "Edit and send again"
				if running {
					tip = "Edit once the agent is done"
				}
				if widget.IconButton(c, icon.Pencil, tip, running).Clicked() && !running {
					ch.edit(e)
				}
			}
		})
		t := ui.Text(c, text).TextColor(theme.TextStrong).Selectable()
		if editing {
			t.TextColor(theme.Accent)
		}
		ch.pictureChips(c, e.Text)
	})
}

// workNode is a done turn's work in one line, opening it step by step
// (open) or folding it again: how long it took, its tool calls, the files
// it changed.
func (ch *Chat) workNode(c *ui.Context, t turn, open, last bool) {
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
	tn, svg := toneStep, icon.Check
	if failed > 0 {
		tn, svg = toneFail, icon.Alert
	}
	node(c, svg, tn, last, func() {
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
		what := "Show the work"
		if open {
			what = "Hide the work"
		}
		head := ui.ButtonBase(c).FillWidth().Height(20).Gap(7).AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).Label(what)
		head.Children(func() {
			col := theme.TextSoft
			if head.Hovered() {
				col = theme.Text
			}
			ui.Text(c, label).FontSize(theme.TextUI).TextColor(col).SingleLine()
			if len(parts) > 0 {
				ui.Text(c, "· "+strings.Join(parts, " · ")).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
			}
			chev := icon.ChevronRight
			if open {
				chev = icon.ChevronDown
			}
			ui.Icon(c, chev).FontSize(11).TextColor(theme.Muted)
		})
		if head.Clicked() {
			ch.open[t.start] = !open
		}
	})
}

// entryNode is a thought, the agent's words, or one tool call.
func (ch *Chat) entryNode(c *ui.Context, e proto.TranscriptEntry, latest, last bool) {
	switch e.Kind {
	case proto.TranscriptThinking:
		label := "Thought"
		if e.DurationMs > 0 {
			label += " for " + (time.Duration(e.DurationMs) * time.Millisecond).Round(time.Second).String()
		}
		tn := toneStep
		if latest {
			tn = toneRun
		}
		node(c, icon.Brain, tn, last, func() {
			ch.stepLine(c, e.Index, label, "", nil, func() {
				ui.Text(c, e.Text).FontSize(theme.TextLabel).TextColor(theme.Muted).Italic().Selectable()
			})
		})
	case proto.TranscriptAssistant:
		node(c, icon.Sparkle, toneAgent, last, func() {
			widget.MarkdownLinks(c, e.Text, ch.links)
		})
	case proto.TranscriptTool:
		svg, verb := toolLook(e.Text)
		if e.Text == "Bash" {
			svg, verb = shellLook(e)
		}
		tn := toneStep
		switch {
		case e.Failed:
			tn = toneFail
		case latest && !e.Done:
			tn = toneRun
		}
		node(c, svg, tn, last, func() {
			ch.stepLine(c, e.Index, verb, toolDetailShown(e), func() { toolStatus(c, e, latest) }, func() { ch.toolBody(c, e) })
		})
	}
}

// groupNode is a run of tool calls on one line, opening to each.
func (ch *Chat) groupNode(c *ui.Context, tools []proto.TranscriptEntry, latest, last bool) {
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
	tn := toneStep
	switch {
	case failed > 0:
		tn = toneFail
	case latest && !tools[len(tools)-1].Done:
		tn = toneRun
	}
	svg, _ := toolLook(tools[len(tools)-1].Text)
	node(c, svg, tn, last, func() {
		status := func() {
			if added+removed > 0 {
				ui.Text(c, fmt.Sprintf("+%d", added)).FontSize(theme.TextLabel).TextColor(theme.Done).SingleLine()
				ui.Text(c, fmt.Sprintf("−%d", removed)).FontSize(theme.TextLabel).TextColor(theme.Error).SingleLine()
			}
			if failed > 0 {
				ui.Text(c, fmt.Sprintf("%d failed", failed)).FontSize(theme.TextLabel).TextColor(theme.Error).SingleLine()
			}
		}
		if stepHead(c, summary(tools), "", status, open).Clicked() {
			ch.open[key] = !open
		}
		if open {
			ui.Column(c).FillWidth().Gap(0).Children(func() {
				for i, e := range tools {
					ch.toolRow(c.Key(e.Index), e, latest && i == len(tools)-1)
				}
			})
		}
	})
}

// stepLine is a step's line, opening to body.
func (ch *Chat) stepLine(c *ui.Context, index int, label, detail string, status func(), body func()) {
	open := ch.open[index]
	if stepHead(c, label, detail, status, open).Clicked() {
		ch.open[index] = !open
	}
	if open {
		ui.Column(c).FillWidth().Transition(widget.PageMotion).Children(body)
	}
}

// stepHead is a step's clickable line: what it is, a detail in the code
// font, its status, and the chevron saying it opens.
func stepHead(c *ui.Context, label, detail string, status func(), open bool) ui.Element {
	head := ui.ButtonBase(c).FillWidth().Height(20).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Justify(ui.Start).Label(label)
	head.Children(func() {
		col := theme.TextSoft
		if head.Hovered() {
			col = theme.Text
		}
		ui.Text(c, label).FontSize(theme.TextUI).TextColor(col).SingleLine().Shrink(0)
		if detail != "" {
			ui.Text(c, detail).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
		}
		ui.Spacer(c)
		if status != nil {
			ui.Row(c).Gap(5).Shrink(0).Children(status)
		}
		chev := icon.ChevronRight
		if open {
			chev = icon.ChevronDown
		}
		ui.Icon(c, chev).FontSize(11).TextColor(theme.Muted)
	})
	return head
}

// toolStatus is a tool call's status at the end of its line.
func toolStatus(c *ui.Context, e proto.TranscriptEntry, latest bool) {
	switch {
	case e.Added+e.Removed > 0:
		ui.Text(c, fmt.Sprintf("+%d", e.Added)).FontSize(theme.TextLabel).TextColor(theme.Done).SingleLine()
		ui.Text(c, fmt.Sprintf("−%d", e.Removed)).FontSize(theme.TextLabel).TextColor(theme.Error).SingleLine()
	case e.Failed:
		ui.Text(c, "failed").FontSize(theme.TextLabel).TextColor(theme.Error).SingleLine()
	case latest && !e.Done:
		widget.Spinner(c, 11)
	}
	if e.Done && e.DurationMs > 0 {
		ui.Text(c, took(e.DurationMs)).FontSize(theme.TextLabel).TextColor(theme.Idle).SingleLine()
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
