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

// askNode is the tool call e the agent waits on: its questions, or the
// tool to allow.
func (ch *Chat) askNode(c *ui.Context, e proto.TranscriptEntry, last bool) {
	if len(e.Questions) > 0 {
		node(c, icon.Chat, toneAsk, last, func() { ch.questionCard(c, e) })
		return
	}
	node(c, icon.Shield, toneAsk, last, func() { ch.permissionCard(c, e) })
}

// waitNode stands for the agent waiting on something the chat can't show:
// its terminal answers it.
func (ch *Chat) waitNode(c *ui.Context) {
	node(c, icon.Shield, toneAsk, true, func() {
		askCard(c, func() {
			ui.Text(c, "The agent waits for you").FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong)
			ui.Text(c, "It asks something its terminal shows.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			ch.terminalButton(c)
		})
	})
}

// askCard is the card an ask shows in: lit, so it isn't missed.
func askCard(c *ui.Context, body func()) {
	ui.Column(c).FillWidth().Gap(8).Padding(12, 14).Radius(theme.RadiusSmall).
		Background(theme.Fade(theme.Warning, 0.08)).Border(1, theme.Fade(theme.Warning, 0.7)).Children(body)
}

// permissionCard asks to allow tool call e: what it would do, and allow
// once, allow from now on, or deny.
func (ch *Chat) permissionCard(c *ui.Context, e proto.TranscriptEntry) {
	askCard(c, func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Column(c).Grow(1).MinWidth(200).Gap(3).Children(func() {
				ui.Text(c, permissionTitle(e)).FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong)
				if e.Detail != "" {
					ui.Text(c, e.Detail).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Selectable()
				}
			})
			if !ch.ask.canAnswer() {
				ch.terminalButton(c)
				return
			}
			if ch.answered[e.Index] > 0 {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					widget.Spinner(c, 11)
					ui.Text(c, "Sent").FontSize(theme.TextLabel).TextColor(theme.Muted)
				})
				return
			}
			ui.Row(c).Gap(6).Shrink(0).Children(func() {
				send := func(keys string) {
					ch.answered[e.Index] = 1
					ch.ask.answer(keys)
				}
				if widget.Button(c.Key("deny"), widget.Secondary, nil, "Deny", "").Height(30).Clicked() {
					send(ch.ask.keys.Deny)
				}
				if ch.ask.keys.Always != "" && widget.Button(c.Key("always"), widget.Secondary, nil, "Always allow", "").Height(30).
					Tooltip("Allow it, and don't ask again for the like of it").Clicked() {
					send(ch.ask.keys.Always)
				}
				if widget.Button(c.Key("allow"), widget.Primary, nil, "Allow", "").Height(30).Clicked() {
					send(ch.ask.keys.Allow)
				}
			})
		})
	})
}

// permissionTitle is what the agent asks to do, by its tool.
func permissionTitle(e proto.TranscriptEntry) string {
	_, verb := toolLook(e.Text)
	switch verb {
	case "Run":
		return "Run this command?"
	case "Edit", "Write":
		return "Change this file?"
	case "Read":
		return "Read this file?"
	case "Search":
		return "Search here?"
	case "Web":
		return "Go to the web?"
	}
	return "Use " + verb + "?"
}

// questionCard is the agent's questions, the next unanswered one showing:
// its options as buttons, and an answer of the user's own words.
func (ch *Chat) questionCard(c *ui.Context, e proto.TranscriptEntry) {
	at := ch.answered[e.Index]
	askCard(c, func() {
		if at >= len(e.Questions) {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				widget.Spinner(c, 11)
				ui.Text(c, "Answered").FontSize(theme.TextLabel).TextColor(theme.Muted)
			})
			if ch.ask.terminal != nil {
				ui.Text(c, "If the agent asks you to confirm, its terminal shows it.").FontSize(theme.TextLabel).TextColor(theme.Muted)
				ch.terminalButton(c)
			}
			return
		}
		q := e.Questions[at]
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if q.Header != "" {
				widget.Tag(c, q.Header, theme.Warning)
			}
			if len(e.Questions) > 1 {
				ui.Text(c, strconv.Itoa(at+1)+" of "+strconv.Itoa(len(e.Questions))).FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
		})
		ui.Text(c, q.Question).FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong).Selectable()
		if !ch.ask.canAnswer() || q.Multi {
			for i, o := range q.Options {
				ui.Text(c.Key(i), "· "+o).FontSize(theme.TextUI).TextColor(theme.TextSoft)
			}
			ch.terminalButton(c)
			return
		}
		pick := func(keys string) {
			ch.answered[e.Index] = at + 1
			ch.ask.answer(keys)
		}
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			for i, o := range q.Options {
				b := ui.ButtonBase(c.Key(i)).FillWidth().Padding(7, 10).Gap(10).Radius(theme.RadiusControl).Justify(ui.Start).
					AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(o).Background(theme.Raised).Border(1, theme.BorderStrong)
				if b.Hovered() {
					b.Border(1, theme.AccentLine)
				}
				b.Children(func() {
					ui.Text(c, strconv.Itoa(i+1)).FontSize(theme.TextLabel).Font(theme.MonoFont).TextColor(theme.Muted)
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, o).FontSize(theme.TextUI).TextColor(theme.Text)
						if i < len(q.About) && q.About[i] != "" {
							ui.Text(c, q.About[i]).FontSize(theme.TextLabel).TextColor(theme.Muted)
						}
					})
				})
				if b.Clicked() {
					pick(strconv.Itoa(i + 1))
				}
			}
		})
		// The user's own answer: the option after the last, then the text.
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			in := widget.Input(c.Key("own"), &ch.ownAnswer, "Or answer in your own words").Grow(1)
			if (in.Submitted() || widget.Button(c.Key("send-own"), widget.Secondary, nil, "Send", "").Height(30).Clicked()) && strings.TrimSpace(ch.ownAnswer) != "" {
				pick(strconv.Itoa(len(q.Options)+1) + strings.TrimSpace(ch.ownAnswer) + "\r")
				ch.ownAnswer = ""
			}
		})
	})
}

// terminalButton shows the session's terminal, to answer there.
func (ch *Chat) terminalButton(c *ui.Context) {
	if ch.ask.terminal == nil {
		return
	}
	if widget.Button(c.Key("terminal"), widget.Secondary, icon.Terminal, "Answer in the terminal", "").Height(30).Clicked() {
		ch.ask.terminal()
	}
}
