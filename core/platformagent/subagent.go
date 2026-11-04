package platformagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/enowx/enowx/core/platformagent/tools"
)

const (
	taskToolName = "Task"
	askToolName  = "AskUserQuestion"
)

// taskTool runs a subagent: a nested loop with a history of its own. It passes for
// read-only because what it does passes permission and hooks call by call, so it is not
// asked about as a whole and several can run side by side.
type taskTool struct{ a *Agent }

func (*taskTool) Name() string { return taskToolName }

func (*taskTool) Description() string {
	return "Hand a self-contained task to a subagent with its own context, read and search tools and Bash, and get its final report back. Use it for investigations that would fill this conversation with reading, and to run several at once by calling it several times in one answer. It starts knowing nothing of this conversation, so the prompt must say all it needs."
}

func (*taskTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"description":{"type":"string","description":"What the task is, in a few words, for the user."},"prompt":{"type":"string","description":"The task in full: everything the subagent needs to know, and what to report back."}},"required":["description","prompt"],"additionalProperties":false}`)
}

func (*taskTool) ReadOnly() bool { return true }
func (*taskTool) Edits() bool    { return false }

func (*taskTool) Detail(input json.RawMessage) string {
	var in struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal(input, &in)
	return in.Description
}

func (task *taskTool) Run(ctx context.Context, input json.RawMessage, _ tools.Env) tools.Result {
	var in struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{Text: "Invalid input: " + err.Error(), Failed: true}
	}
	a := task.a

	sub := &thread{a: a, sidechain: callOf(ctx).ID}
	sub.setup(subagentTools(a.all), systemJSON(subagentPrompt))
	msg, files := sub.userMessage(in.Prompt, "", nil)
	if msg == nil {
		return tools.Result{Text: "The task was empty.", Failed: true}
	}
	sub.appendUser(msg, in.Prompt, files)

	stop, text, err := sub.run(ctx)
	switch {
	case err != nil:
		return tools.Result{Text: "The subagent failed: " + a.scrub(err.Error()), Failed: true}
	case stop != stopDone:
		return tools.Result{Text: fmt.Sprintf("The subagent stopped early (%s).", stop), Failed: true}
	}
	return tools.Result{Text: text}
}

// subagentTools are what a subagent may use: what only reads, and Bash; no nested Task,
// and no asking the user.
func subagentTools(all []tools.Tool) []tools.Tool {
	var out []tools.Tool
	for _, t := range all {
		if t == nil {
			continue
		}
		name := t.Name()
		if name == taskToolName || name == askToolName {
			continue
		}
		if tools.RunsSideBySide(t) || name == "Bash" {
			out = append(out, t)
		}
	}
	return out
}
