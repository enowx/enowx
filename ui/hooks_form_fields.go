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
	widget.Field(c, "Name (optional)", func() { widget.Input(c, &h.Name, "Format").FillWidth().AutoFocus() })
	when := func() {
		widget.Field(c, "When", func() { hookSelect(c, "When", &h.Event, proto.HookEvents, hookEventNames) })
	}
	if f.adding {
		widget.FieldCols(c.Key("kept-when"), func() {
			widget.Field(c, "Kept", func() { a.hookPlacePicker(c, f) })
		}, when)
	} else {
		when()
	}
	widget.CardNote(c, a.hookReach(st, h.Event), 0)
	if h.Event == proto.HookSchedule {
		widget.Field(c, "Schedule", func() {
			widget.Input(c, &h.Schedule, "weekdays 09:00").FillWidth().Font(theme.MonoFont)
			widget.CardNote(c, "hourly (or hourly :15), daily 09:00, weekdays 18:30, weekly mon 08:00, in "+a.host.Name+"'s time.", 0)
		})
	}
	widget.Field(c, "Do", func() {
		hookSelect(c, "Do", &h.Action, proto.HookActions, hookActionNames)
		if note := hookActionNote(h.Event, h.Action); note != "" && !h.Background {
			ui.Text(c, note).FontSize(theme.TextLabel).TextColor(theme.Warning).FillWidth().MinWidth(0)
		}
	})
	a.hookActionFields(c, h)
	widget.Field(c, "Only when", func() {
		ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
			hookCondition(c, h, "agents", "Agents", f.lists["agents"], "claude, codex")
			hookCondition(c, h, "projects", "Projects", f.lists["projects"], "enowx, ~/code/site")
			hookCondition(c, h, "branches", "Branches", f.lists["branches"], "main, release/*")
			hookCondition(c, h, "hosts", "Hosts", f.lists["hosts"], st.info.Host)
			hookCondition(c, h, "tool", "Tool or command", &h.Tool, `^Bash$|git push.*main`)
			hookCondition(c, h, "paths", "Files", f.lists["paths"], "*.go, src/**")
			hookCondition(c, h, "text", "Prompt or command text", &h.Text, `deploy|release`)
		})
		switchRow(c, &h.Any, "Any of them", "Run when one condition holds, not only when all do.")
		widget.CardNote(c, "Empty means any. Tool and text are regular expressions, files and branches globs. Is not turns a condition round.", 0)
	})
	widget.Field(c, "How often", func() {
		switchRow(c, &h.Once, "Once a session", "")
		widget.FieldCols(c.Key("how-often"), func() {
			hookNumber(c, "At most, a session", f.numbers["max"], "times")
		}, func() {
			hookNumber(c, "Apart at least", f.numbers["every"], "seconds")
		}, func() {
			widget.Field(c, "Only within", func() { widget.Input(c, &h.Hours, "09-18").FillWidth() })
		})
	})
}

// hookActionFields are what action h.Action needs.
func (a *App) hookActionFields(c *ui.Context, h *proto.Hook) {
	area := func(v *string, label, placeholder string, mono bool) {
		hookArea(c, v, label, placeholder, mono).MinHeight(60).MaxHeight(180)
	}
	note := func(s string) { widget.CardNote(c, s, 0) }
	switch h.Action {
	case proto.HookActionRun:
		widget.Field(c, "Command", func() {
			area(&h.Command, "Command", `gofmt -w $(jq -r '.paths[]')`, true)
			note(`Runs in the session's folder, the event as JSON on stdin. Exit 2 refuses (stderr the reason); before a tool, printing {"command": "…"} changes it; otherwise what it prints is given to the agent before a prompt or at a session's start.`)
		})
	case proto.HookActionEnowX:
		widget.Field(c, "enowX command", func() {
			area(&h.Command, "enowX command", `notes write "Release plan" --append "- [x] tests pass"`, true)
			note("Arguments to enowX's command line, as in a session: notes, canvas, project autopush --now, spawn…")
		})
	case proto.HookActionContext:
		widget.Field(c, "Context", func() {
			area(&h.Context, "Context", "Answer in Indonesian. Run make lint before saying it's done.", false)
		})
	case proto.HookActionBlock:
		widget.Field(c, "Reason", func() {
			widget.Input(c, &h.Reason, "Not on this project: ask me first").FillWidth()
			note("Told to the agent as why.")
		})
	case proto.HookActionRewrite:
		widget.Field(c, "Find and replace", func() {
			ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
				widget.Input(c, &h.Find, `^npm install`).Grow(1).Basis(0).MinWidth(0).Font(theme.MonoFont)
				ui.Icon(c, icon.ArrowRight).FontSize(12).TextColor(theme.TextSoft)
				widget.Input(c, &h.Replace, "pnpm install").Grow(1).Basis(0).MinWidth(0).Font(theme.MonoFont)
			})
			note("A regular expression in the command, and what replaces it ($1 for what a group caught). Claude Code asks you about the new command unless the session skips permissions; Codex gets it as the command to run instead.")
		})
	case proto.HookActionJudge:
		note("enowX's judge (Settings → Decisions) allows the command without asking you, leaves it to you, or refuses it, as sure as you set it to be. With no judge, or Approving commands off, nothing changes.")
	case proto.HookActionTrim:
		note("The command runs through enowX: a long output (over 150 lines) reaches the agent cut by the judge to what matters, the whole kept in a file it can read, its exit code unchanged. Set Tool or command to the commands with long output (tests, builds, installs).")
	case proto.HookActionNotify:
		widget.Field(c, "Sound", func() {
			hookSelect(c, "Sound", &h.Kind, []string{"done", "input", "error"}, map[string]string{"": "Done sound", "done": "Done sound", "input": "Input sound", "error": "Error sound"})
		})
		hookMessage(c, h, "{agent}, {project}, {branch}, {event}, {prompt}, {command}, {paths}, {commit} are filled in.")
	case proto.HookActionWebhook:
		widget.Field(c, "Address", func() { widget.Input(c, &h.URL, "https://discord.com/api/webhooks/…").FillWidth() })
		hookMessage(c, h, "Discord and Slack webhooks get the message; any other address the event as JSON with it.")
	}
	if h.Action == proto.HookActionRun || h.Action == proto.HookActionEnowX {
		switchRow(c, &h.Background, "In the background", "The agent doesn't wait for it, so it can neither refuse nor add context.")
	}
}

func hookMessage(c *ui.Context, h *proto.Hook, about string) {
	widget.Field(c, "Message", func() {
		widget.Input(c, &h.Message, "{agent} finished in {project}").FillWidth()
		widget.CardNote(c, about, 0)
	})
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
	ui.Row(c.Key(name)).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Row(c).Width(150).Shrink(0).Children(func() { widget.Running(c, label, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft}) })
		not := slices.Contains(h.Not, name)
		which := "is"
		if not {
			which = "is not"
		}
		if widget.GhostButton(c, which).Width(72).Tooltip("Turn this condition round").Clicked() {
			if not {
				h.Not = slices.DeleteFunc(h.Not, func(n string) bool { return n == name })
			} else {
				h.Not = append(h.Not, name)
			}
		}
		widget.Input(c, value, placeholder).Grow(1).Basis(0).MinWidth(0).Font(theme.MonoFont)
	})
}

func hookNumber(c *ui.Context, label string, value *string, unit string) {
	widget.Field(c.Key(label), label, func() {
		ui.Row(c).FillWidth().Gap(theme.S1).Children(func() {
			widget.Input(c, value, "").Grow(1).Basis(0).MinWidth(0)
			ui.Text(c, unit).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0)
		})
	})
}

// hookSelect picks value among options, each named by names.
func hookSelect(c *ui.Context, label string, value *string, options []string, names map[string]string) {
	sel := ui.SelectBase(c.Key("select:"+label), value)
	widget.Trigger(sel.Trigger, sel.Open()).Label(label).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, names[*value], widget.RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(theme.S1, 0, 0, 0).Width(320))
		widget.Scroll(c).FillWidth().MaxHeight(360).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, o := range options {
					item := sel.Item(o)
					widget.PopupItem(item, item.Highlighted()).Children(func() {
						ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, names[o], widget.RunStyle{}) })
					})
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
