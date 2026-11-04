package standinapi

import "encoding/json"

// Reply is what the server does with one request.
type Reply struct {
	// Status, when not 0, answers with that HTTP error; RetryAfter is its header,
	// Message its error text.
	Status     int
	RetryAfter string
	Message    string

	ID, Model string // default to a counter's and the request's
	Blocks    []Block
	// Stop is the stop_reason; by default "tool_use" when a block is a tool call and
	// "end_turn" otherwise.
	Stop        string
	StopDetails json.RawMessage
	Usage       Usage

	// A streamed reply can fail on the way. Drop aborts the connection after DropAfter
	// whole blocks and half of the next, with no error event; StreamError sends an SSE
	// error event of that type there instead.
	Drop        bool
	StreamError string
	DropAfter   int

	// Hold, when not nil, stops the stream after HoldAfter whole blocks and half of the
	// next, until it closes or the client goes away.
	Hold      <-chan struct{}
	HoldAfter int
}

func (r Reply) stop() string {
	if r.Stop != "" {
		return r.Stop
	}
	for _, b := range r.Blocks {
		if b.Type == "tool_use" {
			return "tool_use"
		}
	}
	return "end_turn"
}

func (r Reply) outputTokens() int64 {
	if r.Usage.Output != 0 {
		return r.Usage.Output
	}
	var n int
	for _, b := range r.Blocks {
		n += len(b.Text) + len(b.Input)
	}
	return int64(n/4 + 1)
}

// Usage are a reply's token counts, as the API counts them.
type Usage struct {
	Input, Output, CacheRead, CacheWrite int64
}

func (u Usage) fields(output int64) map[string]any {
	return map[string]any{
		"input_tokens":                u.Input,
		"output_tokens":               output,
		"cache_read_input_tokens":     u.CacheRead,
		"cache_creation_input_tokens": u.CacheWrite,
	}
}

// Block is a content block of a reply: "text" (Text), "thinking" (Text, Signature),
// "redacted_thinking" (Text is its data), "tool_use" (ID, Name, Input, raw JSON sent as
// it stands, broken if a test wants it broken). Another Type is sent as Raw, whole.
type Block struct {
	Type      string
	Text      string
	Signature string
	ID, Name  string
	Input     string
	Raw       json.RawMessage
}

// Text is a reply of one text block.
func Text(s string) Reply { return Reply{Blocks: []Block{TextBlock(s)}} }

// TextBlock is a text block.
func TextBlock(s string) Block { return Block{Type: "text", Text: s} }

// Thinking is a thinking block with its signature.
func Thinking(s, signature string) Block {
	return Block{Type: "thinking", Text: s, Signature: signature}
}

// ToolUse is a tool call block with input as raw JSON.
func ToolUse(id, name, input string) Block {
	return Block{Type: "tool_use", ID: id, Name: name, Input: input}
}

// Calls is a reply of tool calls.
func Calls(blocks ...Block) Reply { return Reply{Blocks: blocks} }

// Err is a reply of an HTTP error.
func Err(status int) Reply { return Reply{Status: status} }

// RateLimited is a 429 with its retry-after, in seconds.
func RateLimited(retryAfter string) Reply { return Reply{Status: 429, RetryAfter: retryAfter} }

// Refusal is a reply the model's safeguards stopped, before any output or after blocks.
func Refusal(blocks ...Block) Reply {
	return Reply{
		Blocks:      blocks,
		Stop:        "refusal",
		StopDetails: json.RawMessage(`{"type":"refusal","category":"cyber","explanation":"standin refusal"}`),
	}
}

// final is the block as a non-streamed message holds it.
func (b Block) final() map[string]any {
	switch b.Type {
	case "thinking":
		return map[string]any{"type": "thinking", "thinking": b.Text, "signature": b.Signature}
	case "redacted_thinking":
		return map[string]any{"type": "redacted_thinking", "data": b.Text}
	case "tool_use":
		input := json.RawMessage(b.Input)
		if !json.Valid(input) {
			input = json.RawMessage(`{}`)
		}
		return map[string]any{"type": "tool_use", "id": b.ID, "name": b.Name, "input": input}
	}
	if b.Raw != nil {
		return b.rawFields()
	}
	return map[string]any{"type": "text", "text": b.Text}
}

func (b Block) rawFields() map[string]any {
	fields := map[string]any{}
	_ = json.Unmarshal(b.Raw, &fields)
	return fields
}

// Fallback is the block marking a switch to another model.
func Fallback(from, to string) Block {
	raw, _ := json.Marshal(map[string]any{
		"type": "fallback",
		"from": map[string]string{"model": from},
		"to":   map[string]string{"model": to},
	})
	return Block{Type: "fallback", Raw: raw}
}
