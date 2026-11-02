package chatview

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The agent waiting on the user, in the timeline: a tool it asks to use,
// to allow or deny, or its questions, each option a button. Answers are
// the keys its terminal takes (the preset's); an agent whose keys aren't
// known is answered in its terminal.

// asking is what the agent waits on the user for, as of this frame.
type asking struct {
	waiting  bool
	answer   func(keys string)
	terminal func()
	keys     agent.ApproveKeys
}

// canAnswer says the chat can answer for the agent.
func (a asking) canAnswer() bool { return a.answer != nil && a.keys.Allow != "" }

// asks says entry e is what the agent waits on: the tool call under way
// while it waits.
func (ch *Chat) asks(e proto.TranscriptEntry, latest bool) bool {
	return ch.ask.waiting && latest && e.Kind == proto.TranscriptTool && !e.Done
}

// askCardOf is the tool call e the agent waits on: its questions, or the
// tool to allow.
func (ch *Chat) askCardOf(c *ui.Context, e proto.TranscriptEntry) {
	if len(e.Questions) > 0 {
		ch.questionCard(c, e)
		return
	}
	ch.permissionCard(c, e)
}

// waitCard stands for the agent waiting on something the chat can't show:
// its terminal answers it.
func (ch *Chat) waitCard(c *ui.Context) {
	askCard(c, func() {
		ui.Text(c, "The agent waits for you").FontSize(theme.TextUI).FontWeight(500).TextColor(theme.Text)
		ui.Text(c, "It asks something its terminal shows.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		ui.Row(c).FillWidth().Children(func() { ch.terminalButton(c) })
	})
}

// askCard is the card an ask shows in, a dark piece on the frame that the
// plain conversation around it doesn't have, so it isn't missed.
func askCard(c *ui.Context, body func()) {
	ui.Column(c).FillWidth().Gap(theme.S2).Padding(theme.S3, theme.S4).Radius(theme.RadiusCard).
		Background(theme.Fade(theme.Popover, 0.72)).Children(body)
}

// permissionCard asks to allow tool call e: what it would do, and allow
// once, allow from now on, or deny.
func (ch *Chat) permissionCard(c *ui.Context, e proto.TranscriptEntry) {
	askCard(c, func() {
		ui.Text(c, permissionTitle(e, ch.agentName)).FontSize(theme.TextUI).FontWeight(500).TextColor(theme.Text)
		if e.Detail != "" {
			widget.CodeBlockText(c, e.Detail)
		}
		ui.Row(c).FillWidth().Gap(theme.S2).Wrap().AlignItems(ui.Center).Children(func() {
			if !ch.ask.canAnswer() {
				ch.terminalButton(c)
				return
			}
			if ch.answered[e.Index] > 0 {
				widget.Spinner(c, 12)
				ui.Text(c, "Sent").FontSize(theme.TextUI).TextColor(theme.TextSoft)
				return
			}
			send := func(keys string) {
				ch.answered[e.Index] = 1
				ch.ask.answer(keys)
			}
			if widget.Button(c.Key("allow"), widget.Primary, nil, "Allow", "").Clicked() {
				send(ch.ask.keys.Allow)
			}
			if ch.ask.keys.Always != "" && widget.Button(c.Key("always"), widget.Secondary, nil, "Always allow", "").
				Tooltip("Allow it, and don't ask again for the like of it").Clicked() {
				send(ch.ask.keys.Always)
			}
			if widget.Button(c.Key("deny"), widget.Secondary, nil, "Deny", "").Clicked() {
				send(ch.ask.keys.Deny)
			}
		})
	})
}

// permissionTitle is what the agent, called who, asks to do, by its tool.
func permissionTitle(e proto.TranscriptEntry, who string) string {
	if who == "" {
		who = "The agent"
	}
	_, verb := toolLook(e.Text)
	switch verb {
	case "Run":
		return who + " asks to run a command"
	case "Edit", "Write":
		return who + " asks to change this file"
	case "Read":
		return who + " asks to read this file"
	case "Search":
		return who + " asks to search here"
	case "Web":
		return who + " asks to go to the web"
	}
	return who + " asks to use " + verb
}

// shortOptions says a question's options read as small buttons in a line: short,
// with nothing to say about them. Otherwise each takes a row.
func shortOptions(q proto.TranscriptQuestion) bool {
	for i, o := range q.Options {
		if len([]rune(o)) > 48 || i < len(q.About) && q.About[i] != "" {
			return false
		}
	}
	return true
}

// questionCard is the agent's questions, the next unanswered one showing:
// its options as buttons, and an answer of the user's own words.
func (ch *Chat) questionCard(c *ui.Context, e proto.TranscriptEntry) {
	at := ch.answered[e.Index]
	askCard(c, func() {
		if at >= len(e.Questions) {
			ui.Row(c).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
				widget.Spinner(c, 12)
				ui.Text(c, "Answered").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			})
			if ch.ask.terminal != nil {
				ui.Text(c, "If the agent asks you to confirm, its terminal shows it.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
				ui.Row(c).FillWidth().Children(func() { ch.terminalButton(c) })
			}
			return
		}
		q := e.Questions[at]
		if q.Header != "" || len(e.Questions) > 1 {
			ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
				if q.Header != "" {
					widget.Tag(c, q.Header, theme.Warning)
				}
				if len(e.Questions) > 1 {
					ui.Text(c, strconv.Itoa(at+1)+" of "+strconv.Itoa(len(e.Questions))).FontSize(theme.TextUI).TextColor(theme.TextSoft)
				}
			})
		}
		ui.Text(c, q.Question).FontSize(theme.TextUI).FontWeight(500).TextColor(theme.Text).Selectable()
		if !ch.ask.canAnswer() || q.Multi {
			for i, o := range q.Options {
				ui.Text(c.Key(i), "· "+o).FontSize(theme.TextUI).TextColor(theme.TextSoft)
			}
			ui.Row(c).FillWidth().Children(func() { ch.terminalButton(c) })
			return
		}
		pick := func(keys string) {
			ch.answered[e.Index] = at + 1
			ch.ask.answer(keys)
		}
		if shortOptions(q) {
			ui.Row(c).FillWidth().Gap(theme.S2).Wrap().Children(func() {
				for i, o := range q.Options {
					kind := widget.Secondary
					if i == 0 {
						kind = widget.Primary
					}
					if widget.Button(c.Key(i), kind, nil, o, "").Clicked() {
						pick(strconv.Itoa(i + 1))
					}
				}
			})
		} else {
			ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
				for i, o := range q.Options {
					if optionRow(c.Key(i), i, o, q.About).Clicked() {
						pick(strconv.Itoa(i + 1))
					}
				}
			})
		}
		// The user's own answer: the option after the last, then the text.
		ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			in := widget.Input(c.Key("own"), &ch.ownAnswer, "Or answer in your own words").Grow(1).MinWidth(0)
			if (in.Submitted() || widget.Button(c.Key("send-own"), widget.Secondary, nil, "Send", "").Clicked()) && strings.TrimSpace(ch.ownAnswer) != "" {
				pick(strconv.Itoa(len(q.Options)+1) + strings.TrimSpace(ch.ownAnswer) + "\r")
				ch.ownAnswer = ""
			}
		})
	})
}

// optionRow is option o of a question, its number the key the terminal takes,
// with what about says of it.
func optionRow(c *ui.Context, i int, o string, about []string) ui.Element {
	b := ui.ButtonBase(c).FillWidth().Padding(theme.S2, theme.S3).Gap(theme.S3).Radius(theme.RadiusSmall).Justify(ui.Start).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(o)
	bg := theme.Raised
	if b.Hovered() {
		bg = theme.Selected
	}
	b.Background(widget.QuickTint(b, "bg", bg)).Children(func() {
		ui.Text(c, strconv.Itoa(i+1)).FontSize(theme.TextUI).TextColor(theme.TextSoft).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, o).FontSize(theme.TextUI).TextColor(theme.Text)
			if i < len(about) && about[i] != "" {
				ui.Text(c, about[i]).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			}
		})
	})
	return b
}

// terminalButton shows the session's terminal, to answer there.
func (ch *Chat) terminalButton(c *ui.Context) {
	if ch.ask.terminal == nil {
		return
	}
	if widget.Button(c.Key("terminal"), widget.Secondary, icon.Terminal, "Answer in the terminal", "").Clicked() {
		ch.ask.terminal()
	}
}
