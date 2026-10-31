package proto

import "time"

// Agents pointed at a model API of the user's choosing: its address, key
// and model, in each agent's own settings on the host, which are kept
// first to be put back (core/agentapi, docs/agent-api.md).
const (
	MethodAgentAPIList    = "agentapi.list"    // → []AgentAPIInfo
	MethodAgentAPISet     = "agentapi.set"     // AgentAPISetParams → AgentAPIInfo
	MethodAgentAPIClear   = "agentapi.clear"   // AgentAPIRef → AgentAPIInfo
	MethodAgentAPIRestore = "agentapi.restore" // AgentAPIRestoreParams → AgentAPIInfo
)

// AgentAPIKeepKey as a key keeps the key the agent is set with: clients
// only see it masked.
const AgentAPIKeepKey = "\u0000keep"

// AgentAPI is a model API. Its key is masked in what the daemon answers.
type AgentAPI struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey,omitempty"`
	// Model is the one the agent starts with; Models all it offers, Model
	// first, with what the agent takes of each (AgentAPIInfo.Caps).
	Model      string          `json:"model,omitempty"`
	Models     []AgentAPIModel `json:"models,omitempty"`
	SmallModel string          `json:"smallModel,omitempty"`
	Effort     string          `json:"effort,omitempty"`   // for every model, where Caps.Effort is "global"
	Protocol   string          `json:"protocol,omitempty"` // anthropic, openai, openai-responses, gemini
}

// AgentAPIModel is one of an API's models; zero values leave it to the
// agent.
type AgentAPIModel struct {
	ID      string `json:"id"`
	Vision  bool   `json:"vision,omitempty"`
	Context int    `json:"context,omitempty"` // tokens
	Output  int    `json:"output,omitempty"`  // tokens
	Effort  string `json:"effort,omitempty"`
}

// AgentAPICaps are what an agent takes of an API's models: several of
// them; images; the context window and the output limit per model
// ("model") or the default's for all ("global"); the reasoning effort per
// model or one for all, and its values.
type AgentAPICaps struct {
	Multi   bool     `json:"multi,omitempty"`
	Vision  bool     `json:"vision,omitempty"`
	Context string   `json:"context,omitempty"`
	Output  string   `json:"output,omitempty"`
	Effort  string   `json:"effort,omitempty"`
	Efforts []string `json:"efforts,omitempty"`
}

// AgentAPIInfo is an agent's API on the host: the protocols it speaks to
// one, the files it is set in, the API it is set to (nil for its own),
// and the backups of its files.
type AgentAPIInfo struct {
	Agent     string           `json:"agent"`
	Name      string           `json:"name"`
	Installed bool             `json:"installed,omitempty"`
	Protocols []string         `json:"protocols"`
	Files     []string         `json:"files"`
	Note      string           `json:"note,omitempty"`
	Small     bool             `json:"small,omitempty"`   // it takes a model for quick work
	NoModel   bool             `json:"noModel,omitempty"` // it takes no model: the API's own
	Caps      AgentAPICaps     `json:"caps"`
	Current   *AgentAPI        `json:"current,omitempty"`
	Backups   []AgentAPIBackup `json:"backups,omitempty"`
	Error     string           `json:"error,omitempty"`
	// PerSession: a config can give it an API per session, its own
	// settings untouched (proto/config.go); otherwise only Set writes it.
	PerSession bool `json:"perSession,omitempty"`
}

// AgentAPIBackup is a backup of an agent's files: Original is the files
// as the user had them before enowX changed them.
type AgentAPIBackup struct {
	ID       string    `json:"id"`
	Time     time.Time `json:"time"`
	Original bool      `json:"original,omitempty"`
	Note     string    `json:"note,omitempty"`
}

// AgentAPIRef names an agent.
type AgentAPIRef struct {
	Agent string `json:"agent"`
}

// AgentAPISetParams point Agent at API.
type AgentAPISetParams struct {
	Agent string   `json:"agent"`
	API   AgentAPI `json:"api"`
}

// AgentAPIRestoreParams put Agent's files back as Backup kept them ("" for
// the original).
type AgentAPIRestoreParams struct {
	Agent  string `json:"agent"`
	Backup string `json:"backup,omitempty"`
}
