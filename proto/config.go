package proto

// An agent's configs: what a session of it runs with, picked at New
// session. The official config is the agent as installed, never written
// to by enowX; the others are Accounts (MethodAccountList): another
// sign-in of the agent's (Kind ConfigKindAccount, a home of its own) or a
// model API (Kind ConfigKindAPI), each with its effort, extra variables and
// flags, a name and a badge. A config is applied to a session when it
// starts, through its environment and command line (docs/configs.md).
const (
	// MethodAccountSave adds a config (ID empty) or changes one.
	MethodAccountSave = "account.save" // Account → Account
)

// Config kinds (Account.Kind).
const (
	ConfigKindAccount = ""    // another sign-in, or a key of the vault's (Cred)
	ConfigKindAPI     = "api" // a model API
)

// ConfigAPI is a model API config's: where, how it is spoken, the vault's
// credential holding its key (read on the host starting the session,
// never written to disk), its models.
type ConfigAPI struct {
	BaseURL  string `json:"baseURL"`
	Protocol string `json:"protocol,omitempty"` // anthropic, openai, openai-responses, gemini
	KeyCred  string `json:"keyCred,omitempty"`
	// Model is the one the agent starts with: Models' first.
	Model      string          `json:"model,omitempty"`
	Models     []AgentAPIModel `json:"models,omitempty"`
	SmallModel string          `json:"smallModel,omitempty"`
}

// SessionConfig is the config a session runs with (SessionInfo.Config),
// nil for its agent's official one: what is shown on it and what reads
// it (the judge's policy, token tracking) need.
type SessionConfig struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind,omitempty"`
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
	// Provider is where its models come from: a provider ID (anthropic,
	// openai, …, local, other) for a model API, "" for the agent's own.
	Provider string `json:"provider,omitempty"`
	BaseURL  string `json:"baseURL,omitempty"`
	Model    string `json:"model,omitempty"`
}
