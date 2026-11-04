package platformagent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/enowx/enowx/core/platformagent/permission"
	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/proto"
)

const (
	// chatResultLimit is how much of a result the chat's record holds.
	chatResultLimit = 8 << 10
	// killGrace is how long a tool gets to stop after an interruption before the loop
	// goes on without it.
	killGrace = 5 * time.Second

	interruptedText = "interrupted"
)

// setup freezes what the thread sends before its messages. Nothing changes these bytes
// for as long as the thread lives: the prompt cache and the API's check of thinking
// blocks both depend on it.
func (t *thread) setup(list []tools.Tool, system json.RawMessage) {
	t.toolsByName = map[string]tools.Tool{}
	for _, tool := range list {
		if tool != nil {
			t.toolsByName[tool.Name()] = tool
		}
	}
	names := make([]string, 0, len(t.toolsByName))
	for name := range t.toolsByName {
		names = append(names, name)
	}
	slices.Sort(names)

	type toolJSON struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
		// The loop checks inputs itself (validate.go), so it can take them unbuffered.
		Eager bool `json:"eager_input_streaming,omitempty"`
	}
	out := make([]toolJSON, 0, len(names))
	for _, name := range names {
		tool := t.toolsByName[name]
		schema := tool.Schema()
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		out = append(out, toolJSON{name, tool.Description(), schema, t.a.firstParty})
	}
	t.toolsJSON = marshalStable(out)
	t.systemJSON = system
}

// runCalls runs one answer's calls and returns their tool_result blocks, in the calls'
// order. Consecutive read-only calls run side by side, the others alone in order.
func (t *thread) runCalls(ctx context.Context, calls []toolCall) []json.RawMessage {
	results := make([]json.RawMessage, len(calls))
	for i := 0; i < len(calls); {
		end := i + 1
		if t.readOnly(calls[i]) {
			for end < len(calls) && t.readOnly(calls[end]) {
				end++
			}
		}
		var wg sync.WaitGroup
		for j := i; j < end; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[j] = t.runCall(ctx, calls[j])
			}()
		}
		wg.Wait()
		i = end
	}
	return results
}

func (t *thread) readOnly(c toolCall) bool {
	tool, ok := t.toolsByName[c.Name]
	return ok && tool.ReadOnly()
}

// runCall takes one call from ToolStart to ToolEnd.
func (t *thread) runCall(ctx context.Context, c toolCall) json.RawMessage {
	a := t.a
	tool := t.toolsByName[c.Name]
	shown := record.Tool{ID: c.ID, Name: c.Name, Input: c.Input}
	if tool != nil && c.BadJSON == "" {
		shown.Detail = tool.Detail(c.Input)
	}
	a.emit(record.Record{Type: record.ToolStart, Tool: &shown, Sidechain: t.sidechain})

	started := time.Now()
	res := t.execute(ctx, c, tool, shown)

	// The input is on the ToolStart and the Assistant already; a big Write needn't be
	// kept a third time.
	end := shown
	end.Input = nil
	end.Result = clip(res.Text, chatResultLimit)
	end.Failed = res.Failed
	end.DurationMs = time.Since(started).Milliseconds()
	end.Old, end.New, end.Added, end.Removed = res.Old, res.New, res.Added, res.Removed
	a.emit(record.Record{Type: record.ToolEnd, Tool: &end, Sidechain: t.sidechain})
	return toolResultBlock(c.ID, res)
}

// execute is everything a call passes: its input's check, permission and hooks, the run.
func (t *thread) execute(ctx context.Context, c toolCall, tool tools.Tool, shown record.Tool) tools.Result {
	if ctx.Err() != nil {
		return tools.Result{Text: interruptedText, Failed: true}
	}
	if tool == nil {
		return tools.Result{Text: fmt.Sprintf("There is no tool named %s.", c.Name), Failed: true}
	}
	if c.BadJSON != "" {
		// The API's advice: hand the model back what it sent, as JSON, to try again.
		return tools.Result{Text: string(marshalStable(map[string]string{"INVALID_JSON": c.BadJSON})), Failed: true}
	}
	if problem := validateInput(tool.Schema(), c.Input); problem != "" {
		return tools.Result{Text: "Invalid input, the call did not run: " + problem, Failed: true}
	}

	refusal, extra, err := t.gate(ctx, tool, c, shown)
	if err != nil {
		return tools.Result{Text: interruptedText, Failed: true}
	}
	if refusal != "" {
		return tools.Result{Text: withContext(refusal, extra), Failed: true}
	}

	res := t.runTool(ctx, tool, c)
	if t.a.hooks != nil {
		t.a.hooks.ToolAfter(ctx, c.Name, c.Input, res.Failed)
	}
	res.Text = withContext(res.Text, extra)
	return res
}

func withContext(text, extra string) string {
	if extra == "" {
		return text
	}
	if text == "" {
		return extra
	}
	return text + "\n\n" + extra
}

// gate says whether the call may run: the mode and rules first, then hooks, which can
// refuse what any mode allows (this is what enforces GOD of Agents) or spare the asking,
// then the user. refusal is what to tell the model; extra is a hook's text for it.
func (t *thread) gate(ctx context.Context, tool tools.Tool, c toolCall, shown record.Tool) (refusal, extra string, err error) {
	a := t.a
	verdict, rule, why := a.engine.Check(tool, c.Input)

	var hook HookAnswer
	if a.hooks != nil {
		hook = a.hooks.ToolBefore(ctx, c.Name, c.Input)
	}
	extra = hook.Context
	if hook.Deny {
		return first(hook.Why, "A hook refused this call."), extra, nil
	}
	switch verdict {
	case permission.Deny:
		return first(why, "This call isn't permitted."), extra, nil
	case permission.Allow:
		return "", extra, nil
	}
	if hook.Allow {
		return "", extra, nil
	}

	decision, err := a.askPermission(ctx, shown, PermissionRequest{
		Tool: c.Name, Detail: shown.Detail, Rule: first(rule, c.Name),
	}, t.sidechain)
	if err != nil {
		return "", extra, err
	}
	switch decision {
	case AllowAlways:
		a.keepRule(first(rule, c.Name))
	case AllowOnce:
	default:
		return "The user denied this call.", extra, nil
	}
	return "", extra, nil
}

// askPermission puts the question through Front: the wait is recorded first, so the chat
// and the file show it, and the session reads as waiting until it is answered.
func (a *Agent) askPermission(ctx context.Context, shown record.Tool, req PermissionRequest, sidechain string) (Decision, error) {
	release, err := a.takeAskSlot(ctx)
	if err != nil {
		return Deny, err
	}
	defer release()
	a.emit(record.Record{Type: record.ToolWait, Asks: "permission", Tool: &shown, Sidechain: sidechain})
	a.state(proto.StateWaiting)
	defer a.state(proto.StateWorking)
	return a.front.Permission(ctx, req)
}

// takeAskSlot waits its turn to ask the user anything.
func (a *Agent) takeAskSlot(ctx context.Context) (release func(), err error) {
	select {
	case a.askSlot <- struct{}{}:
		return func() { <-a.askSlot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Agent) keepRule(rule string) {
	a.engine.Add(rule)
	a.emit(record.Record{Type: record.Allow, Rule: rule})
}

// callKey carries the running call's ID in a tool's context, for what a tool does
// through the agent (a question, a subagent).
type callKey struct{}

func callOf(ctx context.Context) toolCall {
	c, _ := ctx.Value(callKey{}).(toolCall)
	return c
}

// runTool runs it in a goroutine of its own, so an interruption is answered even if a
// tool doesn't stop in time. Such a tool's goroutine outlives the call, and its result
// is dropped.
func (t *thread) runTool(ctx context.Context, tool tools.Tool, c toolCall) tools.Result {
	a := t.a
	done := make(chan tools.Result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- tools.Result{Text: fmt.Sprintf("The tool crashed: %v", r), Failed: true}
			}
		}()
		env := tools.Env{Dir: a.opts.Dir, Environ: a.environ}
		done <- tool.Run(context.WithValue(ctx, callKey{}, c), c.Input, env)
	}()

	select {
	case res := <-done:
		if ctx.Err() != nil {
			return tools.Result{Text: interruptedText, Failed: true}
		}
		return res
	case <-ctx.Done():
	}
	select {
	case <-done:
	case <-time.After(killGrace):
	}
	return tools.Result{Text: interruptedText, Failed: true}
}

// toolResultBlock is res as the API takes it.
func toolResultBlock(id string, res tools.Result) json.RawMessage {
	var content []map[string]any
	if res.Text != "" {
		content = append(content, map[string]any{"type": "text", "text": res.Text})
	}
	for _, image := range res.Images {
		content = append(content, map[string]any{
			"type":   "image",
			"source": map[string]string{"type": "base64", "media_type": image.MediaType, "data": image.Base64},
		})
	}
	block := struct {
		Type      string           `json:"type"`
		ToolUseID string           `json:"tool_use_id"`
		IsError   bool             `json:"is_error,omitempty"`
		Content   []map[string]any `json:"content,omitempty"`
	}{"tool_result", id, res.Failed, content}
	return marshalStable(block)
}

// asker is the Asker AskUserQuestion takes: its questions go through Front, the wait on
// record under the call that asks.
type asker struct{ a *Agent }

func (q asker) Questions(ctx context.Context, qs []record.Question) ([]string, error) {
	a := q.a
	c := callOf(ctx)
	release, err := a.takeAskSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	shown := record.Tool{ID: c.ID, Name: c.Name, Input: c.Input, Questions: qs}
	a.emit(record.Record{Type: record.ToolWait, Asks: "questions", Tool: &shown})
	a.state(proto.StateWaiting)
	defer a.state(proto.StateWorking)
	return a.front.Questions(ctx, qs)
}
