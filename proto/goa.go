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
	GOATaskDone    = "done"
	GOATaskFailed  = "failed"
	GOATaskStopped = "stopped"
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
	// Model, Effort and Config are what the prompt runs with ("" the
	// agent's own); Why says why, Difficulty how hard the judge found it.
	Model      string `json:"model,omitempty"`
	Effort     string `json:"effort,omitempty"`
	Config     string `json:"config,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
	Why        string `json:"why,omitempty"`
	// Override: the user chose the model and effort, not the judge.
	Override bool   `json:"override,omitempty"`
	Status   string `json:"status"`
	Result   string `json:"result,omitempty"`
	// Asks is what its session waits on; Urgent, why the user answers it
	// rather than GOA (it can't be undone, money, publishing, secrets,
	// the judge unsure).
	Asks    string    `json:"asks,omitempty"`
	Urgent  string    `json:"urgent,omitempty"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// GOABoard is GOA's board on a host.
type GOABoard struct {
	// Session is GOA's session on this host, "" for none running.
	Session string    `json:"session,omitempty"`
	Agent   string    `json:"agent,omitempty"`
	Level   string    `json:"level,omitempty"`
	Tasks   []GOATask `json:"tasks"`
}

// GOADelegateParams is a task GOA (From, its session) hands over: to
// Session on Host, or to a new session of Agent in Project there.
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
