package chatview

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// composer is where the user writes to the agent: a capsule floating over the
// frame's foot, the conversation scrolling under it, with the message over a
// row of the agent's settings. What it says of the conversation (the lines
// changed, the context) keeps to the row's free end.
func (ch *Chat) composer(c *ui.Context, o Options) {
	ui.Row(c.Key("composer")).Absolute().Left(0).Bottom(theme.S4).FillWidth().Justify(ui.Center).PaddingX(theme.S5).PassThrough().Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).MaxWidth(o.columnWidth()).Gap(theme.S2).PassThrough().Children(func() {
			ch.slashMenu(c)
			ch.box(c, o)
		})
	})
}

// diffChip says the lines the agent's edits changed, with five blocks once
// there's room for them.
func (ch *Chat) diffChip(c *ui.Context, bar bool) {
	ui.Row(c.Key("diff")).Gap(theme.S1).Shrink(0).AlignItems(ui.Center).Children(func() {
		figures(c, ch.added, ch.removed)
		if bar {
			diffBar(c, ch.added, ch.removed)
		}
	})
}

// diffBar is five blocks split between added and removed lines, as code
// hosts show a change.
func diffBar(c *ui.Context, added, removed int) {
	green := 5 * added / max(added+removed, 1)
	if added > 0 && green == 0 {
		green = 1
	}
	ui.Row(c).Gap(2).Padding(0, 0, 0, theme.S1).Children(func() {
		for i := range 5 {
			col := theme.Del
			if i < green {
				col = theme.Add
			}
			ui.Box(c.Key(i)).Size(6, 6).Radius(2).Background(col)
		}
	})
}

// compactWidth is the composer's width under which it shows its settings
// as icons: their labels and the context's tokens no longer fit. The row's
// other figures go by room: diffBarWidth for the blocks of a change, and
// spentRoom is the least the tokens spent read in.
const (
	compactWidth = 600
	diffBarWidth = 700
	spentRoom    = 72
)

// box is the message and the row of buttons under it.
func (ch *Chat) box(c *ui.Context, o Options) {
	box := ui.Column(c).FillWidth().Padding(theme.S3, theme.S2, theme.S2, theme.S2).Gap(theme.S1).Radius(theme.RadiusPanel).Background(theme.Popover).
		InsetShadow(0, 0, 0, 0.5, theme.Border).Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, float32(0.35*theme.SurfaceOpacity)))
	// Narrow, as beside the side panel: the settings show as icons and the
	// context as its percentage, so nothing is cut.
	width := box.LayoutBounds().W
	ch.compact = width > 0 && width < compactWidth
	if h := box.LayoutBounds().H; h > 0 {
		ch.composerH = h
	}
	// An inset ring, as a border would move what is inside when focus comes.
	if box.FocusWithin() {
		box.InsetShadow(0, 0, 0, 1, theme.Now)
	}
	box.Children(func() {
		ch.editBanner(c)
		ch.attachmentRow(c)
		ch.imageViewer(c)
		// The text area moves its caret with the arrows and breaks lines
		// with Enter itself, so the composer takes those keys first, with
		// OnTextKey: the slash menu its arrows, Tab, Enter and Escape;
		// Enter sends otherwise, Shift+Enter breaking the line.
		send := false
		in := ui.TextAreaBase(c, &ch.input).FillWidth().Lines(1, 5).Padding(0, widget.ScrollGutter, 0, theme.S2).
			FontSize(theme.TextRead).LineHeight(readLeading).Placeholder(composerPlaceholder(o)).Label("Message").
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
		if ch.caretEnd {
			n := utf8.RuneCountInString(ch.input)
			in.SetTextSelection(n, n)
			ch.caretEnd = false
		}
		if ch.focus {
			in.Focus()
			ch.focus = false
		}
		ui.Row(c).FillWidth().Gap(theme.S1).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Shrink(0).Gap(theme.S1).AlignItems(ui.Center).Children(func() {
				if o.AttachFiles != nil && widget.IconButton(c.Key("attach"), icon.Paperclip, "Attach files or images (or drop or paste them)", false).Clicked() {
					o.AttachFiles()
				}
				ch.settings(c, o)
			})
			right := ui.Row(c.Key("row-end")).Grow(1).MinWidth(0).Justify(ui.End).Gap(theme.S2).AlignItems(ui.Center)
			room := right.LayoutBounds().W
			right.Children(func() {
				empty := strings.TrimSpace(ch.input) == "" && len(ch.attachments) == 0
				stops := o.Working
				// What goes before the buttons takes what they leave.
				room -= theme.S2 + roundSize
				if stops && !empty {
					room -= theme.S2 + roundSize
				}
				room -= ch.contextWidth()
				bar := width >= diffBarWidth
				if ch.added+ch.removed > 0 && !ch.compact {
					room -= theme.S2 + diffWidth(bar)
				}
				if o.Spent != "" && !ch.compact && room >= spentRoom {
					ui.Row(c.Key("spent")).MinWidth(0).Shrink(1).Children(func() {
						widget.Running(c, o.Spent, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft}).Tooltip(o.SpentTip)
					})
				}
				if ch.added+ch.removed > 0 && !ch.compact {
					ch.diffChip(c, bar)
				}
				ch.contextMeter(c)
				if stops {
					// With a message typed, sending is the main action and stopping steps back.
					if roundButton(c.Key("stop"), icon.Stop, "Stop "+o.AgentName+" (Esc)", false, !empty).Clicked() && o.Interrupt != nil {
						o.Interrupt()
						ch.focus = true
					}
					if empty {
						return
					}
				}
				if roundButton(c.Key("send"), icon.ArrowUp, "Send (Enter)", empty || ch.attaching(), false).
					Tooltip("Send (Enter). Shift+Enter makes a new line.").Clicked() {
					send = true
				}
			})
		})
		if send && ch.send(o, ch.input) {
			ch.input, ch.scrollToEnd, ch.focus = "", true, true
		}
	})
}

// diffWidth is about how wide diffChip runs.
func diffWidth(bar bool) float32 {
	if bar {
		return 112
	}
	return 72
}

// editBanner says the composer holds a message sent before, which sending
// replaces with what came after it, and offers to stop editing; while the
// agent goes back, it says so, and why when it couldn't.
func (ch *Chat) editBanner(c *ui.Context) {
	if ch.editing == nil {
		return
	}
	ui.Row(c.Key("editing")).FillWidth().Gap(theme.S2).Padding(0, 0, theme.S1, theme.S2).AlignItems(ui.Center).Children(func() {
		switch {
		case ch.editBusy:
			widget.Spinner(c, 12)
			ui.Text(c, "Going back to before this message…").FontSize(theme.TextUI).TextColor(theme.TextSoft).Grow(1).Shrink(1)
			return
		case ch.editErr != "":
			ui.Icon(c, icon.Alert).FontSize(14).TextColor(theme.Del).Shrink(0)
			ui.Text(c, ch.editErr).FontSize(theme.TextUI).TextColor(theme.Del).Grow(1).Shrink(1)
		default:
			ui.Icon(c, icon.Pencil).FontSize(14).TextColor(theme.Now).Shrink(0)
			ui.Text(c, "Editing a message: sending it replaces it and what came after. Files stay as they are.").
				FontSize(theme.TextUI).TextColor(theme.TextSoft).Grow(1).Shrink(1)
		}
		if widget.IconButton(c, icon.Close, "Stop editing", false).Clicked() {
			ch.editing, ch.editErr, ch.input, ch.focus = nil, "", "", true
		}
	})
}

// contextWidth is about how wide contextMeter runs, with its gap.
func (ch *Chat) contextWidth() float32 {
	switch {
	case ch.contextUsed == 0:
		return 0
	case ch.compact:
		return theme.S2 + 40
	case ch.contextWindow == 0:
		return theme.S2 + 80
	}
	return theme.S2 + 92
}

// contextMeter is how full the model's context is: a meter of the part used
// and the percentage, the token counts in its tooltip.
func (ch *Chat) contextMeter(c *ui.Context) {
	if ch.contextUsed == 0 {
		return
	}
	if ch.contextWindow == 0 {
		// The model's window isn't known: the count alone, no guess.
		tip := "Context: " + tokens(ch.contextUsed) + " tokens used. This model's window isn't known to enowX."
		ui.Row(c.Key("context")).Shrink(0).Tooltip(tip).Label(tip).Children(func() {
			ui.Text(c, tokens(ch.contextUsed)+" tokens").FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
		})
		return
	}
	fill := float64(ch.contextUsed) / float64(ch.contextWindow)
	col := widget.Level(fill)
	tip := fmt.Sprintf("Context: %s of %s tokens used. The agent compacts the conversation as it fills up.",
		tokens(ch.contextUsed), tokens(ch.contextWindow))
	ui.Row(c.Key("context")).Gap(theme.S2).Shrink(0).AlignItems(ui.Center).Tooltip(tip).Label(tip).Children(func() {
		if !ch.compact {
			ui.Column(c).Width(44).Children(func() {
				widget.Meter(c, widget.Segment{Fraction: fill, Color: col})
			})
		}
		ui.Text(c, fmt.Sprintf("%.0f%%", 100*fill)).FontSize(theme.TextLabel).TextColor(col).SingleLine()
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

// roundSize is the send and stop buttons' size.
const roundSize = 36

// roundButton is the composer's send or stop button: in the accent, or
// quiet when back is set.
func roundButton(c *ui.Context, svg *ui.SVG, label string, off, back bool) ui.Element {
	b := ui.ButtonBase(c).Size(roundSize, roundSize).Radius(theme.RadiusFull).Center().Shrink(0).Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	if back {
		bg := theme.Selected
		if b.Hovered() {
			bg = theme.Hover
		}
		b.Background(widget.QuickTint(b, "bg", bg)).TextColor(theme.Text)
	} else {
		bg := theme.Accent
		if b.Hovered() && !off {
			bg = theme.AccentHigh
		}
		b.Background(widget.QuickTint(b, "bg", bg)).TextColor(theme.OnAccent)
	}
	if off {
		b.Opacity(0.45).Disabled(true)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(14) })
	return widget.Pressable(b)
}

// settings are the options of what the agent lets the chat change: its
// model, its permission mode, its effort.
func (ch *Chat) settings(c *ui.Context, o Options) {
	set := agent.SettingsOf(o.Agent)
	if len(set.Models) > 0 {
		ch.setting(c, o, "model", icon.CPU, set.Models, ch.model, "Model", "", modelWidth)
	}
	if len(set.Modes) > 0 && set.ModeKey != "" {
		ch.setting(c, o, "mode", icon.Shield, set.Modes, ch.mode, cmp.Or(set.ModeTitle, "Permissions"), "", optionWidth)
	}
	if len(set.Efforts) > 0 {
		ch.setting(c, o, "effort", icon.Gauge, set.Efforts, ch.effort, "Effort", " effort", optionWidth)
	}
}

// The room a setting's option takes in the row: the model's keeps its width,
// so that changing it doesn't move the others; the rest's are as much as they
// take, up to this, and run when longer.
const (
	modelWidth  = 132
	optionWidth = 120
)

// setting is an option showing a setting's value (with suffix after it) that
// opens the list of its values; picking one asks the agent for it.
func (ch *Chat) setting(c *ui.Context, o Options, key string, svg *ui.SVG, list []agent.Choice, current, title, suffix string, width float32) {
	shown := current
	if p, ok := ch.pending[key]; ok {
		shown = p.id
	}
	cur, known := agent.FindChoice(list, shown)
	label := cur.Label + suffix
	switch {
	case !known && shown != "":
		label = shown
	case !known:
		label = title
	}
	b := ui.ButtonBase(c.Key(key)).Height(theme.ControlHeight).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).Shrink(0).AlignItems(ui.Center).Justify(ui.Start).
		Label(title).Tooltip(title + ": " + label).Cursor(ui.CursorPointer)
	bg, fg := theme.Fade(theme.Selected, 0), theme.TextSoft
	if b.Hovered() || ch.menu == key {
		bg, fg = theme.Selected, theme.Text
	}
	if !ch.compact {
		if key == "model" {
			b.Width(width)
		} else {
			b.MaxWidth(width)
		}
	}
	b.Background(widget.QuickTint(b, "bg", bg)).TextColor(fg).Children(func() {
		if ch.compact {
			ui.Icon(c, svg).FontSize(14).Shrink(0)
		} else {
			widget.Running(c, label, widget.RunStyle{})
		}
		ui.Icon(c, icon.ChevronDown).FontSize(10).Shrink(0)
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
		// Notes under the choices take the room; bare choices don't need it.
		width := float32(160)
		if slices.ContainsFunc(list, func(ch agent.Choice) bool { return ch.Note != "" }) {
			width = 250
		}
		widget.Popup(panel.AttachTo(b, ui.AnchorTopLeft, ui.AnchorBottomLeft).Margin(0, 0, theme.S1, 0).Width(width))
		for _, choice := range list {
			item := ui.ButtonBase(c.Key(choice.ID)).Label(choice.Label).Cursor(ui.CursorPointer).AlignItems(ui.Center).Justify(ui.Start)
			widget.PopupItem(item, item.Hovered()).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, choice.Label+suffix).FontSize(theme.TextUI)
					if choice.Note != "" {
						ui.Text(c, choice.Note).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
					}
				})
				if known && choice.ID == cur.ID {
					ui.Icon(c, icon.Check).FontSize(14).TextColor(theme.Now).Shrink(0)
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

// composerPlaceholder is what the empty composer says.
func composerPlaceholder(o Options) string {
	if o.Placeholder != "" {
		return o.Placeholder
	}
	return "Message " + o.AgentName + ", / for commands, @ for files"
}
