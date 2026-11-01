package chatview

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The bar over the conversation: what the agent is at (working and for
// how long, waiting for the user, done), search within the chat, and
// opening or folding every turn's work at once; and over the end of a
// conversation scrolled up, the button back to its latest.

// toolbar draws the bar over turns, the list's rows more after the row of
// older ones.
func (ch *Chat) toolbar(c *ui.Context, o Options, turns []turn, more int) {
	ui.Row(c.Key("chat-bar")).FillWidth().Height(38).Shrink(0).Padding(0, 12).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ch.status(c, o, turns)
		ui.Spacer(c)
		if ch.finding {
			ch.findField(c, turns, more)
		} else if widget.IconButton(c.Key("find"), icon.Search, "Search the chat", false).Clicked() {
			ch.finding, ch.focusFind = true, true
		}
		label, open := "Open every step", true
		if ch.anyOpen(turns) {
			label, open = "Fold every step", false
		}
		svg := icon.ChevronsUD
		if widget.IconButton(c.Key("fold"), svg, label, false).Clicked() {
			ch.foldAll(turns, open)
		}
	})
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		ch.finding, ch.focusFind = true, true
	}
}

// status is what the agent is at: working and for how long, waiting for
// the user, ended, or done and how long its last turn took.
func (ch *Chat) status(c *ui.Context, o Options, turns []turn) {
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
		dot, text = theme.Working, "Working"
		if last != nil && last.prompt != nil && !last.prompt.Time.IsZero() {
			text += " · " + took(time.Since(last.prompt.Time).Milliseconds())
			c.After(time.Second)
		}
	case last != nil && last.prompt != nil:
		dot, text = theme.Done, "Done"
		if d := last.elapsed(); d > 0 {
			text += " · took " + took(d.Milliseconds())
		}
	}
	ui.Row(c).Gap(7).AlignItems(ui.Center).Shrink(1).MinWidth(0).Children(func() {
		if o.Working && !ch.ask.waiting {
			widget.Spinner(c, 11)
		} else {
			ui.Box(c).Size(7, 7).Radius(theme.RadiusTail).Background(dot)
		}
		ui.Text(c, text).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		// While it works, the step it is at.
		if l := ch.latestTool; o.Working && !ch.ask.waiting && l != nil {
			_, verb := toolLook(l.Text)
			ui.Text(c, verb+" "+l.Detail).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1).MaxWidth(360)
		}
	})
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
	ui.Row(c.Key("find-row")).Gap(4).AlignItems(ui.Center).Children(func() {
		in := widget.Input(c.Key("find-in"), &ch.find, "Search the chat").Width(220).Height(28)
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
		ui.Text(c, count).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Width(64)
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

// toLatest is the button back to the conversation's latest, while it is
// scrolled up away from it.
func (ch *Chat) toLatest(c *ui.Context, working bool) {
	if ch.list.AtEnd() {
		return
	}
	label := "Jump to the latest"
	if working {
		label = "Follow the agent"
	}
	ui.Row(c.Key("to-latest")).Absolute().Left(0).Bottom(14).FillWidth().Justify(ui.Center).PassThrough().Children(func() {
		b := ui.ButtonBase(c.Key("button")).Height(30).Padding(0, 12).Gap(6).Radius(theme.RadiusCard).AlignItems(ui.Center).
			Background(theme.Raised).Border(1, theme.BorderStrong).Cursor(ui.CursorPointer).Label(label)
		if b.Hovered() {
			b.Background(theme.Hover)
		}
		b.Children(func() {
			ui.Icon(c, icon.ArrowDown).FontSize(12).TextColor(theme.TextSoft)
			ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
		})
		if b.Clicked() {
			ch.list.ScrollToEnd()
		}
	})
}
