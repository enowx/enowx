package ui

import (
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// Samples of New session, pipelines, the window's overlays and GOD of Agents'
// mode, for the layout audit: what a real host, agent or release would say.

const ovLong = "a name long enough that no row of the window's dialogs can hold it on one line at any size"

// ovAgents are the agents a host could list, each kind New session treats apart.
func ovAgents() []proto.AgentInfo {
	return []proto.AgentInfo{
		{ID: "claude", Name: "Claude Code", Command: "claude", Installed: true, CanSkipPermissions: true, SignIn: "claude setup-token"},
		{ID: "codex", Name: "Codex", Command: "codex", Installed: true, CanSkipPermissions: true},
		{ID: "opencode", Name: "OpenCode", Command: "opencode", Install: "curl -fsSL https://opencode.ai/install | bash"},
		{ID: "gemini", Name: "Gemini CLI", Command: "gemini", Homepage: "https://example.com/gemini"},
		{ID: agent.Shell, Name: "Shell"},
		{ID: agent.Custom, Name: "Custom"},
	}
}

// ovForm opens New session in a git project; then fills it once what the
// host answered has arrived.
func ovForm(name string, set func(a *App, f *newSessionForm)) auditPage {
	return auditPage{name: "ov-new-" + name, open: func(a *App) {
		a.page = ""
		a.agents = ovAgents()
		a.openNewSession(a.projects[0])
	}, after: func(h *notesHarness) {
		a := h.app
		a.agents = ovAgents()
		f := &a.newSession
		f.agent = "claude"
		f.project.IsGit, f.project.Branch, f.project.Branches = true, "main", []string{"main", "redesign/ui-build", "feature/" + ovLong}
		f.loaded, f.base, f.existing = true, "main", "main"
		f.branchMode = proto.BranchNew
		if set != nil {
			set(a, f)
		}
	}}
}

// ovClick opens New session as ovForm does, then clicks a control of it.
func ovClick(name, label string, set func(a *App, f *newSessionForm)) auditPage {
	page := ovForm(name, set)
	fill := page.after
	page.after = func(h *notesHarness) {
		fill(h)
		for range 3 {
			h.settle("", func() bool { return true })
		}
		if err := h.tt.Click(label); err != nil {
			h.t.Fatalf("%s: %v", name, err)
		}
		for range 8 {
			h.tt.Advance(60 * time.Millisecond)
			h.tt.Frame()
		}
	}
	return page
}

// ovGOA shows GOD of Agents' mode on the audit's session, with a host's board.
func ovGOA(a *App) {
	a.page = ""
	a.sessions[0].GOA = true
	id := a.sessions[0].ID
	now := time.Now()
	dir := a.projects[0].Path
	a.sessions = append(a.sessions[:1:1],
		proto.SessionInfo{ID: "ov-1", Name: "Host keys to the vault", Agent: "codex", Project: dir, State: proto.StateWaiting},
		proto.SessionInfo{ID: "ov-2", Name: "GOA session tabs", Agent: "claude", Project: dir, State: proto.StateWorking},
		proto.SessionInfo{ID: "ov-3", Name: "Dialog footers", Agent: "claude", Project: dir, Exited: true, ExitCode: 1},
		proto.SessionInfo{ID: "ov-4", Name: ovLong, Agent: "claude", Project: dir},
	)
	a.state.GOA = true
	a.goa.host, a.goa.loading = a.host.ID, false
	a.goa.chatSet = id
	a.goa.board = proto.GOABoard{Session: id, Agent: "claude", Level: proto.GOAEnforced, Tasks: []proto.GOATask{
		{ID: "t1", Title: "Move keys typed for one host to the vault", Session: "ov-1", SessionName: "Host keys to the vault", Agent: "codex", Status: proto.GOATaskWaiting, Urgent: "push", Asks: "run git push origin keys-vault", Model: "gpt-5.5", Effort: "high", Why: "It touches secrets.", Created: now, Project: "enowx", Branch: "keys-vault"},
		{ID: "t2", Title: "Open sessions in tabs of GOA, on any host, and " + ovLong, Session: "ov-2", SessionName: "GOA session tabs", Agent: "claude", Status: proto.GOATaskWorking, Ran: []string{"claude-opus-5-5 · high"}, Created: now, Project: "enowx"},
		{ID: "t3", Title: "Line up the dialogs' footers", Session: "ov-3", SessionName: "Dialog footers", Agent: "claude", Status: proto.GOATaskFailed, Result: "Tests fail in ui", Created: now},
		{ID: "t4", Title: "Check the release notes", Status: proto.GOATaskDone, Result: "Done", Created: now},
	}}
	a.goa.flags = []proto.GOAFlag{
		{ID: "f1", Session: id, Time: now, Project: dir, File: "ui/goa_view.go", Change: "modified", Revertable: true},
		{ID: "f2", Session: id, Time: now, Command: "make lint"},
		{ID: "f3", Session: id, Time: now, Project: dir, File: "docs/goa.md", Change: "added", Reverted: true},
	}
	a.selected = id
	a.openPanel(panelSessions)
}

// ovGOATab shows a worker in a tab of GOA's strip, as far as it got.
func ovGOATab(a *App, host, err string) {
	ovGOA(a)
	key := host + "\x00ov-2"
	a.goa.tabs = []*goaTab{{key: key, host: host, id: "ov-2", agent: "claude", err: err}}
	tabs := a.centerOf(a.goa.board.Session)
	tabs.tabs, tabs.front = []string{centerGOA + key}, centerGOA+key
}

// ovLeaveGOA undoes ovGOA for the next page.
func ovLeaveGOA(a *App) {
	if !a.state.GOA {
		return
	}
	a.state.GOA = false
	a.goa.board = proto.GOABoard{}
	delete(a.center, a.sessions[0].ID)
	a.goa.flags, a.goa.tabs, a.goa.detail, a.goa.start, a.goa.override, a.goa.rules, a.goa.stall = nil, nil, nil, nil, nil, nil, nil
	a.sessions = a.sessions[:1]
	a.sessions[0].GOA = false
	a.state.Panel.Open, a.state.Panel.Tab = false, ""
}

// closeOverlayAudit closes what the pages below opened.
func closeOverlayAudit(a *App) {
	ovLeaveGOA(a)
	a.palette.open, a.quick.open, a.history.open, a.compare.open = false, false, false, false
	a.budget, a.agentLimit, a.renaming, a.confirming, a.hibernating, a.piping = nil, nil, nil, nil, nil, nil
	a.hibernateAsks, a.hostAsks, a.sessionToasts = nil, nil, nil
	a.updates = updater{source: a.updates.source}
	a.pipelines = nil
	a.issues.open = false
	a.addProject.Close()
	a.hostFile.Close()
	a.localPick.Close()
}

func ovUpdate(a *App, state int, err string) {
	a.updates.found = &newVersion{Version: "0.6.0", Date: time.Now().Add(-48 * time.Hour), Notes: "## What's new\n\n- Liquid Glass everywhere\n- Sessions keep their tabs\n\n" + ovLong}
	a.updates.state, a.updates.err = state, err
	a.updates.done, a.updates.total = 40, 100
}

// auditOverlayPages are this area's states: New session, pipelines, the
// pickers, what floats over the window, and GOD of Agents' mode.
func auditOverlayPages() []auditPage {
	withDir := func(a *App) string { return a.projects[0].Path }
	plans := []proto.HibernatePlan{
		{ID: "a", Name: "GOA session tabs", Working: true, Ports: []int{5173}, Display: true, RSS: 488 << 20, CPU: 9, Procs: []proto.HibernateProc{{Name: "vite", RSS: 90 << 20, Left: true}, {Name: "gopls", RSS: 146 << 20}}},
		{ID: "b", Name: ovLong, RSS: 96 << 20},
		{ID: "c", Name: "Dialog footers", Blocked: "it has no conversation to come back on"},
	}
	session := func(a *App) proto.SessionInfo { return a.sessions[0] }
	pipeline := func(a *App) {
		a.page = ""
		a.pipelines = []proto.Pipeline{{ID: "p1", Project: withDir(a), Task: "Add search to the notes", Status: proto.PipelineRunning, Error: "The review stage failed: " + ovLong, Stages: []proto.PipelineStageRun{
			{PipelineStage: proto.PipelineStage{Name: "Plan"}, Status: proto.PipelineDone},
			{PipelineStage: proto.PipelineStage{Name: "Implement"}, Status: proto.PipelineRunning},
			{PipelineStage: proto.PipelineStage{Name: "Review"}, Status: proto.PipelineWaiting},
			{PipelineStage: proto.PipelineStage{Name: "Fix"}, Status: proto.PipelineFailed},
			{PipelineStage: proto.PipelineStage{Name: "Ship it, with a long stage name"}, Status: proto.PipelinePending},
		}}}
	}
	out := []auditPage{
		ovForm("base", func(a *App, f *newSessionForm) {
			f.prompt = "Add a search to the notes"
			f.route = routeForm{on: true, asked: f.prompt, choice: &proto.RouteChoice{Agent: "claude", Model: "claude-opus-5-5", Effort: "high", Why: "this looks hard, and no budget is near its limit."}}
		}),
		ovForm("no-git", func(a *App, f *newSessionForm) { f.project.IsGit, f.branchMode = false, proto.BranchNone }),
		ovForm("install", func(a *App, f *newSessionForm) { f.agent = "opencode" }),
		ovForm("install-none", func(a *App, f *newSessionForm) { f.agent = "gemini" }),
		ovForm("custom", func(a *App, f *newSessionForm) { f.agent = agent.Custom; f.command = "npx some-agent --flag" }),
		ovForm("shell", func(a *App, f *newSessionForm) { f.agent = agent.Shell }),
		ovForm("existing", func(a *App, f *newSessionForm) { f.branchMode = proto.BranchExisting }),
		ovForm("no-branches", func(a *App, f *newSessionForm) { f.project.Branches = nil }),
		ovForm("busy-error", func(a *App, f *newSessionForm) {
			f.busy, f.err = true, "Name the branch: "+ovLong
		}),
		ovForm("route-applied", func(a *App, f *newSessionForm) {
			f.prompt = "Add a search"
			f.route = routeForm{on: true, asked: f.prompt, applied: true, choice: &proto.RouteChoice{Agent: "claude", Model: "claude-opus-5-5", Effort: "high", Why: "this looks hard."}}
		}),
		ovForm("route-asking", func(a *App, f *newSessionForm) { f.prompt = "Add"; f.route = routeForm{on: true, asking: true} }),
		ovForm("setup-display", func(a *App, f *newSessionForm) {
			f.project.Setup, f.project.Copy = "make setup", []string{".env", "local.toml"}
			a.daemonInfo.Display = true
		}),
		ovForm("display-install", func(a *App, f *newSessionForm) {
			a.host.ID, a.daemonInfo.Display, a.daemonInfo.DisplayNote, a.daemonInfo.DisplayInstall = "ov-server", false, "needs Xpra", "sudo apt-get install -y xpra"
		}),
		ovClick("session-picker", "Session: New session", func(a *App, f *newSessionForm) {
			f.convs = []proto.Conversation{{ID: "c1", Agent: "claude", Title: "Move the keys to the vault", Updated: time.Now()}}
		}),
		ovClick("agent-picker", "Claude Code", nil),
		ovClick("branch-picker", "main", nil),
		ovClick("issue-picker", "From a GitHub issue…", nil),
		ovForm("continue-past", func(a *App, f *newSessionForm) {
			f.cont = &sessionChoice{key: "c:1", agent: "claude", name: "Move the keys to the vault", where: "project folder", status: "past conversation", conv: &proto.Conversation{ID: "c1", Agent: "claude"}}
			f.convs = []proto.Conversation{{ID: "c1", Agent: "claude", Title: "Move the keys to the vault", Updated: time.Now()}}
		}),
		ovForm("continue-hibernated", func(a *App, f *newSessionForm) {
			s := a.sessions[0]
			s.Hibernated = true
			f.cont = &sessionChoice{key: "s:1", agent: "claude", name: "a shell", where: "project folder", status: "hibernated", session: &s}
		}),
		ovForm("continue-running", func(a *App, f *newSessionForm) {
			s := a.sessions[0]
			f.cont = &sessionChoice{key: "s:1", agent: "shell", name: "a shell", where: "project folder", status: "working", session: &s}
		}),

		{name: "ov-compare", open: func(a *App) {
			a.page = ""
			a.sessions[0].Group = "g"
			a.compare = compareDialog{open: true, group: "g", err: "Couldn't discard them all: " + ovLong, diffs: map[string]proto.Diff{a.sessions[0].ID: {Files: []proto.DiffFile{{Added: 12, Removed: 3}}}}}
		}},
		{name: "ov-compare-gone", open: func(a *App) {
			a.page = ""
			a.compare = compareDialog{open: true, group: "gone", diffs: map[string]proto.Diff{}}
		}},
		{name: "ov-pipeline", open: func(a *App) {
			a.page = ""
			a.agents = ovAgents()
			a.openPipeline(a.projects[0])
		}, after: func(h *notesHarness) {
			f := h.app.piping
			f.task, f.branch, f.err = "Add search to the notes", branchFor("Add search to the notes"), "The branch exists: "+ovLong
			f.branches, f.base, f.editing = []string{"main"}, "main", 1
		}},
		{name: "ov-pipeline-full", open: func(a *App) {
			a.page = ""
			a.agents = ovAgents()
			a.openPipeline(a.projects[0])
		}, after: func(h *notesHarness) {
			f := h.app.piping
			for len(f.stages) < 8 {
				f.stages = append(f.stages, proto.PipelineStage{Name: "Stage", Agent: "claude"})
			}
		}},
		{name: "ov-pipeline-card", open: pipeline},

		{name: "ov-folder", open: func(a *App) { a.page = ""; a.openFolder() }},
		{name: "ov-file", open: func(a *App) {
			a.page = ""
			a.pickHostFile("Choose the app", "The program to run, on this host.", func(string) {})
		}},
		{name: "ov-local", open: func(a *App) {
			a.page = ""
			a.localPick.Open("Add to antares", "Copied to the session's folder, wherever it runs.", func([]string) {})
		}},

		{name: "ov-palette", open: func(a *App) { a.page = ""; a.openPalette() }},
		{name: "ov-palette-none", open: func(a *App) {
			a.page = ""
			a.openPalette()
			a.palette.query = "nothing like this"
		}},
		{name: "ov-quick", open: func(a *App) {
			a.page = ""
			a.quick = quickOpen{open: true, session: session(a).ID, asked: "", paths: []string{"ui/palette.go", "ui/goa_view.go", "docs/session-tab-strip-and-center/" + ovLong + ".md", "README.md"}}
		}},
		{name: "ov-quick-none", open: func(a *App) {
			a.page = ""
			a.quick = quickOpen{open: true, session: session(a).ID, query: "zzz", asked: "zzz"}
		}},
		{name: "ov-quick-error", open: func(a *App) {
			a.page = ""
			a.quick = quickOpen{open: true, session: session(a).ID, err: "The host can't be read: " + ovLong}
		}},
		{name: "ov-history", open: func(a *App) {
			a.page = ""
			now := time.Now()
			a.history = historyDialog{open: true, project: a.projects[0], list: []proto.Conversation{
				{ID: "1", Agent: "claude", Title: "Move the keys to the vault", Branch: "keys-vault", Messages: 42, Model: "claude-opus-5-5", Updated: now},
				{ID: "2", Agent: "codex", Title: ovLong + " " + ovLong, Dir: "/p/enowx", Messages: 1, Updated: now},
				{ID: "3", Agent: "claude", Dir: "/p/enowx", Messages: 0, Updated: now},
			}}
		}},
		{name: "ov-history-loading", open: func(a *App) {
			a.page = ""
			a.history = historyDialog{open: true, project: a.projects[0], loading: true}
		}},
		{name: "ov-history-empty", open: func(a *App) { a.page = ""; a.history = historyDialog{open: true, project: a.projects[0]} }},
		{name: "ov-history-error", open: func(a *App) {
			a.page = ""
			a.history = historyDialog{open: true, project: a.projects[0], err: "Couldn't read them: " + ovLong}
		}},
		{name: "ov-rename", open: func(a *App) { a.page = ""; a.askRename(session(a)) }},
		{name: "ov-budget", open: func(a *App) { a.page = ""; a.askBudget(session(a)) }},
		{name: "ov-agent-limit-reading", open: func(a *App) { a.page = ""; a.agentLimit = &agentsForm{open: true} }},
		{name: "ov-agent-limit", open: func(a *App) { a.page = ""; a.agentLimit = &agentsForm{open: true, loaded: true, pick: 2} }},
		{name: "ov-hibernate", open: func(a *App) { a.page = ""; a.hibernating = &hibernateForm{plans: plans} }},
		{name: "ov-hibernate-asked", open: func(a *App) {
			a.page = ""
			a.hibernateAsks = []*hibernateForm{{plans: plans, asker: "GOA session tabs", until: time.Now().Add(time.Hour), reply: func(bool) {}}}
		}},
		{name: "ov-host-ask", open: func(a *App) {
			a.page = ""
			a.hostAsks = []*hostAsk{{key: 1, about: "The agent of “GOA session tabs” on this Mac asks to push the branch on old-vps. It can't be undone.", until: time.Now().Add(time.Hour), run: func() {}, refuse: func() {}}}
		}},
		{name: "ov-confirm", open: func(a *App) {
			a.page = ""
			a.confirm("Delete “"+ovLong+"”?", "The agent stops. Its worktree and branch stay on disk.", "Delete", func() {})
		}},
		{name: "ov-update", open: func(a *App) { a.page = ""; ovUpdate(a, updateAvailable, ""); a.updates.dialog = true }},
		{name: "ov-update-downloading", open: func(a *App) { a.page = ""; ovUpdate(a, updateDownloading, ""); a.updates.dialog = true }},
		{name: "ov-update-ready", open: func(a *App) { a.page = ""; ovUpdate(a, updateReady, ""); a.updates.dialog = true }},
		{name: "ov-update-failed", open: func(a *App) {
			a.page = ""
			ovUpdate(a, updateAvailable, "the signature doesn't match")
			a.updates.dialog = true
		}},
		{name: "ov-corner-available", open: func(a *App) { a.page = ""; ovUpdate(a, updateAvailable, "") }},
		{name: "ov-corner-downloading", open: func(a *App) { a.page = ""; ovUpdate(a, updateDownloading, "") }},
		{name: "ov-corner-ready", open: func(a *App) { a.page = ""; ovUpdate(a, updateReady, "") }},
		{name: "ov-corner-failed", open: func(a *App) { a.page = ""; ovUpdate(a, updateAvailable, "the signature doesn't match: "+ovLong) }},
		{name: "ov-toasts", open: func(a *App) {
			a.page = ""
			a.toastSession(proto.NotifyInput, a.host.Name, "a", "Host keys to the vault", "")
			a.toastSession(proto.NotifyError, a.host.Name, "b", ovLong, "It ended with exit code 1")
			a.toastSession("done", "old-vps", "c", "Built-in judge", ovLong+" "+ovLong)
			a.toastAction("n", "Look at the canvas", "An agent asks you to look.", func() {})
		}},

		{name: "ov-goa", open: ovGOA},
		{name: "ov-goa-tasks", open: func(a *App) { ovGOA(a); a.state.Panel.Tab = panelTasks }},
		{name: "ov-goa-tool", open: func(a *App) { ovGOA(a); a.openPanel(panelFiles) }},
		{name: "ov-goa-center", open: func(a *App) {
			ovGOA(a)
			a.openPanel(panelFiles)
			a.openCenter(a.sessions[0], fileTabPrefix+"README.md")
		}},
		{name: "ov-goa-tab-connecting", open: func(a *App) { ovGOATab(a, a.host.Name, "") }},
		{name: "ov-goa-tab-error", open: func(a *App) { ovGOATab(a, "old-vps", "The host can't be reached: "+ovLong) }},
		{name: "ov-goa-starting", open: func(a *App) { ovGOA(a); a.goa.board.Session = "gone" }},
		{name: "ov-goa-start", open: func(a *App) {
			a.page = ""
			a.goa.start = &goaStartForm{open: true, agent: "claude"}
			a.goa.err, a.goa.agents = "", []proto.GOAAgent{
				{Agent: "claude", Name: "Claude Code", Installed: true, Level: proto.GOAEnforced, Why: "Enforced: its tools are held to delegating"},
				{Agent: "opencode", Name: "OpenCode", Installed: true, Level: proto.GOAWatched, Why: "Watched: only told, changes are flagged"},
			}
		}},
		{name: "ov-goa-start-reading", open: func(a *App) {
			a.page = ""
			a.goa.start = &goaStartForm{open: true, agent: "claude"}
			a.goa.err, a.goa.agents = "", nil
		}},
		{name: "ov-goa-start-old", open: func(a *App) {
			a.page = ""
			a.goa.start = &goaStartForm{open: true, agent: "claude"}
			a.goa.err = "This host's enowX is too old for GOD of Agents: update it."
		}},
		{name: "ov-goa-detail", open: func(a *App) {
			ovGOA(a)
			a.goa.detail = &goaDetail{open: true, host: a.host.Name, session: a.sessions[1]}
		}},
		{name: "ov-goa-detail-none", open: func(a *App) {
			ovGOA(a)
			a.goa.detail = &goaDetail{open: true, host: "old-vps", session: a.sessions[4]}
		}},
		{name: "ov-goa-override", open: func(a *App) { ovGOA(a); a.openGOAOverride(a.goa.board.Tasks[0]) }},
		{name: "ov-goa-rules", open: func(a *App) {
			ovGOA(a)
			a.goa.rules = &goaRulesForm{open: true, loaded: true, text: "- Delegate every task that edits code.", err: "That looks like it holds a secret"}
		}},
		{name: "ov-goa-rules-loading", open: func(a *App) { ovGOA(a); a.goa.rules = &goaRulesForm{open: true} }},
		{name: "ov-goa-stall", open: func(a *App) {
			ovGOA(a)
			a.goa.stall = &goaStallForm{open: true, loaded: true, blank: "2m", idle: "10m", stuck: "20m"}
		}},
	}
	return out
}
