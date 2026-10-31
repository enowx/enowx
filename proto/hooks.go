package proto

import "time"

// Hooks run what the user set (a command, context for the agent, a
// refusal) when something happens in a session, on every agent: in the
// agent through its own hooks where it has them, else as the daemon sees
// it from outside. See docs/hooks.md.
const (
	MethodHooksList    = "hooks.list"    // ProjectDir → HooksInfo
	MethodHooksSave    = "hooks.save"    // []Hook: the host's own hooks, all of them
	MethodHooksApprove = "hooks.approve" // HooksApproval: a project's hooks allowed to run
	MethodHooksTest    = "hooks.test"    // HookTest → HookRun, run now on a sample event
	MethodHooksLog     = "hooks.log"     // → []HookRun, the latest first
	// A project's hooks (its .enowx/hooks.json, allowed as written) and a
	// session's own (kept until it ends).
	MethodHooksSaveProject = "hooks.saveProject" // ProjectHooks (Dir, Hooks)
	MethodHooksSaveSession = "hooks.saveSession" // SessionHooks
	// An agent proposes a change to the hooks; the user decides.
	MethodHooksPropose = "hooks.propose" // HookProposal → HookProposal (with its ID)
	MethodHooksDecide  = "hooks.decide"  // HookDecideParams
	// EventHooksChanged says the hooks, their proposals or pending files
	// changed: a list shown is to be asked for again.
	EventHooksChanged = "hooks.changed"
	// MethodSessionHook is how an agent's own hooks hand the daemon an
	// event (enowx hook --event): it runs the hooks that match and answers
	// what the agent is to do.
	MethodSessionHook = "session.hook" // SessionHookParams → HookDecision
)

// Hook events: when a hook runs.
const (
	HookSessionStart = "session.start" // the agent started, or was continued
	HookSessionEnd   = "session.end"   // its program ended
	HookPromptStart  = "prompt.start"  // a prompt is about to reach the agent
	HookTurnDone     = "turn.done"     // the agent finished its turn
	HookToolBefore   = "tool.before"   // a tool is about to run
	HookToolAfter    = "tool.after"    // a tool ran
	HookPermission   = "permission"    // the agent asks for permission
	HookWaiting      = "waiting"       // the agent waits for the user
	HookFileChanged  = "file.changed"  // files changed in a turn
	HookCommit       = "git.commit"    // a commit was made in the session's folder
	// More of a session's life.
	HookWorking   = "working"           // the agent started working
	HookAnswered  = "permission.answer" // the user allowed (Allowed) or denied what the agent asked
	HookCompact   = "compact"           // the agent is about to compact its context
	HookError     = "error"             // its program ended with an error (Exit)
	HookHibernate = "session.hibernate" // the daemon stopped the idle agent
	HookWake      = "session.wake"      // a hibernated agent was continued
	// Git: before a commit, push or merge (Op), refusable, from the agent
	// or the app's Git panel; and after.
	HookGitBefore = "git.before"
	HookPush      = "git.push"
	HookMerge     = "git.merge"
	HookConflict  = "git.conflict" // files in conflict (Paths)
	// The host.
	HookPort     = "port.open"        // a session started listening on Port
	HookSync     = "sync.done"        // a sync brought changes (Paths)
	HookNotify   = "notify"           // a notification was sent to the apps (Kind, Text)
	HookSchedule = "schedule"         // the hook's own Schedule came round
	HookFallback = "account.fallback" // a session's account hit its limit and switched (Text: from → to)
)

// HookEvents are the events, in the order they're offered.
var HookEvents = []string{HookSessionStart, HookPromptStart, HookWorking, HookToolBefore, HookToolAfter, HookPermission, HookAnswered, HookWaiting, HookCompact, HookTurnDone, HookFileChanged, HookGitBefore, HookCommit, HookPush, HookMerge, HookConflict, HookError, HookHibernate, HookWake, HookSessionEnd, HookPort, HookSync, HookNotify, HookSchedule, HookFallback}

// Hook actions: what a hook does.
const (
	HookActionRun     = "run"     // run Command, the event as JSON on its stdin
	HookActionContext = "context" // add Context to what the agent reads
	HookActionBlock   = "block"   // refuse the action, Reason told to the agent
	HookActionRewrite = "rewrite" // change the command: Find replaced by Replace
	HookActionNotify  = "notify"  // a sound and banner in the apps: Kind, Message
	HookActionWebhook = "webhook" // POST to URL: Message for Discord and Slack, else the event
	HookActionEnowX   = "enowx"   // run enowX's command with Command's arguments
	// Before a tool runs, with the judge (docs/decide.md): Judge has it
	// allow the command, leave it to the user or refuse it; Trim runs it
	// through enowx decide trim, its long output cut to what matters.
	HookActionJudge = "judge"
	HookActionTrim  = "trim"
)

// HookActions are the actions, in the order they're offered.
var HookActions = []string{HookActionRun, HookActionContext, HookActionBlock, HookActionRewrite, HookActionJudge, HookActionTrim, HookActionNotify, HookActionWebhook, HookActionEnowX}

// HookConditions name a hook's conditions, as Not lists them.
var HookConditions = []string{"agents", "projects", "hosts", "branches", "tool", "paths", "text"}

// How an agent gets an event (HookSupport).
const (
	HookInAgent = "agent"   // from its own hooks: it can be refused, given context
	HookOutside = "outside" // seen by the daemon, after the fact: nothing to refuse
)

// Hook is something to do when an event happens.
type Hook struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	On   bool   `json:"on"`
	// Event is when (Hook* events).
	Event string `json:"event"`
	// When it may run: every one set must match. Agents are base IDs
	// ("claude"); Projects folders or their names; Tool a regular
	// expression on the tool's name or command; Paths globs on the files
	// changed or touched; Text a regular expression on the prompt or the
	// command. Empty matches all.
	Agents   []string `json:"agents,omitempty"`
	Projects []string `json:"projects,omitempty"`
	Tool     string   `json:"tool,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	Text     string   `json:"text,omitempty"`
	// Hosts are host names; Branches globs on the session's branch.
	Hosts    []string `json:"hosts,omitempty"`
	Branches []string `json:"branches,omitempty"`
	// Any runs it when any condition set holds, not all; Not names the
	// conditions (HookConditions) that must not hold.
	Any bool     `json:"any,omitempty"`
	Not []string `json:"not,omitempty"`
	// Limits: Once a session, at most Max times a session, Every seconds
	// apart at least, only within Hours ("09-18", "22:00-06:00").
	Once  bool   `json:"once,omitempty"`
	Max   int    `json:"max,omitempty"`
	Every int    `json:"every,omitempty"`
	Hours string `json:"hours,omitempty"`
	// Schedule is when a HookSchedule hook runs, as automations say it
	// ("weekdays 09:00", "hourly").
	Schedule string `json:"schedule,omitempty"`
	// Action is what it does (HookActionRun, HookActionContext,
	// HookActionBlock).
	Action  string `json:"action"`
	Command string `json:"command,omitempty"`
	Context string `json:"context,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// Find (a regular expression) and Replace change a command
	// (HookActionRewrite), $1 for what Find's groups caught.
	Find    string `json:"find,omitempty"`
	Replace string `json:"replace,omitempty"`
	// URL is where a webhook posts; Message what a notification or a
	// webhook says, {agent}, {project}, {event}, {prompt}, {command},
	// {paths}, {branch}… filled in; Kind a notification's sound (done,
	// input, error).
	URL     string `json:"url,omitempty"`
	Message string `json:"message,omitempty"`
	Kind    string `json:"kind,omitempty"`
	// Background runs Command without the agent waiting for it, so it can
	// neither refuse nor add context.
	Background bool `json:"background,omitempty"`
	// Timeout bounds Command, in seconds (0: 30).
	Timeout int `json:"timeout,omitempty"`
	// Project is the folder of a project's hook (its .enowx/hooks.json),
	// Session the session of a session's own, both "" for the host's;
	// filled in by the daemon.
	Project string `json:"project,omitempty"`
	Session string `json:"session,omitempty"`
}

// HookEvent is what happened, handed to a hook's command as JSON.
type HookEvent struct {
	Event   string `json:"event"`
	Session string `json:"session,omitempty"`
	Agent   string `json:"agent,omitempty"`
	// Dir is the session's folder; Project the project's, Branch its
	// branch.
	Dir     string `json:"dir,omitempty"`
	Project string `json:"project,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	// Tool is the tool's name, Command its command line or what it was
	// given, Failed that it failed.
	Tool    string `json:"tool,omitempty"`
	Command string `json:"command,omitempty"`
	Failed  bool   `json:"failed,omitempty"`
	// Paths are the files changed (HookFileChanged) or the tool touched.
	Paths []string `json:"paths,omitempty"`
	// Commit is the commit made (HookCommit), Message its subject.
	Commit  string `json:"commit,omitempty"`
	Message string `json:"message,omitempty"`
	// Op is the Git operation (commit, push, merge) of HookGitBefore;
	// Allowed the answer of HookAnswered; Exit the exit code of HookError;
	// Port the port of HookPort; Kind and Text the notification of
	// HookNotify; Host the host's name.
	Op      string `json:"op,omitempty"`
	Allowed bool   `json:"allowed,omitempty"`
	Exit    int    `json:"exit,omitempty"`
	Port    int    `json:"port,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Text    string `json:"text,omitempty"`
	Host    string `json:"host,omitempty"`
	// Via is how it was seen (HookInAgent, HookOutside).
	Via  string    `json:"via,omitempty"`
	Time time.Time `json:"time"`
}

// HookDecision is what the agent is to do after the hooks ran: go on,
// refuse with Reason, with Context to read; run Command instead of its
// own, approved (Approve, the session skipping permissions) or asked.
type HookDecision struct {
	Block   bool   `json:"block,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Context string `json:"context,omitempty"`
	Command string `json:"command,omitempty"`
	Approve bool   `json:"approve,omitempty"`
}

// SessionHookParams is an event from session ID's agent, with the state
// it means for the session (State*, "" for none).
type SessionHookParams struct {
	ID    string    `json:"id"`
	State string    `json:"state,omitempty"`
	Event HookEvent `json:"event"`
}

// Ways an event comes besides HookInAgent and HookOutside.
const (
	HookInApp     = "app"      // the app did it (a commit from the Git panel)
	HookScheduled = "schedule" // a hook's schedule
)

// HookRun is a hook's run: what it was given and what came of it.
type HookRun struct {
	Hook    string    `json:"hook"`
	Name    string    `json:"name"`
	Event   string    `json:"event"`
	Session string    `json:"session,omitempty"`
	Agent   string    `json:"agent,omitempty"`
	Start   time.Time `json:"start"`
	Ms      int64     `json:"ms"`
	Exit    int       `json:"exit"`
	// Output is the end of what its command printed; Error why it didn't
	// run or failed (a timeout, say); Blocked that it refused the action.
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	Blocked bool   `json:"blocked,omitempty"`
}

// ProjectDir names a project's folder, "" for none.
type ProjectDir struct {
	Dir string `json:"dir,omitempty"`
}

// HooksInfo is the host's hooks and those of projects, the ones waiting
// for the user's approval apart, and how each agent gets each event.
type HooksInfo struct {
	Hooks   []Hook         `json:"hooks"`
	Pending []ProjectHooks `json:"pending,omitempty"`
	// Projects are the host's projects' hook files allowed, Sessions the
	// sessions' own hooks, Proposals the changes agents proposed.
	Projects  []ProjectHooks `json:"projects,omitempty"`
	Sessions  []SessionHooks `json:"sessions,omitempty"`
	Proposals []HookProposal `json:"proposals,omitempty"`
	// Host is the host's name, as the Hosts condition takes it.
	Host string `json:"host,omitempty"`
	// Support is, per agent base ID, how it gets each event (HookInAgent,
	// HookOutside); an event missing it doesn't get.
	Support map[string]map[string]string `json:"support,omitempty"`
}

// ProjectHooks are the hooks of a project's .enowx/hooks.json, as one
// approval: Hash is of the file, so a change asks again.
type ProjectHooks struct {
	Dir   string `json:"dir"`
	Hash  string `json:"hash"`
	Hooks []Hook `json:"hooks"`
}

// HooksApproval allows (or, Allow false, stops allowing) the hooks of
// project Dir's file with Hash.
type HooksApproval struct {
	Dir   string `json:"dir"`
	Hash  string `json:"hash"`
	Allow bool   `json:"allow"`
}

// HookTest runs Hook on Event now, as if it happened.
type HookTest struct {
	Hook  Hook      `json:"hook"`
	Event HookEvent `json:"event"`
}

// SessionHooks are session Session's own hooks, gone when it ends.
type SessionHooks struct {
	Session string `json:"session"`
	Hooks   []Hook `json:"hooks"`
}

// Where a hook is kept.
const (
	HookScopeHost    = "host"
	HookScopeProject = "project"
	HookScopeSession = "session"
)

// HookProposal is a change to the hooks an agent proposed: Hook added, or
// changed when it has an ID, or removed (Remove), in Scope (Dir for a
// project, Session for a session), to run once the user allows it.
type HookProposal struct {
	ID      string `json:"id"`
	Hook    Hook   `json:"hook"`
	Remove  bool   `json:"remove,omitempty"`
	Scope   string `json:"scope"`
	Dir     string `json:"dir,omitempty"`
	Session string `json:"session,omitempty"`
	// By is the session that proposed it, Why what its agent said.
	By   string    `json:"by,omitempty"`
	Why  string    `json:"why,omitempty"`
	Time time.Time `json:"time"`
}

// HookDecideParams allows or rejects proposal ID.
type HookDecideParams struct {
	ID    string `json:"id"`
	Allow bool   `json:"allow"`
}
