// Package record is what Claude Platform, enowX's own agent, keeps of a
// conversation: one JSON line per Record, in <session dir>/<id>.jsonl. The agent
// resumes from these lines, the chat view (core/transcript) and token usage
// (core/tokens) read them, so this package holds no API client. No record ever
// holds an API key.
package record

import (
	"encoding/json"
	"time"
)

// AgentID is the agent's preset ID.
const AgentID = "claude-platform"

// Types of record.
const (
	// Meta opens a conversation and says each change of its settings.
	Meta = "meta"
	// User is a message of the user's: Text as shown, Message as sent.
	User = "user"
	// BlockStart and Delta stream the answer under way, for the chat alone: an
	// Assistant record with the same Msg replaces them. Resuming ignores them.
	BlockStart = "block_start"
	Delta      = "delta"
	// Assistant is a finished answer of the API's: Message replayed unchanged
	// (thinking blocks and their signatures included), Usage and cost.
	Assistant = "assistant"
	// ToolStart says a tool call began; ToolWait that it waits for the user's answer
	// (a permission or questions); ToolEnd how it ended.
	ToolStart = "tool_start"
	ToolWait  = "tool_wait"
	ToolEnd   = "tool_end"
	// Results is the user message of tool results that follows an Assistant's calls,
	// all of them in one message, as the API takes them.
	Results = "results"
	// Notice is a line for the user: a retry, an error, a refusal, the budget.
	Notice = "notice"
	// Compact starts the API's history over from Messages; what came before stays
	// in the file for the chat.
	Compact = "compact"
	// TurnEnd says the turn ended, how in Stop.
	TurnEnd = "turn_end"
	// Allow keeps an "always allow" of the user's, so a resumed session has it.
	Allow = "allow"
)

// Record is one line. Message-bearing records (User, Assistant, Results) replayed
// in order, from the last Compact on, are the API's history.
type Record struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`

	// Message is the API's MessageParam JSON, unchanged.
	Message json.RawMessage `json:"message,omitempty"`
	// Messages are the history a Compact starts from.
	Messages []json.RawMessage `json:"messages,omitempty"`

	// Meta.
	Conversation string  `json:"conversation,omitempty"`
	Dir          string  `json:"dir,omitempty"`
	Model        string  `json:"model,omitempty"`
	Effort       string  `json:"effort,omitempty"`
	Mode         string  `json:"mode,omitempty"`
	Budget       float64 `json:"budget,omitempty"` // dollars a session may spend; 0 none
	Window       int64   `json:"window,omitempty"`

	// User, Notice: what the chat shows. Delta: the text added.
	Text string `json:"text,omitempty"`
	// Level of a Notice: "info", "warning", "error".
	Level string `json:"level,omitempty"`
	// Attachments are the files a user message took along.
	Attachments []string `json:"attachments,omitempty"`

	// BlockStart, Delta, Assistant: the API's message ID; the content block's index
	// and kind ("text", "thinking").
	Msg   string `json:"msg,omitempty"`
	Block int    `json:"block,omitempty"`
	Kind  string `json:"kind,omitempty"`

	// Assistant.
	Usage *Usage `json:"usage,omitempty"`
	// Cost of this answer and of the session so far, in dollars at list prices.
	Cost      float64 `json:"cost,omitempty"`
	TotalCost float64 `json:"totalCost,omitempty"`
	// Stop is the API's stop_reason on an Assistant; on a TurnEnd "done",
	// "interrupted", "error", "budget" or "refusal".
	Stop string `json:"stop,omitempty"`

	// ToolStart, ToolWait, ToolEnd.
	Tool *Tool `json:"tool,omitempty"`
	// Asks is set on a ToolWait: "permission" or "questions".
	Asks string `json:"asks,omitempty"`

	// Compact: the tokens the conversation held before it.
	Before int64 `json:"before,omitempty"`

	// Allow: the rule kept (a tool's name, or Bash(prefix *)).
	Rule string `json:"rule,omitempty"`

	// Sidechain marks what a subagent (the Task tool) did: its parent call's ID. The
	// chat folds it into that call; it isn't the API's history.
	Sidechain string `json:"sidechain,omitempty"`
}

// Tool is a tool call as the chat shows it.
type Tool struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input,omitempty"`
	Detail string          `json:"detail,omitempty"` // what it does, one line
	// Result, clipped for the chat; the Results record has what the API got.
	Result     string `json:"result,omitempty"`
	Failed     bool   `json:"failed,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	// File edits.
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
	Added   int    `json:"added,omitempty"`
	Removed int    `json:"removed,omitempty"`
	// Questions the AskUserQuestion tool asks.
	Questions []Question `json:"questions,omitempty"`
}

// Question is one the agent asks the user.
type Question struct {
	Header   string   `json:"header,omitempty"`
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
	About    []string `json:"about,omitempty"`
	Multi    bool     `json:"multi,omitempty"`
}

// Usage is an answer's tokens, as the API counts them: Input leaves the cache out.
type Usage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

// Modes of permission, Claude Code's names so that enowX's settings and GOA read them.
const (
	ModeDefault     = "default"           // asks before what changes anything
	ModeAcceptEdits = "acceptEdits"       // edits files without asking
	ModePlan        = "plan"              // reads only
	ModeBypass      = "bypassPermissions" // asks nothing
)

// Default settings of a new conversation (the owner's open decision, taken in
// docs/claude-platform.md): the current Opus, high effort, no more than $10 a session
// without the user's yes.
const (
	DefaultModel          = "claude-opus-5-5"
	DefaultEffort         = "high"
	DefaultBudget float64 = 10
)
