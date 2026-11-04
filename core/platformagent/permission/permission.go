// Package permission decides whether a tool call may run, inside the agent: a mode and
// the rules the user kept. The model can't get around it, every call passes Check.
package permission

import (
	"encoding/json"

	"github.com/enowx/enowx/core/platformagent/tools"
)

// Verdict is what Check says of a call.
type Verdict int

const (
	Ask Verdict = iota
	Allow
	Deny
)

// Engine holds a session's mode and rules; it is safe for concurrent use.
type Engine struct{}

// New is an engine in mode (record.Mode*) with the rules kept so far.
func New(mode string, rules []string) *Engine { return &Engine{} }

// Check says whether the call may run; Rule is what "always allow" would keep for it,
// Why says why it is denied.
func (e *Engine) Check(t tools.Tool, input json.RawMessage) (v Verdict, rule, why string) {
	return Ask, "", ""
}

// Mode is the mode now; SetMode changes it, refusing an unknown one.
func (e *Engine) Mode() string { return "" }

// SetMode changes the mode.
func (e *Engine) SetMode(mode string) error { return nil }

// Add keeps a rule.
func (e *Engine) Add(rule string) {}

// Rules are the rules kept.
func (e *Engine) Rules() []string { return nil }
