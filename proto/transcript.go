package proto

import "time"

// MethodSessionTranscript reads a session's conversation as the agent
// records it, for the chat view: TranscriptParams → Transcript.
const MethodSessionTranscript = "session.transcript"

// Kinds of transcript entries.
const (
	TranscriptUser      = "user"
	TranscriptAssistant = "assistant"
	TranscriptThinking  = "thinking"
	TranscriptTool      = "tool"
	TranscriptNotice    = "notice"
)

// TranscriptParams asks for the entries changed after revision Since.
// Asking from the start (Since 0), or when the transcript starts over
// (Reset), Tail limits the answer to about the last Tail entries, whole
// turns; Before, when set, asks instead for about Tail
// entries before that index, to show older ones. Daemons before these
// fields answer with every entry.
type TranscriptParams struct {
	ID     string `json:"id"`
	Since  int64  `json:"since,omitempty"`
	Tail   int    `json:"tail,omitempty"`
	Before int    `json:"before,omitempty"`
}

// Transcript is a session's conversation, or why there is none.
type Transcript struct {
	Supported bool   `json:"supported"`        // the agent keeps a transcript enowX reads
	Reason    string `json:"reason,omitempty"` // why not, or why nothing shows yet
	// Entries are those added or changed after Since, by Index.
	Entries []TranscriptEntry `json:"entries,omitempty"`
	Rev     int64             `json:"rev"`   // the latest revision
	Reset   bool              `json:"reset"` // the transcript started over: drop what you have
	// The agent's settings now, when it says: the model it answers with,
	// its permission mode and its effort, as the agent names them.
	Model  string `json:"model,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Effort string `json:"effort,omitempty"`
	// ContextTokens is how much of the model's context the conversation
	// fills, as of the agent's last answer; ContextWindow is how much the
	// model holds. Zero when unknown.
	ContextTokens int64 `json:"contextTokens,omitempty"`
	ContextWindow int64 `json:"contextWindow,omitempty"`
	// CostUSD is what the conversation has cost at list prices, BudgetUSD what it may
	// cost without the user's yes; for agents that say (Claude Platform). Zero: unknown.
	CostUSD   float64 `json:"costUsd,omitempty"`
	BudgetUSD float64 `json:"budgetUsd,omitempty"`
}

// MethodSessionMode switches a session's agent to another permission mode
// the way its user would, as the agent's preset says: SessionModeParams.
// It returns once the agent's screen says the mode, or with why it didn't.
const MethodSessionMode = "session.mode"

// SessionModeParams names the mode, by the agent's own name for it.
type SessionModeParams struct {
	ID   string `json:"id"`
	Mode string `json:"mode"`
}

// MethodSessionSettings changes a session's model and effort, as its user would, and
// returns once its agent is on them: SessionSettingsParams. An agent whose own /model and
// /effort save what they set as the default for new sessions, in a file all its sessions
// read, is never given them: the session starts again on its conversation, with its
// command line's flags. Others are told the way they take them.
const MethodSessionSettings = "session.settings"

// SessionSettingsParams change a session's model, its effort, or both; "" keeps one,
// and "default" asks for the agent's own.
type SessionSettingsParams struct {
	ID     string `json:"id"`
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// MethodSessionRewind takes a session's agent back to before one of the
// user's messages, as its user would, so that a message sent next takes
// its place: SessionRewindParams. It answers once the agent waits for that
// message, or with why it couldn't go back.
const MethodSessionRewind = "session.rewind"

// SessionRewindParams name the message: Back of the user's messages from
// the end (1 for the last), and its text, to find it by.
type SessionRewindParams struct {
	ID   string `json:"id"`
	Back int    `json:"back"`
	Text string `json:"text"`
}

// TranscriptEntry is one message, thought or tool call.
type TranscriptEntry struct {
	Index int       `json:"index"`
	Rev   int64     `json:"rev"`
	Kind  string    `json:"kind"`
	Time  time.Time `json:"time,omitzero"`
	Text  string    `json:"text,omitempty"` // the message, or a tool's name
	// Tool calls: what the call does, and its result once it came.
	Detail string `json:"detail,omitempty"`
	Result string `json:"result,omitempty"`
	Done   bool   `json:"done,omitempty"`
	Failed bool   `json:"failed,omitempty"`
	// File edits: the text replaced and its replacement, clipped, and how
	// many lines went and came.
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
	Added   int    `json:"added,omitempty"`
	Removed int    `json:"removed,omitempty"`
	// Thoughts: how long the agent thought; tool calls: how long they
	// took, from the call to its result. In milliseconds.
	DurationMs int64 `json:"durationMs,omitempty"`
	// Questions the agent asks the user, with options to pick from (a
	// tool asking them, as Claude Code's AskUserQuestion). Older daemons
	// leave it out.
	Questions []TranscriptQuestion `json:"questions,omitempty"`
}

// TranscriptQuestion is a question an agent asks: a short Header, the
// Question, the Options offered (their labels, then descriptions), and
// whether several may be picked.
type TranscriptQuestion struct {
	Header   string   `json:"header,omitempty"`
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
	About    []string `json:"about,omitempty"`
	Multi    bool     `json:"multi,omitempty"`
}

// MethodSessionCommands lists the slash commands of a session's agent:
// SessionRef → AgentCommands. Asking the agent takes seconds, so the first
// answer may say Loading; ask again for the list.
const MethodSessionCommands = "session.commands"

// AgentCommands are an agent's slash commands, or that they are on the way.
type AgentCommands struct {
	Commands []AgentCommand `json:"commands,omitempty"`
	Loading  bool           `json:"loading,omitempty"`
}

// AgentCommand is one slash command, named without its slash.
type AgentCommand struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"` // "built-in", "command" or "skill"
}

// MethodAgentHistory lists the conversations agents had in a project's
// folder and its worktrees, newest first, to go on with one:
// HistoryParams → []Conversation.
const MethodAgentHistory = "agent.history"

// HistoryParams name the project.
type HistoryParams struct {
	Project string `json:"project"`
	Limit   int    `json:"limit,omitempty"`
}

// Conversation is a conversation an agent recorded.
type Conversation struct {
	Agent    string    `json:"agent"`
	ID       string    `json:"id"`              // what the agent resumes it by
	Title    string    `json:"title,omitempty"` // its first message's first line
	Dir      string    `json:"dir"`             // the folder it ran in
	Branch   string    `json:"branch,omitempty"`
	Model    string    `json:"model,omitempty"`
	Messages int       `json:"messages"`
	Updated  time.Time `json:"updated"`
	Path     string    `json:"path"` // its file on the host
}
