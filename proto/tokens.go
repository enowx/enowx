package proto

import "time"

// Token usage: each host's daemon records the tokens its agents use, one
// record per model call (only counts, never what was said), read from the
// agents' own records, metered from their traffic, or read from their
// screen, by the best layer each agent allows. Costs come from a price
// table the user can edit; budgets alert at 80% and act at 100%. See
// docs/tokens.md.
const (
	MethodTokensSettings    = "tokens.settings"    // → TokensSettings
	MethodTokensSetSettings = "tokens.setSettings" // TokensSettings → TokensSettings (turning it on backfills)
	MethodTokensSummary     = "tokens.summary"     // TokensQuery → TokensSummary
	MethodTokensSession     = "tokens.session"     // SessionRef → TokensSummary of that session, grouped by model
	MethodTokensSessions    = "tokens.sessions"    // → []TokenSessionTotal: the sessions with use, for badges
	MethodTokensCoverage    = "tokens.coverage"    // → []TokenCoverage: each agent's layer
	MethodTokensBackfill    = "tokens.backfill"    // TokensBackfill → TokensBackfillResult
	MethodTokensPrices      = "tokens.prices"      // → []TokenPrice: the shipped table with the user's over it
	MethodTokensHosts       = "tokens.hosts"       // TokensQuery → []TokensHostDays: every host's synced daily totals
	MethodTokensBudgets     = "tokens.budgets"     // → []TokenBudgetState
	// EventTokensChanged says which sessions' totals grew, at most every
	// few seconds.
	EventTokensChanged = "tokens.changed" // TokensChanged
	// EventTokensBudget is a budget crossing 80% or 100%.
	EventTokensBudget = "tokens.budget" // TokenBudgetState
)

// Layers: where a call's counts came from, best first.
const (
	TokenLayerRecords = "records" // the agent's own records on disk
	TokenLayerTraffic = "traffic" // its model API's answers, through the interceptor
	TokenLayerScreen  = "screen"  // what it shows on its screen
	TokenLayerNone    = "none"    // not tracked, never guessed
)

// What a summary is split by (TokensQuery.By).
const (
	TokensByAgent   = "agent"
	TokensByModel   = "model"
	TokensByProject = "project"
	TokensBySession = "session"
	TokensByConfig  = "config"
	TokensByHost    = "host"
	TokensByAccount = "account"
	TokensByLayer   = "layer"
)

// TokensBy are the splits, in the order the app offers them.
var TokensBy = []string{TokensByAgent, TokensByModel, TokensByProject, TokensBySession, TokensByConfig, TokensByHost}

// enowX's own uses (TokenGroup keys of the Own split).
const (
	TokensOwnJudge = "judge" // the Decision model
	TokensOwnRAG   = "rag"   // RAG's embedder
)

// TokenCounts are tokens used and what they cost. Input is what wasn't
// read from a cache; Output includes Reasoning.
type TokenCounts struct {
	Input      int64 `json:"input,omitempty"`
	CacheRead  int64 `json:"cacheRead,omitempty"`
	CacheWrite int64 `json:"cacheWrite,omitempty"`
	Output     int64 `json:"output,omitempty"`
	Reasoning  int64 `json:"reasoning,omitempty"`
	Calls      int   `json:"calls,omitempty"`
	// Cost is in US dollars; Equivalent is the part of it that a
	// subscription paid for, the API-equivalent cost; Unpriced the calls
	// whose model the price table doesn't know.
	Cost       float64 `json:"cost,omitempty"`
	Equivalent float64 `json:"equivalent,omitempty"`
	Unpriced   int     `json:"unpriced,omitempty"`
}

// Total is every token counted.
func (c TokenCounts) Total() int64 { return c.Input + c.CacheRead + c.CacheWrite + c.Output }

// Add adds o to c.
func (c *TokenCounts) Add(o TokenCounts) {
	c.Input += o.Input
	c.CacheRead += o.CacheRead
	c.CacheWrite += o.CacheWrite
	c.Output += o.Output
	c.Reasoning += o.Reasoning
	c.Calls += o.Calls
	c.Cost += o.Cost
	c.Equivalent += o.Equivalent
	c.Unpriced += o.Unpriced
}

// TokensQuery asks for a summary: of [Since, Until) (zero: the last 30
// days, up to now), split By, only what matches the filters given.
type TokensQuery struct {
	Since   time.Time `json:"since,omitzero"`
	Until   time.Time `json:"until,omitzero"`
	By      string    `json:"by,omitempty"`
	Session string    `json:"session,omitempty"`
	Project string    `json:"project,omitempty"`
	Agent   string    `json:"agent,omitempty"`
	Model   string    `json:"model,omitempty"`
	Config  string    `json:"config,omitempty"`
}

// TokensSummary is a host's use over a range: its total and each day's,
// split as asked; Own is enowX's own use (the judge, RAG), apart from the
// agents' and not in Total, split by use in OwnGroups.
type TokensSummary struct {
	Host      string       `json:"host,omitempty"`
	HostID    string       `json:"hostId,omitempty"`
	On        bool         `json:"on"`
	Since     time.Time    `json:"since"`
	Until     time.Time    `json:"until"`
	Total     TokenCounts  `json:"total"`
	Days      []TokenDay   `json:"days,omitempty"`
	Groups    []TokenGroup `json:"groups,omitempty"`
	Own       TokenCounts  `json:"own"`
	OwnGroups []TokenGroup `json:"ownGroups,omitempty"`
	// Budgets are those near or at their limit (80% on).
	Budgets []TokenBudgetState `json:"budgets,omitempty"`
}

// TokenDay is one day's use (Day as 2006-01-02, the host's time zone).
type TokenDay struct {
	Day string `json:"day"`
	TokenCounts
}

// TokenGroup is the use of one agent, model, project, session, config
// or host: Key what was split on, Label how to show it.
type TokenGroup struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	TokenCounts
}

// TokenSessionTotal is a session's use so far, for its badge.
type TokenSessionTotal struct {
	Session string `json:"session"`
	TokenCounts
}

// TokensChanged lists the sessions whose use grew.
type TokensChanged struct {
	Sessions []TokenSessionTotal `json:"sessions,omitempty"`
}

// TokenCoverage is how an agent's tokens are tracked: its layer, what is
// recorded, where, and whether this was checked against a real install
// (else only its documentation).
type TokenCoverage struct {
	Agent   string `json:"agent"`
	Name    string `json:"name"`
	Layer   string `json:"layer"`
	Records string `json:"records,omitempty"`
	Where   string `json:"where,omitempty"`
	Checked bool   `json:"checked,omitempty"`
}

// TokensBackfill reads the records already on disk back to Since (zero:
// the settings' BackfillDays).
type TokensBackfill struct {
	Since time.Time `json:"since,omitzero"`
}

// TokensBackfillResult says how many calls a backfill added.
type TokensBackfillResult struct {
	Calls int `json:"calls"`
	Files int `json:"files"`
}

// TokenPrice is what a model's tokens cost, in US dollars per million:
// Model an ID or a prefix ending in "*"; Provider "" for any; Config a
// model API config's own price ("" for any). Shipped is enowX's, not the
// user's.
type TokenPrice struct {
	Provider   string  `json:"provider,omitempty"`
	Model      string  `json:"model"`
	Config     string  `json:"config,omitempty"`
	Input      float64 `json:"input"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Output     float64 `json:"output"`
	Shipped    bool    `json:"shipped,omitempty"`
}

// Budget periods, scopes and what happens at 100%.
const (
	BudgetDay   = "day"
	BudgetWeek  = "week"
	BudgetMonth = "month"

	BudgetAll     = "all"
	BudgetAgent   = "agent"
	BudgetConfig  = "config"
	BudgetProject = "project"

	BudgetNotify = "notify" // a notification only
	BudgetAsk    = "ask"    // new sessions and prompts sent through enowX ask first
	BudgetStop   = "stop"   // new agent sessions refused
)

// How session.create's error starts when a budget at its limit holds a
// new agent session: asking first (the app asks, then sends
// TokenBudgetOK), or stopping it.
const (
	TokenBudgetAsks  = "token budget reached, ask first:"
	TokenBudgetStops = "token budget reached:"
)

// TokenBudget caps use over a period, for everything or one agent,
// config or project (Target): in tokens, or cost in dollars, whichever is
// set (both: the first reached).
type TokenBudget struct {
	ID      string  `json:"id"`
	Period  string  `json:"period"`
	Scope   string  `json:"scope"`
	Target  string  `json:"target,omitempty"`
	Tokens  int64   `json:"tokens,omitempty"`
	Cost    float64 `json:"cost,omitempty"`
	AtLimit string  `json:"atLimit,omitempty"` // BudgetNotify when ""
}

// TokenBudgetState is a budget and how much of it is used: Used since
// Since (the period's start), Fraction of the limit, Level 0, 80 or 100.
type TokenBudgetState struct {
	TokenBudget
	Since    time.Time   `json:"since"`
	Used     TokenCounts `json:"used"`
	Fraction float64     `json:"fraction"`
	Level    int         `json:"level"`
}

// TokensSettings are the user's (synced): tracking on, its layers
// (Traffic meters agents that keep nothing through the interceptor;
// Screen reads those that only show it), how far back turning it on
// reads, prices over the shipped ones, and budgets.
type TokensSettings struct {
	On           bool          `json:"on,omitempty"`
	Traffic      bool          `json:"traffic,omitempty"`
	Screen       bool          `json:"screen,omitempty"`
	BackfillDays int           `json:"backfillDays,omitempty"` // 0: 30
	Prices       []TokenPrice  `json:"prices,omitempty"`
	Budgets      []TokenBudget `json:"budgets,omitempty"`
}

// TokensHostDays is a host's daily totals as it last synced them, for
// the hosts the app doesn't reach: each day split by agent, model,
// project and config.
type TokensHostDays struct {
	Host    string        `json:"host"`
	Name    string        `json:"name,omitempty"`
	Updated time.Time     `json:"updated"`
	Rows    []TokenDayRow `json:"rows,omitempty"`
}

// TokenDayRow is one day's use of an agent's model in a project.
type TokenDayRow struct {
	Day     string `json:"day"`
	Agent   string `json:"agent,omitempty"`
	Model   string `json:"model,omitempty"`
	Project string `json:"project,omitempty"`
	Config  string `json:"config,omitempty"`
	Own     string `json:"own,omitempty"`
	TokenCounts
}
