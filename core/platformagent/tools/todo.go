package tools

import (
	"context"
	"encoding/json"
	"strings"
)

const todoSchema = `{"type":"object","properties":{"todos":{"type":"array","description":"The whole list as it stands now","items":{"type":"object","properties":{"content":{"type":"string","description":"The step"},"status":{"type":"string","enum":["pending","in_progress","completed"]},"activeForm":{"type":"string","description":"The step as under way, such as Running tests"}},"required":["content","status"],"additionalProperties":false}}},"required":["todos"],"additionalProperties":false}`

type todoTool struct{}

// NewTodoWrite is the to-do list tool, optional beside Builtin: it keeps nothing, the
// list lives in the conversation and the chat reads it from the call.
func NewTodoWrite() Tool { return todoTool{} }

func (todoTool) Name() string { return "TodoWrite" }
func (todoTool) Description() string {
	return "Keep a list of the steps of a longer task: send the whole list each time something changes, with one step in_progress."
}
func (todoTool) Schema() json.RawMessage { return json.RawMessage(todoSchema) }
func (todoTool) ReadOnly() bool          { return true }
func (todoTool) Edits() bool             { return false }

type todoInput struct {
	Todos []struct {
		Content string `json:"content"`
		Status  string `json:"status"`
	} `json:"todos"`
}

func (todoTool) Detail(input json.RawMessage) string {
	in, err := decode[todoInput](input)
	if err != nil {
		return "TodoWrite"
	}
	for _, t := range in.Todos {
		if t.Status == "in_progress" {
			return oneLine(t.Content, 200)
		}
	}
	return "Update the to-do list"
}

func (todoTool) Run(_ context.Context, input json.RawMessage, _ Env) Result {
	in, err := decode[todoInput](input)
	if err != nil {
		return failf("%v", err)
	}
	var out strings.Builder
	for _, t := range in.Todos {
		out.WriteString("[" + t.Status + "] " + t.Content + "\n")
	}
	if out.Len() == 0 {
		return Result{Text: "The list is empty"}
	}
	return Result{Text: out.String()}
}
