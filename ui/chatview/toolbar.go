package chatview

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// What floats over the conversation, as a frame's corner holds little and
// the transcript runs under it: search within the chat, opening or folding
// every turn's work at once, and the button back to the latest; and at the
// conversation's end, the line of what the agent is at.

// chatTools are the search and the fold, in the frame's corner: bare beside a
// centered column, in a capsule where they'd lie over its text. turns are
// the list's rows, more after the row of older ones.
func (ch *Chat) chatTools(c *ui.Context, turns []turn, more int) {
	r := ui.Row(c.Key("chat-bar")).Absolute().Top(theme.S2).Right(theme.S3).Gap(theme.S1).AlignItems(ui.Center)
	if ch.width > 0 {
		r.MaxWidth(ch.width - 2*theme.S3)
	}
	if ch.finding || ch.toolsOverColumn() {
		widget.Rim(r.Padding(theme.S1).Radius(theme.RadiusFull).Background(theme.Popover))
	}
	r.Children(func() {
		if ch.finding {
			ch.findField(c, turns, more)
		} else if widget.IconButton(c.Key("find"), icon.Search, "Search the chat", false).Clicked() {
			ch.finding, ch.focusFind = true, true
		}
		label, open := "Open every step", true
		if ch.anyOpen(turns) {
			label, open = "Fold every step", false
		}
		if widget.IconButton(c.Key("fold"), icon.ChevronsUD, label, false).Clicked() {
			ch.foldAll(turns, open)
		}
	})
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		ch.finding, ch.focusFind = true, true
	}
}

// toolsOverColumn says the column leaves no room at its side for chatTools'
// two buttons and a margin.
func (ch *Chat) toolsOverColumn() bool {
	return ch.width < columnWidth+2*(2*theme.ControlHeight+theme.S5)
}

// statusLine ends the conversation, saying what the agent is at when it isn't
// working (the line of the turn under way says so then): waiting for the user,
// ended, or done and how long its last turn took. It also says where approvals
// show for an agent the chat can't answer for.
func (ch *Chat) statusLine(c *ui.Context, o Options, turns []turn) {
	// The last turn of the user's: notices after it are turns of their own.
	var last *turn
	for i := len(turns) - 1; i >= 0 && last == nil; i-- {
		if turns[i].prompt != nil {
			last = &turns[i]
		}
	}
	dot, text := theme.Muted, "Ready"
	switch {
	case o.Ended:
		dot, text = theme.Idle, "Ended"
	case ch.ask.waiting:
		dot, text = theme.Warning, "Waiting for you"
	case o.Working:
		text = ""
	case last != nil && last.prompt != nil:
		dot, text = theme.Done, "Done"
		if d := last.elapsed(); d > 0 {
			text += ", took " + took(d.Milliseconds())
		}
	}
	ui.Row(c).FillWidth().Height(lineHeight).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		if text != "" {
			ui.Row(c).Size(14, 14).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(8, 8).Radius(theme.RadiusFull).Background(dot)
			})
			ui.Text(c, text).FontSize(theme.TextUI).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		}
		ui.Spacer(c)
		if !ch.ask.canAnswer() {
			widget.Running(c.Key("hint"), "Approvals and menus show in the terminal", widget.RunStyle{Color: theme.Muted})
		}
	})
}

// hasStatus says the conversation ends with a status line.
func (ch *Chat) hasStatus(o Options) bool {
	return !o.Working || ch.ask.waiting || o.Ended || !ch.ask.canAnswer()
}

// anyOpen says some turn's work shows open.
func (ch *Chat) anyOpen(turns []turn) bool {
	for _, t := range turns {
		if ch.open[t.start] {
			return true
		}
	}
	return false
}

// foldAll opens every turn's work, or folds it, the steps in it with it.
func (ch *Chat) foldAll(turns []turn, open bool) {
	for _, t := range turns {
		if len(t.work) > 0 {
			ch.open[t.start] = open
		}
		if !open {
			for _, e := range t.work {
				delete(ch.open, e.Index)
				delete(ch.open, -(e.Index + 1))
			}
		}
	}
}

// findField is the search: the words to find, how many turns hold them,
// and moving from one to the next; a match in a turn's work opens it.
func (ch *Chat) findField(c *ui.Context, turns []turn, more int) {
	hits := findTurns(turns, ch.find)
	if ch.findAt >= len(hits) {
		ch.findAt = 0
	}
	goTo := func(i int) {
		if len(hits) == 0 {
			return
		}
		ch.findAt = (i + len(hits)) % len(hits)
		t := turns[hits[ch.findAt]]
		if turnWorkHas(t, ch.find) {
			ch.open[t.start] = true
		}
		ch.list.ScrollTo(hits[ch.findAt]+more, ui.Start)
	}
	ui.Row(c.Key("find-row")).MinWidth(0).Shrink(1).Gap(theme.S1).AlignItems(ui.Center).Children(func() {
		in := widget.Input(c.Key("find-in"), &ch.find, "Search the chat").Width(220).Shrink(1).MinWidth(64)
		if ch.focusFind {
			in.Focus()
			ch.focusFind = false
		}
		if ch.find != ch.found {
			ch.found, ch.findAt = ch.find, 0
			goTo(0)
		}
		if in.Submitted() {
			goTo(ch.findAt + 1)
		}
		count := "No match"
		switch {
		case strings.TrimSpace(ch.find) == "":
			count = ""
		case len(hits) > 0:
			count = fmt.Sprintf("%d of %d", ch.findAt+1, len(hits))
		}
		ui.Text(c, count).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Width(64).Shrink(0)
		if widget.IconButton(c.Key("prev"), icon.ChevronUp, "Previous match", false).Clicked() {
			goTo(ch.findAt - 1)
		}
		if widget.IconButton(c.Key("next"), icon.ChevronDown, "Next match (Enter)", false).Clicked() {
			goTo(ch.findAt + 1)
		}
		if widget.IconButton(c.Key("close-find"), icon.Close, "Close the search (Esc)", false).Clicked() || c.Shortcut(0, ui.KeyEscape) {
			ch.finding, ch.find = false, ""
		}
	})
}

// findTurns are the turns holding words, by their index; none for none.
func findTurns(turns []turn, words string) []int {
	words = strings.ToLower(strings.TrimSpace(words))
	if words == "" {
		return nil
	}
	var out []int
	for i, t := range turns {
		has := t.prompt != nil && strings.Contains(strings.ToLower(t.prompt.Text), words) || turnWorkHas(t, words)
		for _, e := range t.answer {
			has = has || strings.Contains(strings.ToLower(e.Text), words)
		}
		if has {
			out = append(out, i)
		}
	}
	return out
}

// turnWorkHas says words are in turn t's work: its thoughts, remarks,
// tool calls and their output.
func turnWorkHas(t turn, words string) bool {
	words = strings.ToLower(strings.TrimSpace(words))
	for _, e := range t.work {
		if strings.Contains(strings.ToLower(entryText(e)), words) {
			return true
		}
	}
	return false
}

// entryText is what an entry says, to search in.
func entryText(e proto.TranscriptEntry) string {
	return e.Text + "\n" + e.Detail + "\n" + e.Result
}

// toLatest is the button back to the conversation's latest, over the composer,
// while it is scrolled up away from it.
func (ch *Chat) toLatest(c *ui.Context, working bool) {
	if ch.list.AtEnd() {
		return
	}
	label := "Jump to the latest"
	if working {
		label = "Follow the agent"
	}
	ui.Row(c.Key("to-latest")).Absolute().Left(0).Bottom(ch.composerH + theme.S4 + theme.S3).FillWidth().Justify(ui.Center).PassThrough().Children(func() {
		b := ui.ButtonBase(c.Key("button")).Height(theme.ControlHeight).PaddingX(theme.S3).Gap(theme.S1).Radius(theme.RadiusFull).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Label(label)
		bg := theme.Popover
		if b.Hovered() {
			bg = theme.Solid(theme.Selected)
		}
		widget.Rim(b.Background(bg)).Children(func() {
			ui.Icon(c, icon.ArrowDown).FontSize(13).TextColor(theme.TextSoft)
			ui.Text(c, label).FontSize(theme.TextUI).TextColor(theme.Text).SingleLine()
		})
		if b.Clicked() {
			ch.list.ScrollToEnd()
		}
	})
}
