package platformagent

import (
	"errors"
	"fmt"

	"github.com/enowx/enowx/core/platformagent/record"
)

// Settings are those now.
func (a *Agent) Settings() (model, effort, mode string, budget float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model, a.effort, a.mode, a.budget
}

// SetModel applies from the next request. The history stays as it is: the API drops,
// unbilled, thinking blocks the new model can't read.
func (a *Agent) SetModel(model string) error {
	if model == "" {
		return errors.New("platformagent: no model")
	}
	a.mu.Lock()
	a.model = model
	a.mu.Unlock()
	a.writeMeta()
	return nil
}

// SetEffort applies from the next request.
func (a *Agent) SetEffort(effort string) error {
	if !validEffort(effort) {
		return fmt.Errorf("platformagent: unknown effort %q", effort)
	}
	a.mu.Lock()
	a.effort = effort
	a.mu.Unlock()
	a.writeMeta()
	return nil
}

// SetMode changes the mode of permission. The prompt stays: a call the mode refuses
// tells the model why.
func (a *Agent) SetMode(mode string) error {
	if err := a.engine.SetMode(mode); err != nil {
		return err
	}
	a.mu.Lock()
	a.mode = mode
	a.mu.Unlock()
	a.writeMeta()
	return nil
}

// SetBudget changes the budget, 0 for none.
func (a *Agent) SetBudget(budget float64) {
	a.mu.Lock()
	a.budget, a.limit = budget, budget
	a.mu.Unlock()
	a.writeMeta()
}

// Clear starts a new conversation, in a file of its own; rules and money carry over.
func (a *Agent) Clear() error {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	id, err := newConversationID()
	if err != nil {
		return err
	}
	file, err := openConvFile(conversationPath(a.opts.SessionDir, id))
	if err != nil {
		return err
	}
	a.mu.Lock()
	old := a.file
	a.conv, a.file, a.used = id, file, 0
	a.mu.Unlock()
	if old != nil {
		_ = old.close()
	}

	a.main.history, a.main.stripBefore, a.main.used, a.main.lastPrompt = nil, 0, 0, ""
	a.writeMeta()
	for _, rule := range a.engine.Rules() {
		a.emit(record.Record{Type: record.Allow, Rule: rule})
	}
	return nil
}

// Cost is the session's dollars so far.
func (a *Agent) Cost() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.total
}

// Context is the tokens the conversation fills and the model's window.
func (a *Agent) Context() (tokens, window int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.used, specOf(a.model).window
}

// Conversation is the conversation's ID.
func (a *Agent) Conversation() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.conv
}

// Close ends the MCP servers and closes the file. Turn must have returned.
func (a *Agent) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	file := a.file
	a.mu.Unlock()

	var err error
	if a.mcp != nil {
		err = a.mcp.Close()
	}
	if file != nil {
		err = errors.Join(err, file.close())
	}
	return err
}
