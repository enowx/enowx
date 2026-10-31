package proto

import (
	"encoding/json"
	"time"
)

// A session controlling sessions and settings of any host (docs/cli.md,
// "Control"): one method relays another to a host, this one or one the
// user's app reaches; core/hostcontrol says which methods it reaches, and
// those that lose or publish something are confirmed by the user in the
// app when an agent asks.
const (
	// MethodHostCall calls Method with Params on Host: HostCallParams →
	// the method's result.
	MethodHostCall = "host.call"
	// MethodHostControlLog lists the requests made through host.call on
	// this host lately, the newest last: → []ControlLogEntry.
	MethodHostControlLog = "host.controlLog"
	// MethodSessionInterrupt stops what a session's agent is doing, as its
	// user would with the agent's interrupt key (Escape, Ctrl+C):
	// SessionRef.
	MethodSessionInterrupt = "session.interrupt"
	// MethodSessionAsk says what a session's agent waits on the user for:
	// a tool to allow, or its questions. SessionRef → SessionAsk.
	MethodSessionAsk = "session.ask"
	// MethodSessionAnswer answers it, pressing the agent's own keys:
	// SessionAnswerParams → SessionAsk (as it is after).
	MethodSessionAnswer = "session.answer"
	// MethodAppSettings reads the app's own settings (what New session
	// starts with, the terminals' font, the theme…), changing those Set
	// names first; the app attached answers. AppSettingsParams →
	// AppSettings.
	MethodAppSettings = "app.settings"
	// MethodProjectConfig reads a project's .enowx.toml (ProjectRef →
	// ProjectConfig); MethodProjectConfigSave writes it (ProjectConfig →
	// ProjectConfig).
	MethodProjectConfig     = "project.config"
	MethodProjectConfigSave = "project.config.save"
)

// HostCallParams call Method with Params on Host: a host's name as the
// app lists it, its daemon's ID, or "" for this one. From is the session
// asking, "" for the user's own terminal.
type HostCallParams struct {
	Host   string          `json:"host,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	From   string          `json:"from,omitempty"`
}

// ControlLogEntry is a request made through host.call: when, from which
// session, to which host, the method and its level, and how it ended.
type ControlLogEntry struct {
	Time   time.Time `json:"time"`
	From   string    `json:"from,omitempty"`
	Host   string    `json:"host,omitempty"` // "" this one
	Method string    `json:"method"`
	Level  string    `json:"level"`
	Err    string    `json:"err,omitempty"`
}

// Answers to a session's agent asking to use a tool.
const (
	AnswerAllow  = "allow"  // this once
	AnswerAlways = "always" // from now on
	AnswerDeny   = "deny"
)

// SessionAsk is what a session's agent waits on: Waiting, a tool call
// (Tool, Detail: what it would do) or Questions, and the Answers its keys
// are known for ("allow", "always", "deny", or a question's option
// numbers "1", "2"…). Reason says why it can't be answered from here.
type SessionAsk struct {
	Waiting   bool                 `json:"waiting"`
	Tool      string               `json:"tool,omitempty"`
	Detail    string               `json:"detail,omitempty"`
	Questions []TranscriptQuestion `json:"questions,omitempty"`
	Answers   []string             `json:"answers,omitempty"`
	Reason    string               `json:"reason,omitempty"`
}

// SessionAnswerParams answer session ID's agent: one of SessionAsk's
// Answers.
type SessionAnswerParams struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

// AppSettingsParams change the app's settings Set names (AppSettings'
// keys) before reading them all; none only reads.
type AppSettingsParams struct {
	Set map[string]string `json:"set,omitempty"`
}

// AppSettings are the app's settings an agent reaches, by key, as text.
type AppSettings struct {
	Values map[string]string `json:"values"`
	// About says what each key is and the values it takes.
	About map[string]string `json:"about,omitempty"`
}

// ProjectConfig is a project's .enowx.toml (docs/project-settings.md):
// what new worktrees get from its folder (Copy), and the script that
// readies one (Setup).
type ProjectConfig struct {
	Path  string   `json:"path"`
	Copy  []string `json:"copy"`
	Setup string   `json:"setup,omitempty"`
}
