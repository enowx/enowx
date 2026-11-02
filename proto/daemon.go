package proto

import "time"

// The daemon itself and the agents it can start.
const (
	MethodDaemonInfo = "daemon.info" // → DaemonInfo
	MethodAgentList  = "agent.list"  // → []AgentInfo
)

// DaemonInfo describes the running daemon.
type DaemonInfo struct {
	Version string `json:"version"`
	// Build is set by development builds only (see version.Build).
	Build    string    `json:"build,omitempty"`
	PID      int       `json:"pid"`
	Started  time.Time `json:"started"`
	Sessions int       `json:"sessions"`
	Hostname string    `json:"hostname"`
	Home     string    `json:"home"`
	// Hash is the SHA-256 of the executable the daemon started from: the
	// code it runs, which a newer file on disk doesn't change. "" when
	// unknown (development daemons started without it, older ones).
	Hash string `json:"hash,omitempty"`
	// Display says sessions can run with a virtual display here;
	// DisplayNote, when they can't, what the host needs.
	Display     bool   `json:"display,omitempty"`
	DisplayNote string `json:"displayNote,omitempty"`
	// DisplayInstall is the command that installs what displays need,
	// when they can't run yet and the host's package manager is known.
	DisplayInstall string `json:"displayInstall,omitempty"`
	// ID names this daemon's folder for good, whatever the host's name or
	// address: links between hosts' copies of a project name it.
	ID string `json:"id,omitempty"`
}

// AgentInfo describes an agent the daemon can start.
type AgentInfo struct {
	ID        string `json:"id"`      // "claude", "codex", "shell", "custom", …
	Name      string `json:"name"`    // "Claude Code"
	Command   string `json:"command"` // "claude"; empty for shell and custom
	Installed bool   `json:"installed"`
	// CanSkipPermissions: the agent has a mode that runs tools without asking.
	CanSkipPermissions bool `json:"canSkipPermissions,omitempty"`
	// Install is a shell command that installs the agent, "" if unknown.
	Install string `json:"install,omitempty"`
	// SignIn is the command that signs the agent in without a browser on
	// the host, as it reads; "" when there is none (or an older daemon).
	SignIn string `json:"signIn,omitempty"`
	// Homepage is where the agent's documentation is.
	Homepage string `json:"homepage,omitempty"`
}

// Settings of the daemon's host, kept by the daemon.
const (
	MethodDaemonSettings    = "daemon.settings"    // → DaemonSettings
	MethodDaemonSetSettings = "daemon.setSettings" // DaemonSettings → DaemonSettings
)

// MethodDaemonStopAgents stops every agent running on the host, keeping
// their sessions to continue later, as hibernation does: when the user
// quits the app and wants nothing working. → StopAgentsResult.
const MethodDaemonStopAgents = "daemon.stopAgents"

// StopAgentsResult says how many agents were stopped.
type StopAgentsResult struct {
	Stopped int `json:"stopped"`
}

// AgentStart is an agent's model, effort and permission mode for new sessions, each ""
// for what the agent's own settings say.
type AgentStart struct {
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	Mode   string `json:"mode,omitempty"`
}

// DaemonSettings are the host's settings. MaxAgents is how many agents may
// work at once (0: no limit); sessions started past it wait, queued, for
// one to finish its turn.
type DaemonSettings struct {
	MaxAgents int `json:"maxAgents"`
	// AgentStart is what new sessions start on, by agent: the user's choice, kept here so
	// that it doesn't follow what one session wrote to an agent's own settings.
	AgentStart map[string]AgentStart `json:"agentStart,omitempty"`
	// HibernateAfter is how many minutes an agent done with its turn may
	// stay idle before the daemon stops it to free what it holds, keeping
	// its session to continue; 0 never does.
	HibernateAfter int `json:"hibernateAfter,omitempty"`
}

// MethodSessionBudget sets how long a session's agent may work:
// SessionBudgetParams. Past it, the daemon interrupts the agent.
const MethodSessionBudget = "session.budget"

// SessionBudgetParams sets session ID's budget, in seconds; 0 lifts it.
type SessionBudgetParams struct {
	ID      string `json:"id"`
	Seconds int64  `json:"seconds"`
}
