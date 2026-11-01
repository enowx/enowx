package chatview

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The quiet lines of what an agent did: an icon, what it was, a path or a
// command in the code font and its figures at the right; a line that opens
// says so only under the pointer, so a long run stays still.

// lineHeight is a line's height, and lineIndent how far what opens from a
// line starts: under its words, past its icon.
const (
	lineHeight = 24
	lineIndent = 22
)

// lineLook is what a line shows.
type lineLook struct {
	svg       *ui.SVG  // nil: the dot of what happens now
	color     ui.Color // of its words; zero is the quiet one
	iconColor ui.Color // zero follows its words
	label     string   // its words
	name      string   // what it is called for a click, when not its words
	detail    string   // a path or a command, in the code font
	note      string   // quiet words after them
	right     func(hovered bool)
	opens     bool // it opens to something
	open      bool
	chevron   bool // says so at rest, too
}

// drawLine is a line, clickable when it opens.
func drawLine(c *ui.Context, l lineLook) ui.Element {
	head := ui.ButtonBase(c).FillWidth().Height(lineHeight).Shrink(0).Gap(theme.S2).AlignItems(ui.Center).Justify(ui.Start).Label(l.name)
	if l.name == "" {
		head.Label(l.label)
	}
	if l.opens {
		head.Cursor(ui.CursorPointer)
	}
	hovered := l.opens && head.Hovered()
	col := l.color
	if col.A == 0 {
		col = theme.TextSoft
		if hovered {
			col = theme.Text
		}
	}
	head.Children(func() {
		ui.Row(c).Size(14, 14).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			if l.svg == nil {
				ui.Box(c).Size(8, 8).Radius(theme.RadiusFull).Background(theme.Working)
				return
			}
			fg := l.iconColor
			if fg.A == 0 {
				fg = col
			}
			ui.Icon(c, l.svg).FontSize(14).TextColor(fg)
		})
		if l.label != "" {
			ui.Text(c, l.label).FontSize(theme.TextUI).TextColor(col).SingleLine().Shrink(0)
		}
		if l.detail != "" {
			widget.Running(c.Key("detail"), l.detail, widget.RunStyle{Mono: true, Color: col})
		}
		if l.note != "" {
			widget.Running(c.Key("note"), l.note, widget.RunStyle{Color: lineNoteColor(l)})
		}
		if l.opens && (l.open || hovered || l.chevron) {
			chev := icon.ChevronRight
			if l.open {
				chev = icon.ChevronDown
			}
			ui.Icon(c, chev).FontSize(11).TextColor(theme.Muted).Shrink(0)
		}
		ui.Spacer(c)
		if l.right != nil {
			ui.Row(c).Gap(theme.S1).Shrink(0).AlignItems(ui.Center).Children(func() { l.right(hovered) })
		}
	})
	return head
}

// lineNoteColor is the note's: a failure's words read with its color.
func lineNoteColor(l lineLook) ui.Color {
	if l.color.A != 0 {
		return l.color
	}
	return theme.Muted
}

// lineBody is what a line opens to.
func lineBody(c *ui.Context, body func()) {
	ui.Column(c).FillWidth().Padding(theme.S1, 0, theme.S2, lineIndent).Gap(theme.S2).Transition(widget.PageMotion).Children(body)
}

// figures are the lines a change added and removed.
func figures(c *ui.Context, added, removed int) {
	if added > 0 {
		ui.Text(c, fmt.Sprintf("+%d", added)).FontSize(theme.TextUI).TextColor(theme.Add).SingleLine()
	}
	if removed > 0 {
		ui.Text(c, fmt.Sprintf("−%d", removed)).FontSize(theme.TextUI).TextColor(theme.Del).SingleLine()
	}
}

// doneVerb is how a tool's line reads once done, nowVerb while it runs.
func doneVerb(verb string) string {
	switch verb {
	case "Edit":
		return "Edited"
	case "Write":
		return "Wrote"
	case "Run", "Git":
		return "Ran"
	case "Search":
		return "Searched"
	case "List":
		return "Listed"
	case "Web":
		return "Looked up"
	case "Plan":
		return "Planned"
	}
	return verb
}

func nowVerb(verb string) string {
	switch verb {
	case "Read":
		return "Reading"
	case "Edit":
		return "Editing"
	case "Write":
		return "Writing"
	case "Run", "Git":
		return "Running"
	case "Search":
		return "Searching"
	case "List":
		return "Listing"
	case "Web":
		return "Looking up"
	case "Plan":
		return "Planning"
	}
	return verb
}

// lookOf is a tool call's icon and verb, a shell command's by what it does.
func lookOf(e proto.TranscriptEntry) (*ui.SVG, string) {
	if e.Text == "Bash" {
		return shellLook(e)
	}
	return toolLook(e.Text)
}

// nowLine is what the agent does now, at the end of the turn under way: the
// call it is at (cur, opening to it) or thinking, and how long it has been at
// the turn.
func (ch *Chat) nowLine(c *ui.Context, t turn, cur *proto.TranscriptEntry) {
	when := "Working"
	if t.prompt != nil && !t.prompt.Time.IsZero() {
		when += ", " + took(time.Since(t.prompt.Time).Milliseconds())
		c.After(time.Second)
	}
	l := lineLook{color: theme.Now, label: "Thinking", right: func(bool) {
		ui.Text(c, when).FontSize(theme.TextUI).TextColor(theme.Now).SingleLine()
	}}
	if cur == nil {
		drawLine(c, l)
		return
	}
	_, verb := lookOf(*cur)
	l.label, l.detail, l.name = nowVerb(verb), toolDetailShown(*cur), verb
	e := *cur
	ui.Column(c).FillWidth().Children(func() {
		ch.stepLine(c, e.Index, l, func() { ch.toolBody(c, e) })
	})
}
