// Package tokens tracks the tokens agents use on a host: one record per
// model call (only counts, never prompts or answers), read from the
// agents' own records by an adapter each, metered from their model API's
// answers through the interceptor, or read from their screen; kept a file
// a day, deduplicated by the call's ID, priced from an editable table,
// summed per day, agent, model, project, session, config and host, and
// held against the user's budgets. See docs/tokens.md.
package tokens

import (
	"time"

	"github.com/enowx/enowx/proto"
)

// Call is one model call's counts and where they came from. Input is
// what wasn't read from a cache; Output includes Reasoning.
type Call struct {
	// ID tells the call apart, so that records read again count once:
	// the agent's own message or response ID, prefixed by the adapter.
	ID   string    `json:"id"`
	Time time.Time `json:"t"`

	Project string `json:"project,omitempty"` // its folder
	Session string `json:"session,omitempty"` // enowX's session, "" when run outside it
	Agent   string `json:"agent,omitempty"`   // preset ID
	Account string `json:"account,omitempty"`
	// Config is the session's agent config (its model API and settings),
	// "" for none.
	Config   string `json:"config,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Layer    string `json:"layer"`
	// Own is enowX's own use (proto.TokensOwn*), counted apart from the
	// agents'.
	Own string `json:"own,omitempty"`
	// Dir is where the agent ran, Conversation its own ID for the
	// conversation: what ties a call to a session.
	Dir          string `json:"dir,omitempty"`
	Conversation string `json:"conv,omitempty"`
	// Plan: the agent ran on a subscription, its cost API-equivalent.
	Plan bool `json:"plan,omitempty"`

	Input      int64 `json:"in,omitempty"`
	CacheRead  int64 `json:"cr,omitempty"`
	CacheWrite int64 `json:"cw,omitempty"`
	Output     int64 `json:"out,omitempty"`
	Reasoning  int64 `json:"rs,omitempty"`
	// Cost is what the agent itself said the call cost, 0 when it didn't.
	Cost float64 `json:"cost,omitempty"`
}

// Empty reports whether the call counted no tokens.
func (c Call) Empty() bool { return c.Input+c.CacheRead+c.CacheWrite+c.Output == 0 }

// Counts are the call's tokens, priced by prices.
func (c Call) Counts(prices *Prices) proto.TokenCounts {
	out := proto.TokenCounts{Input: c.Input, CacheRead: c.CacheRead, CacheWrite: c.CacheWrite, Output: c.Output, Reasoning: c.Reasoning, Calls: 1}
	cost, ok := c.Cost, c.Cost > 0
	if !ok && prices != nil {
		cost, ok = prices.Cost(c)
	}
	if !ok {
		out.Unpriced = 1
	}
	out.Cost = cost
	if c.Plan {
		out.Equivalent = cost
	}
	return out
}

// day is the file a call goes in: its local date.
func day(t time.Time) string { return t.Local().Format(time.DateOnly) }
