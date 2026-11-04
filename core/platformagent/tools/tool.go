// Package tools are Claude Platform's own tools. They carry Claude Code's names (Read,
// Write, Edit, Glob, Grep, Bash, ...) so that enowX's GOA rules, hooks and the chat's
// summaries, which go by those names, hold for them without a table of their own.
package tools

import (
	"context"
	"encoding/json"

	"github.com/enowx/enowx/core/platformagent/record"
)

// Image is a picture a tool returns to the model.
type Image struct {
	MediaType string
	Base64    string
}

// Result is what a tool call came to.
type Result struct {
	// Text goes to the model; Failed makes it an is_error result.
	Text   string
	Images []Image
	Failed bool
	// Old, New, Added, Removed describe a file edit, for the chat's diff.
	Old, New       string
	Added, Removed int
}

// Env is what a tool runs with.
type Env struct {
	// Dir is the session's working folder; relative paths are under it.
	Dir string
	// Environ is the environment of programs a tool starts, already without the API
	// key (the loop removes ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN and the rest of
	// CleanEnviron's list).
	Environ []string
	// Progress, when set, receives output as it comes (Bash).
	Progress func(string)
}

// Tool is one the model can call. Implementations are safe for concurrent use: the
// loop runs a read-only answer's calls side by side.
type Tool interface {
	Name() string
	Description() string
	// Schema is the JSON schema of its input.
	Schema() json.RawMessage
	// ReadOnly says it changes nothing: plan mode allows it, default mode asks nothing.
	ReadOnly() bool
	// Edits says it only changes files in the working folder: acceptEdits allows it.
	Edits() bool
	// Detail is one line saying what a call with input would do.
	Detail(input json.RawMessage) string
	// Run does the call; ctx ends it when the user interrupts, killing what it started.
	Run(ctx context.Context, input json.RawMessage, env Env) Result
}

// Asker answers what only the user can: the AskUserQuestion tool takes it.
type Asker interface {
	// Questions asks them and returns one answer each.
	Questions(ctx context.Context, qs []record.Question) ([]string, error)
}
