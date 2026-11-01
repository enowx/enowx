package chatview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// slashShown is how many commands the slash menu lists at once, and
// slashNameWidth how wide their names run: what they do needs room too.
const (
	slashShown     = 8
	slashNameWidth = 260
)

// slashMatches are the agent's commands for the "/word" being typed:
// those starting with it, then those containing it, then those whose
// description does. None unless the input is a slash and a word.
func (ch *Chat) slashMatches() []proto.AgentCommand { return ch.slashMatchesFor(ch.input) }

// slashMatchesFor is slashMatches for the input in.
func (ch *Chat) slashMatchesFor(in string) []proto.AgentCommand {
	if !strings.HasPrefix(in, "/") || strings.ContainsAny(in, " \n") || in == ch.slashClosed {
		return nil
	}
	q := strings.ToLower(in[1:])
	var first, then, last []proto.AgentCommand
	for _, cmd := range ch.commands {
		name := strings.ToLower(cmd.Name)
		switch {
		case strings.HasPrefix(name, q):
			first = append(first, cmd)
		case strings.Contains(name, q):
			then = append(then, cmd)
		case q != "" && strings.Contains(strings.ToLower(cmd.Description), q):
			last = append(last, cmd)
		}
	}
	out := append(append(first, then...), last...)
	if len(out) > slashShown {
		out = out[:slashShown]
	}
	return out
}

// complete puts cmd in the input, ready for its arguments.
func (ch *Chat) complete(cmd proto.AgentCommand) {
	ch.input, ch.slashPick, ch.focus = "/"+cmd.Name+" ", 0, true
}

// slashMenu lists the commands for the slash being typed, over the
// composer; arrows move, Tab or Enter completes, Escape closes it.
func (ch *Chat) slashMenu(c *ui.Context) {
	if !strings.HasPrefix(ch.input, "/") || strings.ContainsAny(ch.input, " \n") || ch.input == ch.slashClosed {
		return
	}
	matches := ch.slashMatches()
	widget.Float(ui.Column(c.Key("slash")).FillWidth().Padding(theme.S1).Radius(theme.RadiusCard).
		Background(theme.Popover)).Transition(widget.OverlayMotion).Children(func() {
		switch {
		case !ch.commandsRead:
			ui.Row(c).Gap(theme.S2).Padding(theme.S2, theme.S3).Children(func() {
				widget.Spinner(c, 12)
				ui.Text(c, "Reading the agent's commands…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			})
			return
		case len(matches) == 0:
			ui.Text(c, "No command matches "+ch.input).FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3)
			return
		}
		for i, cmd := range matches {
			row := ui.ButtonBase(c.Key(cmd.Name)).Label("/" + cmd.Name).Cursor(ui.CursorPointer)
			widget.PopupItem(row, i == ch.slashPick || row.Hovered()).Children(func() {
				svg := icon.Slash
				if cmd.Kind == "skill" {
					svg = icon.Brain
				}
				ui.Icon(c, svg).FontSize(14).TextColor(theme.TextSoft).Shrink(0)
				ui.Row(c).MaxWidth(slashNameWidth).MinWidth(0).Shrink(0).Children(func() {
					widget.Running(c, "/"+cmd.Name, widget.RunStyle{Color: theme.Text})
				})
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, cmd.Description, widget.RunStyle{Color: theme.TextSoft})
				})
				if cmd.Kind != "" && cmd.Kind != "command" {
					widget.Tag(c, cmd.Kind, theme.Muted)
				}
			})
			if row.Clicked() {
				ch.complete(cmd)
			}
		}
	})
}
