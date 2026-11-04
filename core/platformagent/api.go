package platformagent

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"
)

// maxTries is how many times one request is tried in all.
const maxTries = 6

// maxRetryWait is the longest a retry-after is waited out: a turn that sits silent for
// longer reads as hung, and a proxy can say anything.
const maxRetryWait = 10 * time.Minute

// message is one of the API's messages as kept: its bytes, replayed unchanged.
type message struct {
	role string
	raw  json.RawMessage
}

// requestBody is one request. Field order is fixed, and tools and system are the frozen
// bytes of setup, so the prompt's prefix stays the same from request to request.
type requestBody struct {
	Model        string            `json:"model"`
	MaxTokens    int64             `json:"max_tokens"`
	Stream       bool              `json:"stream"`
	CacheControl json.RawMessage   `json:"cache_control,omitempty"`
	Thinking     json.RawMessage   `json:"thinking,omitempty"`
	OutputConfig json.RawMessage   `json:"output_config,omitempty"`
	Fallbacks    string            `json:"fallbacks,omitempty"`
	ToolChoice   json.RawMessage   `json:"tool_choice,omitempty"`
	Tools        json.RawMessage   `json:"tools,omitempty"`
	System       json.RawMessage   `json:"system,omitempty"`
	Messages     []json.RawMessage `json:"messages"`
}

// callOptions shape a request apart from the thread's own history.
type callOptions struct {
	// extra is appended to the history for this request alone.
	extra []message
	// noTools forbids tool calls.
	noTools bool
	// quiet keeps the answer's blocks out of the file: a summary isn't for the chat.
	quiet bool
}

// call asks the model, trying again what can pass, and says what happened through
// Notices. A partial answer is gone by the time it tries again: only a whole one comes
// back.
func (t *thread) call(ctx context.Context, o callOptions) (*answer, error) {
	a := t.a
	recovered := false
	for try := 1; ; try++ {
		body, betas := t.buildRequest(o)
		ans, err := t.streamOnce(ctx, body, betas, o.quiet)
		if err == nil {
			return ans, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		failure := classify(err)
		// History the API won't take back is a one-time repair, not a try.
		if failure.boundThinking && !recovered {
			recovered = true
			t.stripBefore = len(t.history)
			a.notice("warning", "The conversation's earlier reasoning no longer matches its beginning; going on without it.")
			try--
			continue
		}
		if !failure.retryable {
			return nil, failure.final(a.scrub)
		}
		if try >= maxTries {
			return nil, fmt.Errorf("%s: gave up after %d tries", a.scrub(failure.describe), maxTries)
		}
		wait := failure.retryAfter
		if wait > maxRetryWait {
			return nil, fmt.Errorf("%s: the API asks to wait %s, longer than this waits (%s): try again later", a.scrub(failure.describe), roundWait(wait), maxRetryWait)
		}
		if wait <= 0 {
			wait = a.backoffFor(try)
		}
		a.notice("warning", fmt.Sprintf("%s, retrying in %s (%d of %d)", failure.describe, roundWait(wait), try+1, maxTries))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// backoffFor doubles with each try, up to a minute, with some jitter so that a host's
// sessions don't come back together.
func (a *Agent) backoffFor(try int) time.Duration {
	wait := a.backoff << (try - 1)
	if limit := time.Minute; wait > limit || wait <= 0 {
		wait = limit
	}
	return wait/2 + time.Duration(rand.Int64N(int64(wait/2)+1))
}

func roundWait(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}

// buildRequest is the body of the next request, and its beta headers.
func (t *thread) buildRequest(o callOptions) ([]byte, []string) {
	a := t.a
	a.mu.Lock()
	model, effort := a.model, a.effort
	a.mu.Unlock()
	spec := specOf(model)

	body := requestBody{
		Model:        model,
		MaxTokens:    spec.maxTokens,
		Stream:       true,
		CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
		Thinking:     json.RawMessage(spec.thinking),
		OutputConfig: spec.outputConfig(effort),
		Tools:        t.toolsJSON,
		System:       t.systemJSON,
		Messages:     t.requestMessages(o.extra),
	}
	if o.noTools {
		body.ToolChoice = json.RawMessage(`{"type":"none"}`)
	}
	var betas []string
	if a.firstParty && spec.fallback {
		body.Fallbacks = "default"
		betas = append(betas, "server-side-fallback-2026-07-01")
	}
	return marshalStable(body), betas
}

// requestMessages is the history as the API takes it. Interruptions and compaction can
// leave two user messages in a row, which the API wants as one; the merge is the same
// every time, so it doesn't move the cached prefix. Nothing here changes what is kept.
func (t *thread) requestMessages(extra []message) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(t.history)+len(extra))
	lastRole := ""
	for i, m := range slices.Concat(t.history, extra) {
		raw := m.raw
		if i < t.stripBefore && m.role == "assistant" {
			var ok bool
			if raw, ok = stripThinking(raw); !ok {
				continue
			}
		}
		if m.role == "user" && lastRole == "user" {
			out[len(out)-1] = mergeUser(out[len(out)-1], raw)
			continue
		}
		out = append(out, raw)
		lastRole = m.role
	}
	return out
}

type contentOnly struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

func mergeUser(a, b json.RawMessage) json.RawMessage {
	var first, second contentOnly
	if json.Unmarshal(a, &first) != nil || json.Unmarshal(b, &second) != nil {
		return a
	}
	first.Content = append(first.Content, second.Content...)
	return marshalStable(first)
}

// stripThinking takes an answer's thinking blocks out, false when nothing is left.
func stripThinking(raw json.RawMessage) (json.RawMessage, bool) {
	var m contentOnly
	if json.Unmarshal(raw, &m) != nil {
		return raw, true
	}
	kept := m.Content[:0:0]
	for _, block := range m.Content {
		var b struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(block, &b)
		if b.Type != "thinking" && b.Type != "redacted_thinking" {
			kept = append(kept, block)
		}
	}
	if len(kept) == 0 {
		return nil, false
	}
	m.Content = kept
	return marshalStable(m), true
}
