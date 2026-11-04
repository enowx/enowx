// Package platformagent is Claude Platform, enowX's own coding agent: a Go loop over
// Anthropic's Messages API. It runs as a program in a session's terminal ("$ENOWX_BIN"
// claude-platform, core/platformagent/terminal), so sessions, tabs, hibernation and
// remote hosts work as for any agent, and needs nothing installed on the host. See
// docs/claude-platform.md.
package platformagent

import (
	"context"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
)

// Decision is the user's answer to a tool asking for permission.
type Decision int

const (
	Deny Decision = iota
	AllowOnce
	AllowAlways
)

// PermissionRequest is a tool call waiting for the user.
type PermissionRequest struct {
	Tool   string
	Detail string
	// Input is the call's whole input, for whatever judges it: Detail may be clipped.
	Input []byte
	// Rule is what AllowAlways keeps: the tool's name, or Bash(git *). "" when nothing
	// narrow enough would cover the call: AllowAlways then allows this once.
	Rule string
}

// Front is what the user sees and answers through, the terminal in a session and a
// fake in tests. Agent calls it from its turn's goroutine, one call at a time, except
// that Event may come while tools run side by side, so implementations lock.
type Front interface {
	// Event shows a record the moment it is kept: the same a chat reads from the file.
	Event(r record.Record)
	// Permission asks to allow a call; ctx ending means no answer came.
	Permission(ctx context.Context, req PermissionRequest) (Decision, error)
	// Questions asks the AskUserQuestion tool's questions.
	Questions(ctx context.Context, qs []record.Question) ([]string, error)
	// OverBudget asks whether to go on past the session's budget, spent dollars so far.
	OverBudget(ctx context.Context, spent, budget float64) (bool, error)
}

// HookAnswer is what enowX's hooks say of a tool call about to run.
type HookAnswer struct {
	// Deny refuses it, with Why told to the model (GOA's rule, a user's hook).
	Deny bool
	Why  string
	// Allow lets it run without asking (the judge approved it).
	Allow bool
	// Context is text for the model from a user's hook.
	Context string
	// Command runs in place of a Bash call's (a user's rewrite, trimming long output).
	Command string
}

// Hooks hands enowX the agent's events; nil outside a session. The agent acts on what
// they answer, which is what makes GOA enforced on it. ToolBefore and ToolAfter may
// come side by side while read-only calls run, so implementations lock.
type Hooks interface {
	// State reports proto.State* for the session.
	State(state string)
	// ToolBefore asks about a call about to run; ToolAfter says how it went.
	ToolBefore(ctx context.Context, tool string, input []byte) HookAnswer
	ToolAfter(ctx context.Context, tool string, input []byte, failed bool)
	// Prompt says a prompt reached the agent: Deny stops it, with Why, and Context goes
	// with it to the model.
	Prompt(ctx context.Context, text string) HookAnswer
	// TurnDone says a turn ended; what it returns keeps the agent going, as a message
	// (GOD of Agents' lines).
	TurnDone(ctx context.Context) string
}

// Options start an Agent. Key and BaseURL come from the environment the session was
// given (ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN, ANTHROPIC_BASE_URL), never a file.
type Options struct {
	Dir        string // working folder
	SessionDir string // where the conversation's file is kept
	// Resume continues the conversation of this ID; Continue the newest in SessionDir.
	Resume   string
	Continue bool

	// Mode is --permission-mode: it holds over what a conversation kept.
	Model, Effort, Mode string
	// ModeIfNew is the mode to start in when nothing is kept: what enowX starts a session
	// again with.
	ModeIfNew string
	// SkipPermissions starts a new conversation in bypass; it and AllowBypass, which only
	// offers it, let a conversation that was in bypass come back to it.
	SkipPermissions, AllowBypass bool
	Budget                       float64

	// SystemPrompt, when not empty, replaces the built-in prompt whole; Append goes
	// after it (enowX's inject and guide).
	SystemPrompt, Append string

	// MCPConfig is an mcp.json's path or text (mcpclient.Connect), "" for none.
	MCPConfig string

	Front Front
	Hooks Hooks

	// ExtraTools are offered besides the built-in ones, and win over one of their name.
	ExtraTools []tools.Tool
	// FirstParty says BaseURL is the real API behind another address, so requests may
	// carry what a proxy could reject (eager input streaming, refusal fallbacks); an
	// unset BaseURL or Anthropic's own address is first-party anyway.
	FirstParty bool
}

// Runner is the turn-taking API the terminal drives; Agent implements it.
type Runner interface {
	// Turn runs one user message to its end. Cancelling ctx interrupts it: running
	// tools are killed and the history stays valid for the next turn.
	Turn(ctx context.Context, text string, attachments []string) error
	// Settings now, and changes that apply from the next request.
	Settings() (model, effort, mode string, budget float64)
	SetModel(string) error
	SetEffort(string) error
	SetMode(string) error
	SetBudget(float64)
	// Compact summarises the conversation now.
	Compact(ctx context.Context) error
	// Clear starts a new conversation.
	Clear() error
	// Cost is the session's dollars so far, Context the tokens the conversation fills
	// and the model's window.
	Cost() float64
	Context() (tokens, window int64)
	Conversation() string
	Close() error
}
