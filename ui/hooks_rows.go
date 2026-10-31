package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// hookEventNames name the events as the section shows them.
var hookEventNames = map[string]string{
	proto.HookSessionStart: "Session starts",
	proto.HookPromptStart:  "Before a prompt",
	proto.HookWorking:      "Agent starts working",
	proto.HookToolBefore:   "Before a tool runs",
	proto.HookToolAfter:    "After a tool runs",
	proto.HookPermission:   "Agent asks permission",
	proto.HookAnswered:     "Permission answered",
	proto.HookWaiting:      "Agent waits for you",
	proto.HookCompact:      "Context compacted",
	proto.HookTurnDone:     "Turn done",
	proto.HookFileChanged:  "Files changed",
	proto.HookGitBefore:    "Before a commit, push or merge",
	proto.HookCommit:       "Commit made",
	proto.HookPush:         "Pushed",
	proto.HookMerge:        "Merged",
	proto.HookConflict:     "Conflict",
	proto.HookError:        "Agent ends with an error",
	proto.HookHibernate:    "Session hibernates",
	proto.HookWake:         "Session wakes",
	proto.HookSessionEnd:   "Session ends",
	proto.HookPort:         "Port opened",
	proto.HookSync:         "Sync brought changes",
	proto.HookNotify:       "Notification sent",
	proto.HookSchedule:     "On a schedule",
	proto.HookFallback:     "Account switched at its limit",
}

// hookActionNames name the actions.
var hookActionNames = map[string]string{
	proto.HookActionRun:     "Run a command",
	proto.HookActionContext: "Add context",
	proto.HookActionBlock:   "Refuse",
	proto.HookActionRewrite: "Change the command",
	proto.HookActionJudge:   "Ask the judge",
	proto.HookActionTrim:    "Trim its long output",
	proto.HookActionNotify:  "Notify",
	proto.HookActionWebhook: "Call a webhook",
	proto.HookActionEnowX:   "Run an enowX command",
}

// hookSummary is what hook h does, when, in a line.
func hookSummary(h proto.Hook) string {
	what := ""
	switch h.Action {
	case proto.HookActionRun:
		what = "runs " + firstLine(strings.TrimSpace(h.Command))
	case proto.HookActionEnowX:
		what = "runs enowx " + firstLine(strings.TrimSpace(h.Command))
	case proto.HookActionContext:
		what = "adds context"
	case proto.HookActionBlock:
		what = "refuses"
	case proto.HookActionRewrite:
		what = "changes /" + h.Find + "/ to " + h.Replace
	case proto.HookActionJudge:
		what = "asks the judge"
	case proto.HookActionTrim:
		what = "trims long output"
	case proto.HookActionNotify:
		what = "notifies"
	case proto.HookActionWebhook:
		what = "calls a webhook"
	}
	if h.Background {
		what += " in the background"
	}
	var when []string
	add := func(name, text string) {
		if text == "" {
			return
		}
		for _, n := range h.Not {
			if n == name {
				text = "not " + text
			}
		}
		when = append(when, text)
	}
	add("agents", strings.Join(h.Agents, ", "))
	add("projects", strings.Join(h.Projects, ", "))
	add("hosts", strings.Join(h.Hosts, ", "))
	add("branches", strings.Join(h.Branches, ", "))
	if h.Tool != "" {
		add("tool", "tool /"+h.Tool+"/")
	}
	add("paths", strings.Join(h.Paths, ", "))
	if h.Text != "" {
		add("text", "text /"+h.Text+"/")
	}
	event := hookEventNames[h.Event]
	if h.Event == proto.HookSchedule {
		event = h.Schedule
	}
	out := event + " → " + what
	if len(when) > 0 {
		join := " · "
		if h.Any {
			join = " or "
		}
		out += " · only " + strings.Join(when, join)
	}
	var limits []string
	switch {
	case h.Once:
		limits = append(limits, "once a session")
	case h.Max > 0:
		limits = append(limits, fmt.Sprintf("at most %d a session", h.Max))
	}
	if h.Every > 0 {
		limits = append(limits, fmt.Sprintf("%ds apart", h.Every))
	}
	if h.Hours != "" {
		limits = append(limits, h.Hours)
	}
	if len(limits) > 0 {
		out += " · " + strings.Join(limits, ", ")
	}
	return out
}

func hookActionIcon(action string) *ui.SVG {
	switch action {
	case proto.HookActionBlock:
		return icon.Lock
	case proto.HookActionContext:
		return icon.Chat
	case proto.HookActionRewrite:
		return icon.Pencil
	case proto.HookActionNotify:
		return icon.VolumeUp
	case proto.HookActionWebhook:
		return icon.Globe
	case proto.HookActionEnowX:
		return icon.Wand
	}
	return icon.Terminal
}

func (a *App) hookRow(c *ui.Context, st *hooksState, place hookPlace, h proto.Hook) {
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, hookActionIcon(h.Action)).FontSize(15).TextColor(theme.TextSoft)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				name := h.Name
				if name == "" {
					name = hookEventNames[h.Event]
				}
				ui.Text(c, name).FontSize(12.5).SingleLine()
				ui.Text(c, hookSummary(h)).FontSize(11).TextColor(theme.Muted).SingleLine()
			})
			on := h.On
			if widget.Switch(c, &on, "On") {
				h.On = on
				a.saveHook(st, place, h, false)
			}
			widget.MoreMenu(c, &a.menu, "hook:"+h.ID, func(done func()) {
				if widget.MenuItem(c, icon.Pencil, "Edit…", false) {
					st.form.start(h, place, false)
					done()
				}
				if widget.MenuItem(c, icon.Trash, "Remove", true) {
					a.confirm("Remove the hook “"+hookSummary(h)+"”?", "It stops running on every agent of "+a.host.Name+".", "Remove", func() { a.saveHook(st, place, h, true) })
					done()
				}
			})
		})
	})
}

// hookProposal is a change an agent proposed, for the user to allow.
func (a *App) hookProposal(c *ui.Context, st *hooksState, p proto.HookProposal) {
	what := "An agent proposes a hook"
	switch {
	case p.Remove:
		what = "An agent proposes removing a hook"
	case p.Hook.ID != "":
		what = "An agent proposes changing a hook"
	}
	if p.By != "" {
		what += " (" + a.hookSessionName(p.By) + ")"
	}
	where := "on " + a.host.Name
	switch p.Scope {
	case proto.HookScopeProject:
		where = "in project " + filepath.Base(p.Dir)
	case proto.HookScopeSession:
		where = "in session " + a.hookSessionName(p.Session)
	}
	body := hookSummary(p.Hook) + ", " + where + "."
	if p.Why != "" {
		body += " " + p.Why
	}
	widget.Notice(c, icon.Wand, theme.Accent, what, body, func() {
		if detail := hookDetail(p.Hook); detail != "" {
			ui.Text(c, detail).Font(theme.MonoFont).FontSize(11.5).TextColor(theme.TextSoft).MaxLines(6)
		}
		ui.Row(c).Gap(8).Children(func() {
			if widget.Button(c, widget.Secondary, nil, "Reject", "").Disabled(st.busy).Clicked() {
				a.decideHook(st, p.ID, false)
			}
			if widget.PrimaryButton(c, icon.Check, "Allow", "").Disabled(st.busy).Clicked() {
				a.decideHook(st, p.ID, true)
			}
		})
	})
}

// hookDetail is what hook h runs or says, to read before allowing it.
func hookDetail(h proto.Hook) string {
	switch h.Action {
	case proto.HookActionRun, proto.HookActionEnowX:
		return h.Command
	case proto.HookActionContext:
		return h.Context
	case proto.HookActionBlock:
		return h.Reason
	case proto.HookActionWebhook:
		return h.URL
	case proto.HookActionNotify:
		return h.Message
	}
	return ""
}

func (a *App) decideHook(st *hooksState, id string, allow bool) {
	cl := a.client
	if cl == nil {
		return
	}
	st.busy = true
	go func() {
		err := cl.DecideHook(id, allow)
		a.onMain(func() {
			st.busy = false
			if err != nil {
				a.status = client.Explain(err)
			}
			a.loadHooks()
		})
	}()
}

// pendingHooks is a project's hook file waiting for the user.
func (a *App) pendingHooks(c *ui.Context, st *hooksState, ph proto.ProjectHooks) {
	n := len(ph.Hooks)
	what := fmt.Sprintf("%s has %d hook%s of its own", filepath.Base(ph.Dir), n, map[bool]string{true: "", false: "s"}[n == 1])
	widget.Notice(c, icon.Alert, theme.Warning, what, "They don't run until you allow them, and ask again whenever the project's .enowx/hooks.json changes.", func() {
		if widget.Button(c, widget.Secondary, nil, "Review…", "").Clicked() {
			st.review, st.seeing = ph, true
		}
	})
}

// hookReviewSheet shows a project's hooks before they may run.
func (a *App) hookReviewSheet(c *ui.Context, st *hooksState) {
	if !st.seeing {
		return
	}
	ph := st.review
	widget.FormSheet(c, &st.seeing, 560, "Hooks of "+filepath.Base(ph.Dir),
		"From "+filepath.Join(ph.Dir, ".enowx", "hooks.json")+". Allowed, they run like yours on "+a.host.Name+", commands included.", func() {
			for _, h := range ph.Hooks {
				ui.Column(c.Key(h.ID)).FillWidth().Gap(3).Padding(8, 10).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
					ui.Text(c, hookSummary(h)).FontSize(12)
					if d := hookDetail(h); d != "" {
						ui.Text(c, d).Font(theme.MonoFont).FontSize(11.5).TextColor(theme.TextSoft).MaxLines(6)
					}
				})
			}
		}, func() {
			if widget.PrimaryButton(c, icon.Check, "Allow them", "").Disabled(st.busy).Clicked() {
				a.approveHooks(st, ph)
			}
		})
}

func (a *App) approveHooks(st *hooksState, ph proto.ProjectHooks) {
	cl := a.client
	if cl == nil {
		return
	}
	st.busy = true
	go func() {
		err := cl.ApproveHooks(proto.HooksApproval{Dir: ph.Dir, Hash: ph.Hash, Allow: true})
		a.onMain(func() {
			st.busy = false
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			st.seeing = false
			a.loadHooks()
		})
	}()
}

// hookLogList is the latest runs of the host's hooks.
func (a *App) hookLogList(c *ui.Context, st *hooksState) {
	if len(st.log) == 0 {
		ui.Text(c, "No hook has run since "+a.host.Name+"'s daemon started.").FontSize(12).TextColor(theme.Muted)
		return
	}
	for i, r := range st.log {
		ui.Column(c.Key(i)).FillWidth().Gap(3).Padding(8, 10).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				col, label := theme.Done, "ok"
				switch {
				case r.Error != "":
					col, label = theme.Error, r.Error
				case r.Blocked:
					col, label = theme.Warning, "refused"
				case r.Exit != 0:
					col, label = theme.Error, fmt.Sprintf("exit %d", r.Exit)
				}
				ui.Box(c).Size(7, 7).Radius(4).Background(col)
				name := r.Name
				if name == "" {
					name = hookEventNames[r.Event]
				}
				ui.Text(c, name).FontSize(12).SingleLine().Shrink(1)
				ui.Text(c, hookEventNames[r.Event]).FontSize(11).TextColor(theme.Muted).SingleLine()
				ui.Spacer(c)
				meta := label + " · " + fmt.Sprintf("%d ms", r.Ms)
				if r.Agent != "" {
					meta = a.agentName(r.Agent) + " · " + meta
				}
				ui.Text(c, meta+" · "+r.Start.Local().Format("15:04:05")).Font(theme.MonoFont).FontSize(10.5).TextColor(theme.Muted).SingleLine()
			})
			if out := strings.TrimSpace(r.Output); out != "" {
				ui.Text(c, out).Font(theme.MonoFont).FontSize(11).TextColor(theme.TextSoft).MaxLines(4)
			}
		})
	}
}
