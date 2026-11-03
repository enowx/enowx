package ui

import (
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// Settings' sections filled from sample data for the layout audit: a page read
// from the host would only be drawn loading in the frames the audit gives it.

// auditInstalled lists a few agents as installed: the audit's host has none.
func auditInstalled(a *App) {
	a.agents = agent.List()
	for i := range a.agents {
		switch a.agents[i].ID {
		case "claude", "codex", "opencode":
			a.agents[i].Installed = true
		}
	}
}

func auditAgents(a *App) *accountsState {
	a.page, a.settingsSection = pageSettings, sectionAgents
	auditInstalled(a)
	now := time.Now()
	work := proto.Account{ID: "work", Agent: "claude", Name: "Work account"}
	glm := proto.Account{ID: "glm", Agent: "claude", Name: "A model API with a rather long name that has to run", Kind: proto.ConfigKindAPI, Color: "#2B7A5B",
		API: &proto.ConfigAPI{BaseURL: "https://gateway.example.com/v1/and/a/long/path", Model: "glm-5", KeyCred: "glm"}}
	a.accts = accountsState{host: a.host.ID, asked: true, loaded: true, agent: "claude", only: map[string]*string{},
		list: []proto.Account{work, glm},
		info: proto.AccountsInfo{
			Accounts: []proto.AccountStatus{
				{Own: true, Account: proto.Account{Agent: "claude"}, State: proto.AccountLimit, Until: now.Add(2 * time.Hour),
					Credits: &proto.CreditAccount{Agent: "claude", Plan: "Claude Max", Windows: []proto.CreditWindow{{Label: "5 hours", Used: 100, ResetsAt: now.Add(2 * time.Hour)}}}},
				{Account: work, State: proto.AccountOK,
					Credits: &proto.CreditAccount{Agent: "claude", Plan: "Claude Team", Windows: []proto.CreditWindow{{Label: "5 hours", Used: 34, ResetsAt: now.Add(4 * time.Hour)}}}},
				{Account: glm, State: proto.AccountOK},
			},
			Prefs: map[string]proto.AccountPrefs{"claude": {Order: []string{"", "Work account", glm.Name}, Fallback: "next", Return: true}},
		},
		log: []proto.AccountLogEntry{
			{Time: now, Session: "s1", Agent: "claude", Account: "Work account", Event: "fallback", Reason: "Default hit its limit, and this reason runs on long enough to have to run in its row"},
			{Time: now.Add(-time.Minute), Agent: "claude", Event: "limit", Reason: "Resets 16:20"},
			{Time: now.Add(-time.Hour), Session: "s1", Agent: "claude", Event: "start"},
		},
	}
	return &a.accts
}

func auditAutomations(a *App) *automationsState {
	a.page, a.settingsSection = pageSettings, sectionAutomations
	now := time.Now()
	dir := a.projects[0].Path
	a.autos = automationsState{host: a.host.ID, asked: true, loaded: true, list: []proto.Automation{
		{ID: "a1", Name: "Nightly dependency check", Project: dir, Agent: "claude", Prompt: "Check dependencies and open a pull request when one needs updating.", Schedule: "daily 02:00", Branch: true, Enabled: true, NextRun: now.Add(8 * time.Hour), LastRun: now.Add(-16 * time.Hour)},
		{ID: "a2", Name: "Weekly changelog, with a name long enough that it has to run in its card", Project: dir, Agent: "codex", Prompt: "Summarize the week's merged pull requests into CHANGELOG.md, and go on for long enough to be cut at the lines this card keeps for it, whatever the width of the window it is in.", Schedule: "weekly mon 09:00", Enabled: true, NextRun: now.Add(48 * time.Hour), LastRun: now.Add(-120 * time.Hour), LastError: "the agent stopped at its limit, and this runs long enough to wrap to two lines inside the card"},
		{ID: "a3", Name: "Off", Project: dir, Agent: "claude", Prompt: "Off for now.", Schedule: "every 6h"},
	}}
	return &a.autos
}

func auditHooks(a *App) *hooksState {
	a.page, a.settingsSection = pageSettings, sectionHooks
	now := time.Now()
	dir := a.projects[0].Path
	lint := proto.Hook{ID: "h1", Name: "make lint", On: true, Event: proto.HookFileChanged, Paths: []string{"*.go"}, Action: proto.HookActionRun, Command: "make lint"}
	notify := proto.Hook{ID: "h2", Name: "./notify.sh, with a name long enough that it has to run in its card", On: true, Event: proto.HookFileChanged, Action: proto.HookActionRun, Command: "./notify.sh"}
	a.hooks = hooksState{host: a.host.ID, asked: true, loaded: true,
		info: proto.HooksInfo{Hooks: []proto.Hook{notify},
			Projects:  []proto.ProjectHooks{{Dir: dir, Hooks: []proto.Hook{lint}}},
			Pending:   []proto.ProjectHooks{{Dir: dir, Hash: "x", Hooks: []proto.Hook{lint}}},
			Proposals: []proto.HookProposal{{ID: "p1", Hook: lint, Scope: "host", By: "s1", Why: "keep the Go files clean, for a reason long enough to wrap inside its banner whatever the window's width", Time: now}},
		},
		log: []proto.HookRun{
			{Hook: "h1", Name: "make lint", Event: proto.HookFileChanged, Session: "GOA session tabs", Start: now, Ms: 2800},
			{Hook: "h2", Name: "./notify.sh", Event: proto.HookFileChanged, Session: "Dialog footers", Start: now.Add(-time.Hour), Ms: 40, Exit: 1, Error: "exit 1"},
			{Hook: "h1", Name: "make lint", Event: proto.HookFileChanged, Session: "Dialog footers", Start: now.Add(-2 * time.Hour), Ms: 3100, Blocked: true},
		},
	}
	return &a.hooks
}

func auditDecide(a *App) *decideState {
	a.page, a.settingsSection = pageSettings, sectionDecide
	now := time.Now()
	a.decideSet = decideState{host: a.host.ID, asked: true, loaded: true, testing: map[string]bool{"j2": true}, calls: map[string]string{},
		set: proto.DecideSettings{Policy: proto.DecideSameProvider, Uses: map[string]proto.DecideUse{"state": {On: true, MinConfidence: 0.75, DailyCalls: 200}}},
		judges: []proto.Judge{
			{ID: "j1", Kind: "api", Provider: "anthropic", Model: "haiku", Label: "Haiku through Claude Code", Tested: now, OK: true, Ms: 820, Cost: 0.001},
			{ID: "j2", Kind: "api", Provider: "voyage", Model: "x", Label: "A second judge, testing, with a label long enough to run in its row"},
			{ID: "j3", Kind: "api", Provider: "other", Model: "y", Label: "Failed", Own: true, HasKey: true, Tested: now, Error: "the server said 401: check the key, and this goes on long enough to wrap to its two lines"},
		},
		log: []proto.Decision{
			{ID: "d1", Use: "state", Time: now, Verdict: "done", Confidence: 0.92, Why: "It says it is finished, and asks nothing.", Judge: "j1", Ms: 800, Acted: "marked done"},
			{ID: "d2", Use: "approve", Time: now.Add(-time.Hour), Verdict: "refuse", Confidence: 0.6, Why: "A push to main, with a reason long enough to have to wrap or run in its row, whatever the window's width happens to be.", Judge: "j1", Wrong: true},
			{ID: "d3", Use: "trim", Time: now.Add(-2 * time.Hour), Error: "no judge answered"},
		},
	}
	return &a.decideSet
}

// auditIndex opens the session's Index tool on status st.
func auditIndex(a *App, st proto.RAGStatus) {
	a.page, a.state.Panel.Open = "", false
	if len(a.sessions) > 0 {
		a.selectSession(a.sessions[0])
	}
	a.openPanel(panelIndex)
	a.ragPanel = ragPanelState{key: a.host.ID + "\x00" + a.sessions[0].ID, session: a.sessions[0].ID, dir: a.projects[0].Path, loaded: true, st: st}
}

func auditSettingsPages() []auditPage {
	return []auditPage{
		{name: "settings-new-sessions-agents", open: func(a *App) {
			auditInstalled(a)
			a.page, a.settingsSection = pageSettings, sectionSessions
		}, fill: true},
		{name: "session-panel-index-indexed", open: func(a *App) {
			auditIndex(a, proto.RAGStatus{Ready: true, Embedder: "voyage api.voyageai.com", Chunks: 4212, Files: 318, Tokens: 186000, Indexed: time.Now(), Settings: proto.RAGProject{On: true, Consent: "voyage api.voyageai.com"}})
		}},
		{name: "session-panel-index-off", open: func(a *App) {
			auditIndex(a, proto.RAGStatus{Ready: true, Embedder: "voyage api.voyageai.com"})
		}},
		{name: "session-panel-index-none", open: func(a *App) { auditIndex(a, proto.RAGStatus{}) }},
		{name: "settings-agents-filled", open: func(a *App) { auditAgents(a) }, fill: true},
		{name: "settings-agents-only", open: func(a *App) {
			projects := "enowx"
			auditAgents(a).only["claude\x00Work account"] = &projects
		}, fill: true},
		{name: "session-panel-index-memories", open: func(a *App) {
			auditIndex(a, proto.RAGStatus{Ready: true, Embedder: "voyage api.voyageai.com", Chunks: 4212, Files: 318, Indexed: time.Now(), Settings: proto.RAGProject{On: true, Consent: "voyage api.voyageai.com"}})
			a.ragPanel.mems = []proto.RAGMemory{{ID: "m1", Text: "We keep the index in SQLite."}, {ID: "m2", Text: "The next one, being changed."}}
			a.ragPanel.editing, a.ragPanel.editText = "m2", "The next one, being changed."
			a.ragPanel.query, a.ragPanel.searched = "where", "where"
			a.ragPanel.hits = []proto.RAGHit{{File: "core/rag/store.go", Start: 12, Snippet: "func Search finds the parts nearest a question, and this line runs on long enough to be clipped at its lines."}}
		}},
		{name: "settings-agents-history", open: func(a *App) { auditAgents(a).tab = 1 }, fill: true},
		{name: "settings-automations-filled", open: func(a *App) { auditAutomations(a) }, fill: true},
		{name: "settings-hooks-filled", open: func(a *App) { auditHooks(a) }, fill: true},
		{name: "settings-hooks-filled-log", open: func(a *App) { auditHooks(a).tab = 1 }, fill: true},
		{name: "settings-decisions-filled", open: func(a *App) { auditDecide(a) }, fill: true},
		{name: "settings-decisions-filled-uses", open: func(a *App) { auditDecide(a).tab = 1 }, fill: true},
		{name: "settings-decisions-filled-log", open: func(a *App) { auditDecide(a).tab = 2 }, fill: true},
		{name: "dialog-settings-automation", open: func(a *App) {
			auditAutomations(a).editing = &proto.Automation{Project: a.projects[0].Path, Agent: "claude", Name: "Weekday triage"}
		}},
		{name: "dialog-settings-hook", open: func(a *App) {
			st := auditHooks(a)
			st.form.start(proto.Hook{Name: "Refuse a push", On: true, Event: proto.HookFileChanged, Action: proto.HookActionRun, Command: "exit 2"}, hookPlace{}, true)
		}},
		{name: "dialog-settings-config", open: func(a *App) {
			auditAgents(a).edit = a.newConfigForm("claude", proto.Account{})
		}},
		{name: "dialog-settings-judge", open: func(a *App) {
			auditDecide(a).adding = judgeForm{open: true, kind: proto.JudgeDecision}
		}},
		{name: "dialog-settings-hook-review", open: func(a *App) {
			st := auditHooks(a)
			st.review, st.seeing = st.info.Pending[0], true
		}},
		{name: "dialog-settings-fallback", open: func(a *App) {
			auditAgents(a)
			a.acctAsks = []*proto.AccountFallback{{Session: "s1", Agent: "claude", From: "Default", To: "Work account", ToID: "work", Reason: "usage", Until: time.Now().Add(time.Hour), Ask: true}}
		}},
		{name: "dialog-settings-decision", open: func(a *App) {
			auditDecide(a)
			a.decisions = []*proto.Decision{{ID: "d9", Use: "loop", Session: "s1", Verdict: "stuck", Confidence: 0.9, Why: "The same failure, over and over, with no fix tried.", Acted: "paused"}}
		}},
		{name: "dialog-settings-judge-key", open: func(a *App) {
			st := auditDecide(a)
			st.keying = &st.judges[2]
		}},
	}
}
