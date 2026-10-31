package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The form's fields: when, what it does, only when, how often.

func (a *App) hookFields(c *ui.Context, st *hooksState, f *hookForm) {
	h := &f.hook
	widget.Field(c, "Name", func() { widget.Input(c, &h.Name, "No rm -rf").FillWidth().AutoFocus() })
	widget.Field(c, "When", func() {
		hookSelect(c, "When", &h.Event, proto.HookEvents, hookEventNames)
		ui.Text(c, a.hookReach(st, h.Event)).FontSize(11).TextColor(theme.Muted)
		if h.Event == proto.HookSchedule {
			widget.Input(c, &h.Schedule, "weekdays 09:00").FillWidth().Font(theme.MonoFont)
			ui.Text(c, "hourly (or hourly :15), daily 09:00, weekdays 18:30, weekly mon 08:00, in "+a.host.Name+"'s time.").FontSize(11).TextColor(theme.Muted)
		}
	})
	widget.Field(c, "Do", func() {
		hookSelect(c, "Do", &h.Action, proto.HookActions, hookActionNames)
		a.hookActionFields(c, h)
		if note := hookActionNote(h.Event, h.Action); note != "" && !h.Background {
			ui.Text(c, note).FontSize(11).TextColor(theme.Warning)
		}
	})
	widget.Field(c, "Only when", func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			hookCondition(c, h, "agents", "Agents", f.lists["agents"], "claude, codex")
			hookCondition(c, h, "projects", "Projects", f.lists["projects"], "enowx, ~/code/site")
			hookCondition(c, h, "branches", "Branches", f.lists["branches"], "main, release/*")
			hookCondition(c, h, "hosts", "Hosts", f.lists["hosts"], st.info.Host)
			hookCondition(c, h, "tool", "Tool or command", &h.Tool, `^Bash$|git push.*main`)
			hookCondition(c, h, "paths", "Files", f.lists["paths"], "*.go, src/**")
			hookCondition(c, h, "text", "Prompt or command text", &h.Text, `deploy|release`)
		})
		switchRow(c, &h.Any, "Any of them", "Run when one condition holds, not only when all do.")
		ui.Text(c, "Empty means any. Tool and text are regular expressions, files and branches globs. Is not turns a condition round.").FontSize(11).TextColor(theme.Muted)
	})
	widget.Field(c, "How often", func() {
		switchRow(c, &h.Once, "Once a session", "")
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			hookNumber(c, "At most, a session", f.numbers["max"], "times")
			hookNumber(c, "Apart at least", f.numbers["every"], "seconds")
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "Only within").FontSize(11.5).TextColor(theme.TextSoft)
				widget.Input(c, &h.Hours, "09-18").Width(140).Font(theme.MonoFont)
			})
		})
	})
}

// hookActionFields are what action h.Action needs.
func (a *App) hookActionFields(c *ui.Context, h *proto.Hook) {
	area := func(v *string, label, placeholder string, mono bool) {
		t := ui.TextAreaBase(c, v).FillWidth().MinHeight(60).MaxHeight(180).Padding(9, 11).FontSize(12).
			Label(label).Radius(theme.RadiusControl).Background(theme.Sunken).Border(1, theme.Border).Placeholder(placeholder)
		if mono {
			t.Font(theme.MonoFont)
		}
	}
	muted := func(s string) { ui.Text(c, s).FontSize(11).TextColor(theme.Muted) }
	switch h.Action {
	case proto.HookActionRun:
		area(&h.Command, "Command", `gofmt -w $(jq -r '.paths[]')`, true)
		muted(`Runs in the session's folder, the event as JSON on stdin. Exit 2 refuses (stderr the reason); before a tool, printing {"command": "…"} changes it; otherwise what it prints is given to the agent before a prompt or at a session's start.`)
	case proto.HookActionEnowX:
		area(&h.Command, "enowX command", `notes write "Release plan" --append "- [x] tests pass"`, true)
		muted("Arguments to enowX's command line, as in a session: notes, canvas, project autopush --now, spawn…")
	case proto.HookActionContext:
		area(&h.Context, "Context", "Answer in Indonesian. Run make lint before saying it's done.", false)
	case proto.HookActionBlock:
		widget.Input(c, &h.Reason, "Not on this project: ask me first").FillWidth()
		muted("Told to the agent as why.")
	case proto.HookActionRewrite:
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			widget.Input(c, &h.Find, `^npm install`).Grow(1).Font(theme.MonoFont)
			ui.Icon(c, icon.ArrowRight).FontSize(12).TextColor(theme.Muted)
			widget.Input(c, &h.Replace, "pnpm install").Grow(1).Font(theme.MonoFont)
		})
		muted("A regular expression in the command, and what replaces it ($1 for what a group caught). Claude Code asks you about the new command unless the session skips permissions; Codex gets it as the command to run instead.")
	case proto.HookActionJudge:
		muted("enowX's judge (Settings → Decisions) allows the command without asking you, leaves it to you, or refuses it, as sure as you set it to be. With no judge, or Approving commands off, nothing changes.")
	case proto.HookActionTrim:
		muted("The command runs through enowX: a long output (over 150 lines) reaches the agent cut by the judge to what matters, the whole kept in a file it can read, its exit code unchanged. Set Tool or command to the commands with long output (tests, builds, installs).")
	case proto.HookActionNotify:
		hookSelect(c, "Sound", &h.Kind, []string{"done", "input", "error"}, map[string]string{"": "Done sound", "done": "Done sound", "input": "Input sound", "error": "Error sound"})
		widget.Input(c, &h.Message, "{agent} finished in {project}").FillWidth()
		muted("{agent}, {project}, {branch}, {event}, {prompt}, {command}, {paths}, {commit} are filled in.")
	case proto.HookActionWebhook:
		widget.Input(c, &h.URL, "https://discord.com/api/webhooks/…").FillWidth().Font(theme.MonoFont)
		widget.Input(c, &h.Message, "{agent} finished in {project}").FillWidth()
		muted("Discord and Slack webhooks get the message; any other address the event as JSON with it.")
	}
	if h.Action == proto.HookActionRun || h.Action == proto.HookActionEnowX {
		switchRow(c, &h.Background, "In the background", "The agent doesn't wait for it, so it can neither refuse nor add context.")
	}
}

// hookActionNote warns of an action an event can't take.
func hookActionNote(event, action string) string {
	refusable := event == proto.HookToolBefore || event == proto.HookPromptStart || event == proto.HookGitBefore
	answers := refusable || event == proto.HookSessionStart
	switch {
	case action == proto.HookActionBlock && !refusable:
		return "Only a tool about to run, a prompt, or a commit, push or merge can be refused: this one will just be logged."
	case action == proto.HookActionContext && !answers:
		return "Context reaches the agent only before a prompt or at a session's start."
	case action == proto.HookActionRewrite && event != proto.HookToolBefore:
		return "Only a tool about to run has a command to change."
	case (action == proto.HookActionJudge || action == proto.HookActionTrim) && event != proto.HookToolBefore:
		return "The judge and trimming act before a tool runs."
	}
	return ""
}

// hookCondition is a condition's row: its value, and whether it is or
// is not to hold.
func hookCondition(c *ui.Context, h *proto.Hook, name, label string, value *string, placeholder string) {
	ui.Row(c.Key(name)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(theme.TextSoft).Width(150).Shrink(0)
		not := slices.Contains(h.Not, name)
		which := "is"
		if not {
			which = "is not"
		}
		if widget.GhostButton(c, which).Width(64).Tooltip("Turn this condition round").Clicked() {
			if not {
				h.Not = slices.DeleteFunc(h.Not, func(n string) bool { return n == name })
			} else {
				h.Not = append(h.Not, name)
			}
		}
		widget.Input(c, value, placeholder).Grow(1).Font(theme.MonoFont)
	})
}

func hookNumber(c *ui.Context, label string, value *string, unit string) {
	ui.Column(c.Key(label)).Gap(4).Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(theme.TextSoft)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			widget.Input(c, value, "").Width(80)
			ui.Text(c, unit).FontSize(11.5).TextColor(theme.Muted)
		})
	})
}

// hookSelect picks value among options, each named by names.
func hookSelect(c *ui.Context, label string, value *string, options []string, names map[string]string) {
	sel := ui.SelectBase(c.Key("select:"+label), value)
	widget.Trigger(sel.Trigger, sel.Open()).FillWidth().Label(label).Children(func() {
		ui.Text(c, names[*value]).FontSize(12.5).SingleLine()
		ui.Spacer(c)
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.Muted)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(6, 0, 0, 0).Width(320))
		ui.Scroll(c).FillWidth().MaxHeight(360).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, o := range options {
					item := sel.Item(o)
					widget.PopupItem(item, item.Highlighted()).Children(func() { ui.Text(c, names[o]).FontSize(12.5) })
				}
			})
		})
	})
}

// hookReach says which agents get event, and how.
func (a *App) hookReach(st *hooksState, event string) string {
	switch event {
	case proto.HookPort, proto.HookSync, proto.HookNotify, proto.HookSchedule:
		return "Seen by " + a.host.Name + " itself, whichever agent."
	}
	var in, out []string
	for id, events := range st.info.Support {
		switch events[event] {
		case proto.HookInAgent:
			in = append(in, a.agentName(id))
		case proto.HookOutside:
			out = append(out, id)
		}
	}
	slices.Sort(in)
	var parts []string
	if len(in) > 0 {
		parts = append(parts, "In the agent: "+strings.Join(in, ", "))
	}
	switch {
	case len(out) > 0 && len(in) > 0:
		parts = append(parts, "the others seen from outside, after the fact")
	case len(out) > 0:
		parts = append(parts, "Every agent, seen from outside, after the fact")
	}
	if len(parts) == 0 {
		return "Only agents with hooks of their own report this, and none do here."
	}
	return strings.Join(parts, "; ") + "."
}
