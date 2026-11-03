package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
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
	proto.HookStalled:      "A GOD of Agents task isn't moving",
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
		what = "Run: " + firstLine(strings.TrimSpace(h.Command))
	case proto.HookActionEnowX:
		what = "Run: enowx " + firstLine(strings.TrimSpace(h.Command))
	case proto.HookActionContext:
		what = "Add context"
	case proto.HookActionBlock:
		what = "Refuse"
	case proto.HookActionRewrite:
		what = "Change /" + h.Find + "/ to " + h.Replace
	case proto.HookActionJudge:
		what = "Ask the judge"
	case proto.HookActionTrim:
		what = "Trim long output"
	case proto.HookActionNotify:
		what = "Notify"
	case proto.HookActionWebhook:
		what = "Call a webhook"
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
	out := event + " · " + what
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

// hookTitle is what names hook h on its card, and whether that's a command.
func hookTitle(h proto.Hook) (string, bool) {
	switch {
	case h.Name != "":
		return h.Name, false
	case (h.Action == proto.HookActionRun || h.Action == proto.HookActionEnowX) && strings.TrimSpace(h.Command) != "":
		return firstLine(strings.TrimSpace(h.Command)), true
	}
	return hookEventNames[h.Event], false
}

func (a *App) hookRow(c *ui.Context, st *hooksState, g hookGroup, h proto.Hook) {
	place := g.place
	widget.GridCard(c).Children(func() {
		ui.Row(c.Key("title")).FillWidth().Shrink(0).MinHeight(24).Gap(theme.S2).Children(func() {
			title, mono := hookTitle(h)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Running(c, title, widget.RunStyle{Weight: 600, Mono: mono})
			})
			on := h.On
			if widget.Switch(c, &on, "On") {
				h.On = on
				a.saveHook(st, place, h, false)
			}
		})
		ui.Row(c.Key("chips")).FillWidth().Shrink(0).Gap(theme.S1).Children(func() {
			widget.RunningChip(c, hookEventNames[h.Event])
			widget.RunningChip(c, g.chip)
		})
		widget.CardNote(c, hookSummary(h), 2)
		widget.CardFoot(c, func() {
			ui.Row(c.Key("runs")).Grow(1).Basis(0).MinWidth(0).Children(func() {
				if r, ok := st.tested[h.ID]; ok {
					col, what := hookTestWords(r)
					widget.StatusLine(c, col, false, what)
					return
				}
				widget.Running(c, hookRunsWords(st, h), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if widget.GhostButton(c.Key("test"), "Test").Disabled(st.testing[h.ID]).Clicked() {
				a.testHookCard(st, h)
			}
			widget.MoreMenu(c, &a.menu, "hook:"+h.ID, func(done func()) {
				if widget.MenuItem(c, icon.Pencil, "Edit…", false) {
					st.form.start(h, place, false)
					done()
				}
				if widget.MenuItem(c, icon.Trash, "Remove…", true) {
					a.confirm("Remove this hook?", hookSummary(h)+". It stops running on every agent of "+a.host.Name+".", "Remove", func() { a.saveHook(st, place, h, true) })
					done()
				}
			})
		})
	})
}

// hookRunsWords says how often the log has hook h.
func hookRunsWords(st *hooksState, h proto.Hook) string {
	n := 0
	for _, r := range st.log {
		if r.Hook == h.ID {
			n++
		}
	}
	if n == 1 {
		return "1 run in the log"
	}
	return strconv.Itoa(n) + " runs in the log"
}

// testHookCard runs hook h on the sample event the form's Test uses; its card
// says how it went.
func (a *App) testHookCard(st *hooksState, h proto.Hook) {
	cl := a.client
	if cl == nil {
		return
	}
	h.On = true
	if st.testing == nil {
		st.testing, st.tested = map[string]bool{}, map[string]proto.HookRun{}
	}
	st.testing[h.ID] = true
	delete(st.tested, h.ID)
	ev := a.hookSampleEvent(h)
	go func() {
		r, err := cl.TestHook(proto.HookTest{Hook: h, Event: ev})
		a.onMain(func() {
			delete(st.testing, h.ID)
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			st.tested[h.ID] = r
		})
	}()
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
	var more func()
	if detail := hookDetail(p.Hook); detail != "" {
		more = func() {
			ui.Text(c, detail).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).FillWidth().MinWidth(0).MaxLines(6)
		}
	}
	widget.BannerCard(c, theme.Accent, what, body, more, func() {
		if widget.GhostButton(c, "Reject").Disabled(st.busy).Clicked() {
			a.decideHook(st, p.ID, false)
		}
		if widget.PrimaryButton(c, nil, "Allow", "").Disabled(st.busy).Clicked() {
			a.decideHook(st, p.ID, true)
		}
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
	what := filepath.Base(ph.Dir) + " has a hook waiting to be allowed."
	if n != 1 {
		what = fmt.Sprintf("%s has %d hooks waiting to be allowed.", filepath.Base(ph.Dir), n)
	}
	widget.BannerCard(c, theme.Warning, what, "A project's .enowx/hooks.json runs once you allow it, and asks again whenever it changes.", nil, func() {
		if widget.GhostButton(c, "Review").Clicked() {
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
	widget.FormSheet(c, &st.seeing, 480, "Hooks of "+filepath.Base(ph.Dir),
		"Waiting to be allowed. Allowed, they run like yours on "+a.host.Name+", commands included.", func() {
			widget.Running(c.Key("path"), filepath.Join(ph.Dir, ".enowx", "hooks.json"), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft, Mono: true})
			widget.CardLines(c, func() {
				for _, h := range ph.Hooks {
					ui.Column(c.Key(h.ID)).FillWidth().Shrink(0).Padding(theme.S2, 0).Gap(theme.S1).BorderWidth(theme.Rule, 0, 0, 0).BorderColor(theme.Border).Children(func() {
						ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
							title, mono := hookTitle(h)
							ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, title, widget.RunStyle{Mono: mono}) })
							ui.Row(c).MaxWidth(200).Shrink(0).Children(func() { widget.RunningChip(c, hookEventNames[h.Event]) })
						})
						widget.CardNote(c, hookSummary(h), 0)
						if d := hookDetail(h); d != "" {
							ui.Text(c, d).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).FillWidth().MinWidth(0).MaxLines(6)
						}
					})
				}
			})
		}, func() {
			if widget.PrimaryButton(c, nil, "Allow them", "").Disabled(st.busy).Clicked() {
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

// hookRunStatus is how a run went: its dot's color and its words.
func hookRunStatus(r proto.HookRun) (ui.Color, string) {
	switch {
	case r.Error != "":
		return theme.Error, r.Error
	case r.Blocked:
		return theme.Warning, "Refused"
	case r.Exit != 0:
		return theme.Error, fmt.Sprintf("Exit %d", r.Exit)
	}
	return theme.Done, "Exit 0"
}

// hookRunName names the hook a run was of.
func hookRunName(r proto.HookRun) string {
	if r.Name != "" {
		return r.Name
	}
	return hookEventNames[r.Event]
}

// hookLogList is the latest runs of the host's hooks.
func (a *App) hookLogList(c *ui.Context, st *hooksState) {
	if len(st.log) == 0 {
		widget.EmptyState(c, icon.History, "No hook has run since "+a.host.Name+"'s daemon started.", "", nil)
		return
	}
	widget.LogGrid(c, len(st.log), func() {
		for i, r := range st.log {
			col, status := hookRunStatus(r)
			meta := []string{r.Start.Local().Format("15:04:05")}
			if r.Session != "" {
				meta = append(meta, a.hookSessionName(r.Session))
			}
			if r.Agent != "" {
				meta = append(meta, a.agentName(r.Agent))
			}
			meta = append(meta, hookEventNames[r.Event], msWords(r.Ms))
			out := strings.TrimSpace(r.Output)
			widget.LogCard(c.Key(i), hookRunName(r), false, col, status, strings.Join(meta, " · "), nil, func() {
				if out != "" {
					ui.Text(c, out).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).FillWidth().MinWidth(0).MaxLines(2)
				}
			})
		}
	})
}
