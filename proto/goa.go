package proto

import "time"

// GOD of Agents (GOA, docs/goa.md): one session, in a folder of its own,
// that controls every session on every host and only delegates: it hands
// tasks to sessions, each with a model and effort the judge picked, keeps
// a board of them, answers its workers' permission prompts with the judge
// (urgent ones go to the user) and keeps a RAG collection of its own.
const (
	// MethodGOAAgents lists the agents installed and how far GOA is held
	// to delegating on each: → []GOAAgent.
	MethodGOAAgents = "goa.agents"
	// MethodGOABoard is the board of this host's GOA: → GOABoard.
	MethodGOABoard = "goa.board"
	// MethodGOADelegate routes a task (decide route), gives it to a
	// session (a new one, or one named) on any host and puts it on the
	// board: GOADelegateParams → GOATask.
	MethodGOADelegate = "goa.delegate"
	// MethodGOATask sets a task's status or result, as GOA sees it:
	// GOATaskParams → GOATask.
	MethodGOATask = "goa.task"
	// MethodGOAOverride is the user's model and effort for a task, over
	// the judge's: switched in its session, kept on the board.
	// GOAOverrideParams → GOATask.
	MethodGOAOverride = "goa.override"
	// MethodGOAWorker tells GOA's host of a worker on another host, which
	// only the app sees: GOAWorkerParams.
	MethodGOAWorker = "goa.worker"
	// MethodGOAFlags lists what a GOA only watched did itself: files it
	// changed in projects, programs it ran. → []GOAFlag.
	MethodGOAFlags = "goa.flags"
	// MethodGOARevert puts a flagged file back as it was: GOARevertParams
	// → GOAFlag. The user's, from the app.
	MethodGOARevert = "goa.revert"
	// MethodGOARemember keeps a note in GOA's own RAG collection:
	// GOARememberParams → RAGMemory.
	MethodGOARemember = "goa.remember"
	// MethodGOARecall searches GOA's collection, and projects' read-only:
	// GOARecallParams → []RAGHit.
	MethodGOARecall = "goa.recall"
	// MethodGOARules are the user's rules for GOA: → GOARules.
	MethodGOARules = "goa.rules"
	// MethodGOARulesSave keeps them (synced, every host's GOA reads
	// them): GOARules → GOARules.
	MethodGOARulesSave = "goa.rules.save"
	// MethodGOAStall are how long a task on this host's board may not move
	// before GOA is told: → GOAStall.
	MethodGOAStall = "goa.stall"
	// MethodGOAStallSave keeps them, on this host: GOAStall → GOAStall.
	MethodGOAStallSave = "goa.stall.save"

	// EventGOABoard is GOA's board changed: GOABoard.
	EventGOABoard = "goa.changed"
	// EventGOAFlag is something a watched GOA did itself: GOAFlag.
	EventGOAFlag = "goa.flagged"
)

// EnvGOA is set in GOA's session and its enowX MCP server.
const EnvGOA = "ENOWX_GOA"

// How far GOA is held to delegating on an agent, the strictest first.
const (
	// GOAEnforced: the agent's own hooks refuse every tool but enowX's.
	GOAEnforced = "enforced"
	// GOALimited: the agent starts with enowX's tools only.
	GOALimited = "limited"
	// GOAWatched: the agent is only asked; what it does itself is
	// flagged to the user, files revertable.
	GOAWatched = "watched"
)

// GOAAgent is an agent as GOA's: how far it is held to delegating.
type GOAAgent struct {
	Agent     string `json:"agent"`
	Name      string `json:"name"`
	Level     string `json:"level"` // GOAEnforced, GOALimited, GOAWatched
	Why       string `json:"why"`
	Installed bool   `json:"installed"`
}

// Statuses of a task on GOA's board.
const (
	GOATaskWorking = "working"
	GOATaskWaiting = "waiting" // its session asks something
	GOATaskQueued  = "queued"  // its session is busy: delivered at its turn's end
	GOATaskStalled = "stalled" // not moving: GOA was told how
	// GOATaskTurnEnded: its agent finished a turn, which may be mid-work.
	// GOA reads it and settles the task, or sends more.
	GOATaskTurnEnded = "turn-ended"
	GOATaskDone      = "done"
	GOATaskFailed    = "failed"
	GOATaskStopped   = "stopped"
)

// GOATask is a task on GOA's board: the session it went to, how it was
// routed, where it is.
type GOATask struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	// Host is the worker's host as the app names it, "" for GOA's own.
	Host    string `json:"host,omitempty"`
	Session string `json:"session,omitempty"`
	Agent   string `json:"agent,omitempty"`
	// SessionName, Dir and Branch are its session's as delegated, kept for
	// when it has gone; Project below is its project, "" for none.
	SessionName string `json:"sessionName,omitempty"`
	Dir         string `json:"dir,omitempty"`
	Branch      string `json:"branch,omitempty"`
	// Ran are the models it really ran on ("model · effort"), oldest first,
	// as its agent said them; Model and Effort are only what was asked.
	Ran []string `json:"ran,omitempty"`
	// Model, Effort and Config are what the prompt runs with ("" the
	// agent's own); Why says why, Difficulty how hard the judge found it.
	Model      string `json:"model,omitempty"`
	Effort     string `json:"effort,omitempty"`
	Config     string `json:"config,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
	Why        string `json:"why,omitempty"`
	// Override: the user chose the model and effort, not the judge.
	Override bool `json:"override,omitempty"`
	// Reused: no session was named, and the project had this one for it.
	// Woken: its agent had stopped, and was brought back for the prompt.
	Reused bool   `json:"reused,omitempty"`
	Woken  bool   `json:"woken,omitempty"`
	Status string `json:"status"`
	// Project is the project its session works in, when it has one.
	Project string `json:"project,omitempty"`
	// Result is how it ended, once settled. Said is what its agent last
	// said as a turn ended, no outcome until GOA says so.
	Result string `json:"result,omitempty"`
	Said   string `json:"said,omitempty"`
	// Asks is what its session waits on; Urgent, why the user answers it
	// rather than GOA (it can't be undone, money, publishing, secrets,
	// the judge unsure).
	Asks   string `json:"asks,omitempty"`
	Urgent string `json:"urgent,omitempty"`
	// Note and Items are as delegated; Open, once settled done, the items
	// its note doesn't have ticked.
	Note  string   `json:"note,omitempty"`
	Items []string `json:"items,omitempty"`
	Open  []string `json:"open,omitempty"`
	// Outcome is what its session last reported, one of GOAOutcomes; ReportAt
	// that report's time (SessionInfo.ReportAt).
	Outcome  string `json:"outcome,omitempty"`
	ReportAt int64  `json:"reportAt,omitempty"`
	// NoReport is that its session ended without one.
	NoReport bool      `json:"noReport,omitempty"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
}

// GOABoard is GOA's board on a host.
type GOABoard struct {
	// Session is GOA's session on this host, "" for none running.
	Session string    `json:"session,omitempty"`
	Agent   string    `json:"agent,omitempty"`
	Level   string    `json:"level,omitempty"`
	Tasks   []GOATask `json:"tasks"`
	// Told are the sessions GOA sent something to with no task (session send),
	// which report to it all the same: the app relays the other hosts'.
	Told []string `json:"told,omitempty"`
}

// GOADelegateParams is a task GOA (From, its session) hands over: to
// Session on Host, or in Project there to its session (of Agent, when
// given) that is free, or else to a new one; with Branch, always a new one.
type GOADelegateParams struct {
	From    string `json:"from,omitempty"`
	Title   string `json:"title,omitempty"`
	Prompt  string `json:"prompt"`
	Host    string `json:"host,omitempty"`
	Session string `json:"session,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Project string `json:"project,omitempty"`
	// Branch gives a new session a new branch in a worktree of its own.
	Branch string `json:"branch,omitempty"`
	// Model, Effort and Config are chosen by hand, over the route.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	Config string `json:"config,omitempty"`
	// NoRoute keeps the agent's own model and effort, the judge not asked.
	NoRoute         bool `json:"noRoute,omitempty"`
	SkipPermissions bool `json:"skipPermissions,omitempty"`
	// Note and Items are the note the task belongs to and its to-dos it finishes.
	Note  string   `json:"note,omitempty"`
	Items []string `json:"items,omitempty"`
}

// GOATaskParams set task ID's status and result ("" leaves either).
type GOATaskParams struct {
	ID     string `json:"id"`
	Status string `json:"status,omitempty"`
	Result string `json:"result,omitempty"`
}

// GOAOverrideParams are the user's model and effort for task ID.
type GOAOverrideParams struct {
	ID     string `json:"id"`
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// MethodGOAReport is a session saying how a turn ended, on its own host:
// GOAReportParams.
const MethodGOAReport = "goa.report"

// GOAOutcomes are what a session can report of a turn.
var GOAOutcomes = []string{"done", "partly", "nothing", "blocked", "refused", "failed"}

// GOAReportParams is session Session's report of its turn, whatever came of it.
type GOAReportParams struct {
	Session string `json:"session"`
	Outcome string `json:"outcome"`
	Text    string `json:"text,omitempty"`
}

// GOAWorkerParams are a session of another host, as the app saw it
// change: Host as the app names it.
type GOAWorkerParams struct {
	Host    string      `json:"host"`
	Session SessionInfo `json:"session"`
}

// GOAFlag is something a GOA only watched did itself.
type GOAFlag struct {
	ID      string    `json:"id"`
	Session string    `json:"session"`
	Time    time.Time `json:"time"`
	// Project and File: a file it changed ("modified", "added",
	// "deleted"); Command: a program it ran instead.
	Project string `json:"project,omitempty"`
	File    string `json:"file,omitempty"`
	Change  string `json:"change,omitempty"`
	Command string `json:"command,omitempty"`
	// Revertable: the file can be put back as it was before GOA started.
	Revertable bool `json:"revertable,omitempty"`
	Reverted   bool `json:"reverted,omitempty"`
}

// GOARevertParams name a flag whose file to put back.
type GOARevertParams struct {
	ID string `json:"id"`
}

// What GOA keeps in its collection.
const (
	GOAKindTask       = "task"
	GOAKindDelegation = "delegation"
	GOAKindOutcome    = "outcome"
	GOAKindDecision   = "decision"
	GOAKindPreference = "preference"
)

// GOARememberParams keep Text, of Kind, in GOA's collection; Session is
// GOA's.
type GOARememberParams struct {
	Session string `json:"session"`
	Kind    string `json:"kind,omitempty"`
	Text    string `json:"text"`
}

// GOARecallParams search GOA's collection for Query, and the projects
// named, read-only.
type GOARecallParams struct {
	Session  string       `json:"session"`
	Query    string       `json:"query"`
	K        int          `json:"k,omitempty"`
	Projects []GOAProject `json:"projects,omitempty"`
}

// GOAProject is a project on a host ("" GOA's).
type GOAProject struct {
	Host string `json:"host,omitempty"`
	Path string `json:"path"`
}

// GOARules are the user's rules for GOD of Agents, Markdown: how it
// delegates, what it leaves to the user, what it tells its workers.
type GOARules struct {
	Rules string `json:"rules"`
	// Clear: the user agreed to Rules being empty, over rules there.
	Clear bool `json:"clear,omitempty"`
}

// GOAStall are how long a task may not move before GOA is told, in seconds; 0
// (to keep) is its default.
type GOAStall struct {
	// Blank: its session's screen is still empty after the task.
	Blank int `json:"blank"`
	// Idle: its session is hibernated or gone, or its agent isn't at work.
	Idle int `json:"idle"`
	// Stuck: its agent says it works, but nothing shows or uses the CPU.
	Stuck int `json:"stuck"`
}
