package proto

import "time"

// RAG: a project found by meaning, on any host. The daemon indexes the
// projects of its host (core/rag) in its built-in SQLite store or the
// user's PostgreSQL with pgvector, with the embedder the user picked, only
// once the user agreed to send the project's text to it; agents search it
// and keep memories in it. See docs/rag.md.
const (
	MethodRAGSettings    = "rag.settings"    // → RAGSettings
	MethodRAGSetSettings = "rag.setSettings" // RAGSettings → RAGSettings
	MethodRAGTest        = "rag.test"        // RAGSettings → RAGTestResult: embed a line with them
	MethodRAGStatus      = "rag.status"      // RAGRef → RAGStatus
	MethodRAGSetProject  = "rag.setProject"  // RAGProjectParams → RAGStatus
	MethodRAGIndex       = "rag.index"       // RAGRef → RAGStatus, the index started
	MethodRAGPlan        = "rag.plan"        // RAGRef → RAGPlan: what an index would send
	MethodRAGSearch      = "rag.search"      // RAGSearchParams → []RAGHit
	MethodRAGRemember    = "rag.remember"    // RAGRememberParams → RAGMemory
	MethodRAGMemories    = "rag.memories"    // RAGRef → []RAGMemory
	MethodRAGForget      = "rag.forget"      // RAGForgetParams
	// EventRAGProgress carries a RAGStatus as a project's index goes.
	EventRAGProgress = "rag.progress"
	// EventRAGOffer offers to keep what a session decided as a memory.
	EventRAGOffer = "rag.offer" // RAGRememberParams
)

// Embedders and stores (RAGSettings).
const (
	RAGFromConfig = "enowx-rag" // no longer offered: an embedder set so indexes nothing
	RAGVoyage     = "voyage"
	RAGOpenAI     = "openai" // any OpenAI-compatible /v1/embeddings (OpenAI, Ollama, OpenRouter…)
	RAGTEI        = "tei"
	RAGBuiltin    = "builtin"  // SQLite in the daemon's folder
	RAGPostgres   = "postgres" // the user's PostgreSQL with pgvector (RAGSettings.PostgresCred)
	RAGServer     = "server"   // no longer offered: taken as RAGBuiltin
)

// Sources of what is indexed (RAGHit.Source, RAGSearchParams.Scope).
const (
	RAGCode  = "code"
	RAGDocs  = "docs"
	RAGNotes = "notes"
	RAGMemo  = "memory"
)

// RAGSettings are the host's RAG settings (synced, by project ID for the
// projects'): the embedder, its model and the vault's credential with its
// key; the store; each project's.
type RAGSettings struct {
	Embedder string `json:"embedder,omitempty"` // "" none: nothing is indexed
	Model    string `json:"model,omitempty"`
	Dim      int    `json:"dim,omitempty"`
	BaseURL  string `json:"baseUrl,omitempty"`
	KeyCred  string `json:"keyCred,omitempty"`
	Store    string `json:"store,omitempty"` // RAGBuiltin when ""
	// PostgresCred is the vault's credential holding the connection URL of
	// the PostgreSQL store (RAGPostgres), its password in it.
	PostgresCred string `json:"postgresCred,omitempty"`
	// ServerURL and ServerCred are no longer used.
	ServerURL  string                `json:"serverUrl,omitempty"`
	ServerCred string                `json:"serverCred,omitempty"`
	Projects   map[string]RAGProject `json:"projects,omitempty"`
}

// RAGProject is a project's: indexed (On, the user having agreed to send
// its text to Consent's embedder), paths left out, a size limit, and
// whether a prompt gets the top results as context.
type RAGProject struct {
	On       bool     `json:"on,omitempty"`
	Consent  string   `json:"consent,omitempty"` // the embedder agreed to, "provider host"
	Exclude  []string `json:"exclude,omitempty"`
	MaxFiles int      `json:"maxFiles,omitempty"` // 0: 20000
	Context  bool     `json:"context,omitempty"`  // add results to a prompt (start-of-prompt hook)
	Offer    bool     `json:"offer,omitempty"`    // offer a session's decisions as memories
	// ServerURL, ServerCred and ServerProject are no longer used.
	ServerURL     string `json:"serverUrl,omitempty"`
	ServerCred    string `json:"serverCred,omitempty"`
	ServerProject string `json:"serverProject,omitempty"`
}

// RAGRef names a project by its folder, or a session's.
type RAGRef struct {
	Project string `json:"project,omitempty"`
	Session string `json:"session,omitempty"`
}

// RAGProjectParams sets project Project's settings.
type RAGProjectParams struct {
	Project  string     `json:"project"`
	Settings RAGProject `json:"settings"`
}

// RAGStatus is a project's index: on, its ID, the files and chunks in it,
// when last indexed, what the last index did, what it is at now, and
// tokens sent to the embedder.
type RAGStatus struct {
	Project  string     `json:"project"`
	ID       string     `json:"id"`
	Settings RAGProject `json:"settings"`
	Ready    bool       `json:"ready"` // an embedder is set
	Embedder string     `json:"embedder,omitempty"`
	Chunks   int        `json:"chunks"`
	Files    int        `json:"files,omitempty"`
	Indexed  time.Time  `json:"indexed,omitzero"`
	Indexing bool       `json:"indexing,omitempty"`
	Phase    string     `json:"phase,omitempty"` // reading, embedding, done
	Added    int        `json:"added,omitempty"`
	Removed  int        `json:"removed,omitempty"`
	Skipped  int        `json:"skipped,omitempty"`
	Tokens   int64      `json:"tokens,omitempty"`
	Error    string     `json:"error,omitempty"`
}

// RAGPlan is what indexing a project would send: to which embedder (its
// host), how many files and roughly how many tokens, and what is left out.
type RAGPlan struct {
	Embedder string   `json:"embedder"`
	Host     string   `json:"host"`
	Files    int      `json:"files"`
	Tokens   int      `json:"tokens"`
	Left     []string `json:"left,omitempty"` // what is never sent: secrets, ignored, excluded
}

// RAGSearchParams searches project (or session's) for Query, K results
// (5 when 0) from Scope's sources (all when empty).
type RAGSearchParams struct {
	RAGRef
	Query string   `json:"query"`
	K     int      `json:"k,omitempty"`
	Scope []string `json:"scope,omitempty"`
}

// RAGHit is a result: where (File, its lines, the symbol), what it says,
// how close, and its source.
type RAGHit struct {
	File    string  `json:"file,omitempty"`
	Start   int     `json:"start,omitempty"`
	End     int     `json:"end,omitempty"`
	Symbol  string  `json:"symbol,omitempty"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
	Source  string  `json:"source"`
	Memory  string  `json:"memory,omitempty"` // a memory's ID
}

// RAGRememberParams keeps Text as a memory of the project (or session's).
type RAGRememberParams struct {
	RAGRef
	Text string `json:"text"`
}

// RAGMemory is something kept about a project: a decision, a gotcha.
type RAGMemory struct {
	ID   string    `json:"id"`
	Text string    `json:"text"`
	By   string    `json:"by,omitempty"` // the session's agent, or "user"
	Time time.Time `json:"time"`
}

// RAGForgetParams removes memory ID of a project, or changes its Text.
type RAGForgetParams struct {
	RAGRef
	ID   string `json:"id"`
	Text string `json:"text,omitempty"` // with it, the memory is changed, not removed
}

// RAGTestResult is a test of the settings: the vector's size, or why not.
type RAGTestResult struct {
	Dim   int    `json:"dim,omitempty"`
	Error string `json:"error,omitempty"`
}
