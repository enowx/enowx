package proto

import "time"

// An agent's accounts on a host: the agent signed in with each in a home
// of its own, picked per session, so switching needs no new sign-in.
// Agents that keep their sign-in in a folder a variable names (Claude
// Code, Codex) can have them; the default account is the agent's own.
const (
	MethodAccountList   = "account.list"   // → []Account
	MethodAccountAdd    = "account.add"    // AccountAddParams → Account
	MethodAccountRemove = "account.remove" // AccountRef
	// Several accounts of an agent, in an order: their state and what
	// they have left, the preferences (synced), switching a session's,
	// and what was used when (see docs/accounts.md).
	MethodAccountInfo     = "account.info"     // → AccountsInfo
	MethodAccountSetPrefs = "account.setPrefs" // AccountPrefsParams → AccountsInfo
	MethodAccountSwitch   = "account.switch"   // AccountSwitchParams → SessionInfo
	MethodAccountLog      = "account.log"      // → []AccountLogEntry, the latest first
	// EventAccountFallback says a session's account hit its limit, ran out
	// of credits or was signed out: what it was switched to, or asks.
	EventAccountFallback = "account.fallback" // AccountFallback
)

// How a limit is told apart (AccountStatus.State, AccountFallback.Reason).
const (
	AccountOK      = "ok"
	AccountLimit   = "limit"   // a usage limit reached, until it resets
	AccountCredits = "credits" // out of credits
	AccountAuth    = "auth"    // signed out, a key refused
)

// Fallback modes (AccountPrefs.Fallback).
const (
	FallbackOff  = ""
	FallbackAuto = "auto" // switch at once and go on
	FallbackAsk  = "ask"  // ask the user first
)

// Account is one of an agent's accounts.
type Account struct {
	ID    string `json:"id"`
	Agent string `json:"agent"` // the agent it signs in (its base: claude for Claude Agent Teams)
	Name  string `json:"name"`
	// Member is the cloud account of the team's member it is for, on a
	// host they visit: its home links nothing of the host's user's.
	Member string `json:"member,omitempty"`
	// Cred, for an account that is an API key, names the vault's
	// credential lent to its sessions; it has no home of its own.
	Cred string `json:"cred,omitempty"`
	// An account is one of an agent's configs (proto/config.go): Kind
	// says which, API is a model API config's, the rest any config's.
	Kind   string     `json:"kind,omitempty"` // ConfigKindAccount ("") or ConfigKindAPI
	API    *ConfigAPI `json:"api,omitempty"`
	Effort string     `json:"effort,omitempty"` // the agent's reasoning effort flag's value
	Env    []string   `json:"env,omitempty"`    // KEY=VALUE added to its sessions
	Args   []string   `json:"args,omitempty"`   // added to the agent's command line
	Icon   string     `json:"icon,omitempty"`   // its badge: an icon name (ui/icon)
	Color  string     `json:"color,omitempty"`  // and a color, #rrggbb
}

// AccountAddParams name a new account of Agent: a sign-in of its own, or
// with Cred the vault's credential (an API key) its sessions get.
type AccountAddParams struct {
	Agent string `json:"agent"`
	Name  string `json:"name"`
	Cred  string `json:"cred,omitempty"`
}

// AccountPrefs are how an agent's accounts are used, kept by account
// name so they sync to every host ("" is the agent's own account).
type AccountPrefs struct {
	// Order is the accounts by priority, the first used first.
	Order []string `json:"order,omitempty"`
	// Fallback says what a limit does: nothing, switch, or ask.
	Fallback string `json:"fallback,omitempty"`
	// ToAgent is the agent to go on with once every account is spent ("" none).
	ToAgent string `json:"toAgent,omitempty"`
	// Return goes back to the first account once its limit reset.
	Return bool `json:"return,omitempty"`
	// Default is the account new sessions use; Projects sets it per
	// project folder (or name).
	Default  string            `json:"default,omitempty"`
	Projects map[string]string `json:"projects,omitempty"`
	// Only keeps an account to these projects, by account name.
	Only map[string][]string `json:"only,omitempty"`
}

// AccountPrefsParams sets Agent's preferences.
type AccountPrefsParams struct {
	Agent string       `json:"agent"`
	Prefs AccountPrefs `json:"prefs"`
}

// AccountStatus is an account's state: its limit, until when, the line
// that said so, and what it has left when its provider tells.
type AccountStatus struct {
	Account
	Own     bool           `json:"own,omitempty"` // the agent's own account
	State   string         `json:"state"`
	Until   time.Time      `json:"until,omitzero"`
	Line    string         `json:"line,omitempty"`
	Credits *CreditAccount `json:"credits,omitempty"`
}

// AccountsInfo is the host's agents' accounts and their preferences, by
// agent base ID.
type AccountsInfo struct {
	Accounts []AccountStatus         `json:"accounts"`
	Prefs    map[string]AccountPrefs `json:"prefs,omitempty"`
}

// AccountSwitchParams has session Session go on as Account ("" its
// agent's own), or as another Agent; Continue tells it to go on with what
// it was doing.
type AccountSwitchParams struct {
	Session  string `json:"session"`
	Account  string `json:"account"`
	Agent    string `json:"agent,omitempty"`
	Continue bool   `json:"continue,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// AccountFallback is a session's account hitting its limit (Reason, the
// Line said, Until when it resets): switched to To (or ToAgent), or, with
// Ask, waiting for the user; neither when nothing is left to switch to.
type AccountFallback struct {
	Session  string    `json:"session"`
	Agent    string    `json:"agent"`
	From     string    `json:"from"`
	To       string    `json:"to,omitempty"`
	ToID     string    `json:"toId,omitempty"`
	ToAgent  string    `json:"toAgent,omitempty"`
	Reason   string    `json:"reason"`
	Line     string    `json:"line,omitempty"`
	Until    time.Time `json:"until,omitzero"`
	Ask      bool      `json:"ask,omitempty"`
	Switched bool      `json:"switched,omitempty"`
	Back     bool      `json:"back,omitempty"` // back on the first, its limit reset
}

// AccountLogEntry is an account used: a session started on it, switched
// to it, fell back from it, and why.
type AccountLogEntry struct {
	Time    time.Time `json:"time"`
	Session string    `json:"session,omitempty"`
	Agent   string    `json:"agent"`
	Account string    `json:"account"`
	Event   string    `json:"event"` // start, switch, fallback, back, limit
	Reason  string    `json:"reason,omitempty"`
}

// AccountRef names an account.
type AccountRef struct {
	ID string `json:"id"`
}
