// Package permission decides whether a tool call may run, inside the agent: a mode and
// the rules the user kept. The model can't get around it, every call passes Check.
package permission

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
)

// Verdict is what Check says of a call.
type Verdict int

const (
	Ask Verdict = iota
	Allow
	Deny
)

const bashTool = "Bash"

// Engine holds a session's mode and rules; it is safe for concurrent use. Its mutex
// guards the fields alone: Check reads the disk outside it.
type Engine struct {
	mu    sync.Mutex
	mode  string
	rules []string
	// sessionRules allow for this run alone: Rules, what goes on record, leaves them out.
	sessionRules []string
	dir          string
}

// New is an engine in mode (record.Mode*) with the rules kept so far. An unknown mode is
// the default one, which asks.
func New(mode string, rules []string) *Engine {
	if !knownMode(mode) {
		mode = record.ModeDefault
	}
	return &Engine{mode: mode, rules: slices.Clone(rules)}
}

func knownMode(mode string) bool {
	switch mode {
	case record.ModeDefault, record.ModeAcceptEdits, record.ModePlan, record.ModeBypass:
		return true
	}
	return false
}

// snapshot is the fields as they are now.
func (e *Engine) snapshot() (mode string, rules []string, dir string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mode, slices.Concat(e.rules, e.sessionRules), e.dir
}

// Check says whether the call may run. On Ask, rule is what "always allow" would keep
// for it, "" when nothing narrow enough would cover it (the front then offers only
// this once); on Deny, why says why.
func (e *Engine) Check(t tools.Tool, input json.RawMessage) (v Verdict, rule, why string) {
	mode, rules, dir := e.snapshot()
	switch mode {
	case record.ModeBypass:
		return Allow, "", ""
	}
	if t.ReadOnly() {
		if !readsInside(t, dir, input) {
			return Ask, "", "" // this once: a kept rule would open the whole disk
		}
		return Allow, "", ""
	}
	if mode == record.ModePlan {
		if strings.HasPrefix(t.Name(), "mcp__") {
			return Deny, "", "plan mode: reads only, and nothing says an MCP tool only reads"
		}
		return Deny, "", "plan mode: reads only"
	}
	if mode == record.ModeAcceptEdits && t.Edits() && editInside(dir, input) {
		return Allow, "", ""
	}
	if ruleAllows(rules, t, dir, input) {
		return Allow, "", ""
	}
	return Ask, suggestion(t, dir, input), ""
}

// Mode is the mode now.
func (e *Engine) Mode() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mode
}

// SetMode changes the mode, refusing an unknown one.
func (e *Engine) SetMode(mode string) error {
	if !knownMode(mode) {
		return fmt.Errorf("permission: unknown mode %q", mode)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mode = mode
	return nil
}

// SetDir says the session's working folder, which acceptEdits allows edits inside: the
// tools' Env.Dir. Without it every edit asks.
func (e *Engine) SetDir(dir string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dir = dir
}

// Add keeps a rule.
func (e *Engine) Add(rule string) {
	if rule == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !slices.Contains(e.rules, rule) {
		e.rules = append(e.rules, rule)
	}
}

// AddForSession allows by rule for this run alone, never on record.
func (e *Engine) AddForSession(rule string) {
	if rule == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !slices.Contains(e.sessionRules, rule) {
		e.sessionRules = append(e.sessionRules, rule)
	}
}

// Rules are the rules kept, which a new conversation's file carries on.
func (e *Engine) Rules() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.rules)
}

// commandOf is a Bash call's command, "" when it has none.
func commandOf(input json.RawMessage) string {
	var in struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	return in.Command
}

// ruleAllows says a kept rule covers the call: its tool's name, or for Bash patterns
// that between them cover every part of the command. A file tool's name holds inside the
// working folder alone: "always" on one edit mustn't cover .git/hooks or ~/.ssh.
func ruleAllows(rules []string, t tools.Tool, dir string, input json.RawMessage) bool {
	if slices.Contains(rules, t.Name()) {
		return !t.Edits() || editInside(dir, input)
	}
	if t.Name() != bashTool {
		return false
	}
	return bashRulesCover(rules, commandOf(input))
}

// suggestion is the rule "always allow" would keep for a call, "" when one wouldn't hold.
func suggestion(t tools.Tool, dir string, input json.RawMessage) string {
	if t.Name() == bashTool {
		return suggestBashRule(commandOf(input))
	}
	if t.Edits() && !editInside(dir, input) {
		return ""
	}
	return t.Name()
}

// readsInside says a read-only call stays in dir: a tool naming no paths does.
func readsInside(t tools.Tool, dir string, input json.RawMessage) bool {
	reader, ok := t.(tools.PathReader)
	if !ok {
		return true
	}
	if dir == "" {
		return false
	}
	for _, p := range reader.ReadPaths(input, dir) {
		if _, inside := relInside(dir, p); !inside {
			return false
		}
	}
	return true
}
