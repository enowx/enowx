package chatview

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// composer is where the user writes to the agent: what the agent is doing
// and what it changed, as chips over a box with the message and the
// agent's settings under it.
func (ch *Chat) composer(c *ui.Context, o Options) {
	ui.Row(c).FillWidth().Justify(ui.Center).Padding(4, 24, 16, 24).Children(func() {
		ui.Column(c).Grow(1).MaxWidth(columnWidth).Gap(8).Children(func() {
			ch.chips(c, o)
			ch.slashMenu(c)
			ch.box(c, o)
			ui.Row(c).FillWidth().Gap(14).Padding(0, 4).Children(func() {
				if ch.compact {
					ui.Text(c, "Enter to send · Shift+Enter: new line").FontSize(10.5).TextColor(theme.Idle).SingleLine()
					return
				}
				ui.Text(c, "Enter to send · Shift+Enter for a new line").FontSize(10.5).TextColor(theme.Idle).SingleLine()
				ui.Spacer(c)
				if !ch.ask.canAnswer() {
					ui.Text(c, "Approvals and menus show in the terminal").FontSize(10.5).TextColor(theme.Idle).SingleLine()
				}
			})
		})
	})
}

// chips say the lines the agent's edits changed; what it is doing shows
// in the bar over the conversation.
func (ch *Chat) chips(c *ui.Context, o Options) {
	added, removed := ch.added, ch.removed
	if added+removed == 0 {
		return
	}
	ui.Row(c).FillWidth().Gap(6).Children(func() {
		if added+removed > 0 {
			chip(c.Key("diff"), func() {
				ui.Text(c, fmt.Sprintf("+%d", added)).FontSize(11.5).TextColor(theme.Done).SingleLine()
				ui.Text(c, fmt.Sprintf("−%d", removed)).FontSize(11.5).TextColor(theme.Error).SingleLine()
				diffBar(c, added, removed)
			})
		}
	})
}

// chip is a small pill over the composer.
func chip(c *ui.Context, content func()) ui.Element {
	return ui.Row(c).Height(26).PaddingX(10).Gap(7).Radius(13).Shrink(1).MinWidth(0).
		Background(theme.Raised).Border(1, theme.Border).Children(content)
}

// diffBar is five blocks split between added and removed lines, as code
// hosts show a change.
func diffBar(c *ui.Context, added, removed int) {
	green := 5 * added / max(added+removed, 1)
	if added > 0 && green == 0 {
		green = 1
	}
	ui.Row(c).Gap(2).Children(func() {
		for i := range 5 {
			col := theme.Error
			if i < green {
				col = theme.Done
			}
			ui.Box(c.Key(i)).Size(6, 6).Radius(1.5).Background(col)
		}
	})
}

// compactWidth is the composer's width under which it shows its settings
// as icons: their labels and the context's tokens no longer fit.
const compactWidth = 600

// box is the message and the row of buttons under it.
func (ch *Chat) box(c *ui.Context, o Options) {
	box := ui.Column(c).FillWidth().Radius(16).Background(theme.Raised)
	// Narrow, as beside the side panel: the settings show as icons and the
	// context as its percentage, so nothing is cut.
	ch.compact = box.LayoutBounds().W > 0 && box.LayoutBounds().W < compactWidth
	border := theme.BorderStrong
	if box.FocusWithin() {
		border = theme.AccentLine
	}
	widget.Raise(box.Border(1, widget.Tint(box, "border", border))).Children(func() {
		ch.editBanner(c)
		ch.attachmentRow(c)
		ch.imageViewer(c)
		// The text area moves its caret with the arrows and breaks lines
		// with Enter itself, so the composer takes those keys first, with
		// OnTextKey: the slash menu its arrows, Tab, Enter and Escape;
		// Enter sends otherwise, Shift+Enter breaking the line.
		send := false
		in := ui.TextAreaBase(c, &ch.input).FillWidth().MinHeight(44).MaxHeight(220).Padding(12, 14, 4, 14).
			FontSize(13).Placeholder("Message " + o.AgentName + "…  / for commands, @ for files").Label("Message").
			OnPaste(func(text string) bool { return o.Paste != nil && o.Paste(text) }).
			OnTextKey(func(k ui.TextKey) bool {
				if k.Mods != 0 {
					return false
				}
				text := k.Text()
				if matches := ch.slashMatchesFor(text); len(matches) > 0 {
					switch k.Key {
					case ui.KeyDown:
						ch.slashPick = (ch.slashPick + 1) % len(matches)
						return true
					case ui.KeyUp:
						ch.slashPick = (ch.slashPick + len(matches) - 1) % len(matches)
						return true
					case ui.KeyTab, ui.KeyEnter:
						cmd := matches[min(ch.slashPick, len(matches)-1)]
						k.SetText("/" + cmd.Name + " ")
						ch.slashPick = 0
						return true
					case ui.KeyEscape:
						ch.slashClosed = text
						return true
					}
				}
				switch k.Key {
				case ui.KeyEscape:
					// Esc stops the agent, as in its terminal.
					if o.Working && o.Interrupt != nil {
						o.Interrupt()
						return true
					}
					return false
				case ui.KeyUp, ui.KeyDown:
					if next, ok := ch.recall(text, k.Key == ui.KeyUp); ok {
						k.SetText(next)
						return true
					}
					return false
				case ui.KeyEnter:
				default:
					ch.histAt = -1
					return false
				}
				if ch.send(o, text) {
					k.SetText("")
					ch.scrollToEnd, ch.histAt = true, -1
				}
				return true
			})
		// The composer has the focus when the chat shows, and gets it back
		// after a button, a menu or a send takes it.
		in.AutoFocus()
		if ch.focus {
			in.Focus()
			ch.focus = false
		}
		ui.Row(c).FillWidth().Gap(4).Padding(4, 8, 8, 8).Children(func() {
			if o.AttachFiles != nil && widget.IconButton(c.Key("attach"), icon.Paperclip, "Attach files or images (or drop or paste them)", false).Clicked() {
				o.AttachFiles()
			}
			if widget.IconButton(c.Key("mention"), icon.AtSign, "Mention a file (@)", false).Clicked() {
				ch.input += "@"
				ch.focus = true
			}
			if widget.IconButton(c, icon.Slash, "Commands (/)", false).Clicked() {
				if !strings.HasPrefix(ch.input, "/") {
					// Commands only run at the start of a message.
					ch.input, ch.slashClosed = "/"+ch.input, ""
				}
				ch.focus = true
			}
			ch.settings(c, o)
			ui.Spacer(c)
			if o.Spent != "" && !ch.compact {
				ui.Text(c.Key("spent"), o.Spent).FontSize(11).TextColor(theme.Muted).SingleLine().Shrink(1).Tooltip(o.SpentTip)
			}
			ch.contextMeter(c)
			if o.Working && strings.TrimSpace(ch.input) == "" && len(ch.attachments) == 0 {
				if roundButton(c, icon.Stop, "Interrupt (Esc)", false).Clicked() && o.Interrupt != nil {
					o.Interrupt()
					ch.focus = true
				}
				return
			}
			empty := strings.TrimSpace(ch.input) == "" && len(ch.attachments) == 0
			if roundButton(c, icon.ArrowUp, "Send (Enter)", empty || ch.attaching()).Clicked() {
				send = true
			}
		})
		if send && ch.send(o, ch.input) {
			ch.input, ch.scrollToEnd, ch.focus = "", true, true
		}
	})
}

// editBanner says the composer holds a message sent before, which sending
// replaces with what came after it, and offers to stop editing; while the
// agent goes back, it says so, and why when it couldn't.
func (ch *Chat) editBanner(c *ui.Context) {
	if ch.editing == nil {
		return
	}
	ui.Row(c.Key("editing")).FillWidth().Gap(8).Padding(8, 8, 0, 14).AlignItems(ui.Center).Children(func() {
		switch {
		case ch.editBusy:
			widget.Spinner(c, 12)
			ui.Text(c, "Going back to before this message…").FontSize(11.5).TextColor(theme.TextSoft).Grow(1).Shrink(1)
			return
		case ch.editErr != "":
			ui.Icon(c, icon.Alert).FontSize(12).TextColor(theme.Error)
			ui.Text(c, ch.editErr).FontSize(11.5).TextColor(theme.Error).Grow(1).Shrink(1)
		default:
			ui.Icon(c, icon.Pencil).FontSize(12).TextColor(theme.Accent)
			ui.Text(c, "Editing a message: sending it replaces it and what came after. Files stay as they are.").
				FontSize(11.5).TextColor(theme.TextSoft).Grow(1).Shrink(1)
		}
		if widget.IconButton(c, icon.Close, "Stop editing", false).Clicked() {
			ch.editing, ch.editErr, ch.input, ch.focus = nil, "", "", true
		}
	})
}

// contextMeter is how full the model's context is: a ring of the part
// used and the percentage, the token counts in its tooltip.
func (ch *Chat) contextMeter(c *ui.Context) {
	if ch.contextUsed == 0 {
		return
	}
	if ch.contextWindow == 0 {
		// The model's window isn't known: the count alone, no guess.
		tip := "Context: " + tokens(ch.contextUsed) + " tokens used. This model's window isn't known to enowX."
		ui.Row(c.Key("context")).Height(28).PaddingX(8).Radius(theme.RadiusControl).Shrink(0).Tooltip(tip).Label(tip).Children(func() {
			ui.Text(c, tokens(ch.contextUsed)+" tokens").FontSize(11).TextColor(theme.Muted).SingleLine()
		})
		return
	}
	fill := float64(ch.contextUsed) / float64(ch.contextWindow)
	col := widget.Level(fill)
	tip := fmt.Sprintf("Context: %s of %s tokens used. The agent compacts the conversation as it fills up.",
		tokens(ch.contextUsed), tokens(ch.contextWindow))
	ui.Row(c.Key("context")).Height(28).PaddingX(8).Gap(7).Radius(theme.RadiusControl).Shrink(0).Tooltip(tip).Label(tip).Children(func() {
		if !ch.compact {
			ui.Column(c).Width(44).Gap(3).Children(func() {
				widget.Meter(c, widget.Segment{Fraction: fill, Color: col})
			})
		}
		ui.Text(c, fmt.Sprintf("%.0f%%", 100*fill)).FontSize(11.5).TextColor(col).SingleLine()
		if !ch.compact {
			ui.Text(c, tokens(ch.contextUsed)+" / "+tokens(ch.contextWindow)).FontSize(11).TextColor(theme.Muted).SingleLine()
		}
	})
}

// tokens writes a token count short: 950, 140k, 1.2M.
func tokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e6), ".0") + "M"
	case n >= 1000:
		return fmt.Sprintf("%dk", (n+500)/1000)
	}
	return fmt.Sprint(n)
}

// roundButton is the composer's send or stop button.
func roundButton(c *ui.Context, svg *ui.SVG, label string, off bool) ui.Element {
	b := ui.ButtonBase(c).Size(30, 30).Radius(15).Center().Shrink(0).Label(label).Tooltip(label).Cursor(ui.CursorPointer).
		Gradient(theme.AccentHigh, theme.Accent, 180).TextColor(theme.OnAccent).InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.3))
	if off {
		b.Opacity(0.35).Disabled(true)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(14) })
	return widget.Pressable(b)
}

// settings are the chips of what the agent lets the chat change: its
// model, its permission mode, its effort.
func (ch *Chat) settings(c *ui.Context, o Options) {
	set := agent.SettingsOf(o.Agent)
	if len(set.Models) > 0 {
		ch.setting(c, o, "model", icon.CPU, set.Models, ch.model, "Model")
	}
	if len(set.Modes) > 0 && set.ModeKey != "" {
		ch.setting(c, o, "mode", icon.Shield, set.Modes, ch.mode, cmp.Or(set.ModeTitle, "Permissions"))
	}
	if len(set.Efforts) > 0 {
		ch.setting(c, o, "effort", icon.Gauge, set.Efforts, ch.effort, "Effort")
	}
}

// setting is a chip showing a setting's value that opens the list of its
// values; picking one asks the agent for it.
func (ch *Chat) setting(c *ui.Context, o Options, key string, svg *ui.SVG, list []agent.Choice, current, title string) {
	shown := current
	if p, ok := ch.pending[key]; ok {
		shown = p.id
	}
	cur, known := agent.FindChoice(list, shown)
	label := cur.Label
	switch {
	case !known && shown != "":
		label = shown
	case !known:
		label = title
	}
	b := ui.ButtonBase(c.Key(key)).Height(28).PaddingX(9).Gap(6).Radius(theme.RadiusControl).Shrink(0).
		Label(title).Tooltip(title + ": " + label).Cursor(ui.CursorPointer).TextColor(theme.TextSoft)
	if ch.compact {
		b.PaddingX(6).Gap(3)
	}
	bg := theme.Fade(theme.Hover, 0)
	if b.Hovered() || ch.menu == key {
		bg = theme.Hover
	}
	b.Background(widget.Tint(b, "bg", bg)).Children(func() {
		ui.Icon(c, svg).FontSize(13)
		if !ch.compact {
			ui.Text(c, label).FontSize(12).SingleLine()
		}
		ui.Icon(c, icon.ChevronDown).FontSize(10).TextColor(theme.Muted)
	})
	if widget.Pressable(b).Clicked() {
		if ch.menu == key {
			ch.menu = ""
		} else {
			ch.menu = key
		}
	}
	if ch.menu != key {
		return
	}
	open := true
	ui.PopoverBase(c, b, &open, func(panel ui.Element) {
		widget.Popup(panel.AttachTo(b, ui.AnchorTopLeft, ui.AnchorBottomLeft).Margin(0, 0, 6, 0).Width(250))
		widget.PopupLabel(c, strings.ToUpper(title))
		for _, choice := range list {
			item := ui.ButtonBase(c.Key(choice.ID)).Label(choice.Label).Cursor(ui.CursorPointer)
			widget.PopupItem(item, item.Hovered()).Children(func() {
				ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, choice.Label).FontSize(12.5)
					if choice.Note != "" {
						ui.Text(c, choice.Note).FontSize(11).TextColor(theme.Muted)
					}
				})
				if known && choice.ID == cur.ID {
					ui.Icon(c, icon.Check).FontSize(13).TextColor(theme.Accent)
				}
			})
			if item.Clicked() {
				ch.pending[key] = pendingSet{id: choice.ID, at: time.Now()}
				ch.menu, open, ch.focus = "", false, true
				if o.Set != nil {
					o.Set(key, choice.ID)
				}
			}
		}
	})
	if !open {
		ch.menu, ch.focus = "", true
	}
}

// recall is the user's message before (up) or after the one the composer
// shows again, to send again or change: ↑ in an empty composer starts
// from the latest. False leaves the keys to the text.
func (ch *Chat) recall(text string, up bool) (string, bool) {
	var sent []string
	for _, e := range ch.entries {
		if e.Kind == proto.TranscriptUser && strings.TrimSpace(e.Text) != "" {
			sent = append(sent, e.Text)
		}
	}
	shown := func(i int) string { return sent[len(sent)-1-i] }
	switch {
	case len(sent) == 0:
		return "", false
	case ch.histAt < 0:
		if !up || text != "" {
			return "", false
		}
		ch.histAt = 0
	case text != shown(ch.histAt):
		ch.histAt = -1 // changed since: the arrows move in it
		return "", false
	case up && ch.histAt < len(sent)-1:
		ch.histAt++
	case !up && ch.histAt > 0:
		ch.histAt--
	case !up:
		ch.histAt = -1
		return "", true
	default:
		return "", false
	}
	return shown(ch.histAt), true
}
