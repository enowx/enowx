package platformagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/enowx/enowx/core/platformagent/record"
)

// coalesceEvery is how often a block's text under way reaches the file, so that it isn't
// a line per token.
const coalesceEvery = 50 * time.Millisecond

// answer is a whole answer of the API's.
type answer struct {
	id    string
	stop  string
	model string
	usage record.Usage
	// raw is the MessageParam to keep, nil when nothing of it may go back.
	raw   json.RawMessage
	calls []toolCall
	text  string
	cost  float64
}

// toolCall is a tool_use block. BadJSON holds the input when it isn't valid JSON, which
// eager input streaming allows; Input is then {}.
type toolCall struct {
	ID, Name string
	Input    json.RawMessage
	BadJSON  string
}

var (
	errStreamCut     = errors.New("the stream ended before the answer did")
	errStreamStalled = errors.New("the stream stopped sending")
)

// streamOnce sends one request and reads its stream to the end. Any failure comes back
// with nothing kept but the BlockStart and Delta records the chat already got.
func (t *thread) streamOnce(ctx context.Context, body []byte, betas []string, quiet bool) (*answer, error) {
	a := t.a
	params := anthropic.BetaMessageNewParams{}
	for _, beta := range betas {
		params.Betas = append(params.Betas, anthropic.AnthropicBeta(beta))
	}
	// A dead connection sends no error: without this, the turn waits until the user
	// interrupts it. The API pings a stream that has nothing to say.
	streamCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	watchdog := time.AfterFunc(a.streamIdle, func() { cancel(errStreamStalled) })
	defer watchdog.Stop()

	stream := a.client.Beta.Messages.NewStreaming(streamCtx, params, option.WithRequestBody("application/json", body))
	defer stream.Close()

	var msg anthropic.BetaMessage
	inputs := map[int]*strings.Builder{}
	deltas := deltaBuffer{emit: func(r record.Record) {
		r.Sidechain = t.sidechain
		if !quiet {
			a.emit(r)
		}
	}}
	stopped := false
	for stream.Next() {
		watchdog.Reset(a.streamIdle)
		ev := stream.Current()
		if err := msg.Accumulate(ev); err != nil {
			t.recordSpend(cutAnswer(&msg))
			return nil, err
		}
		index := int(ev.Index)
		switch ev.Type {
		case "message_stop":
			stopped = true
		case "content_block_start":
			deltas.flush()
			if kind := ev.ContentBlock.Type; kind == "text" || kind == "thinking" {
				deltas.emit(record.Record{Type: record.BlockStart, Msg: msg.ID, Block: index, Kind: kind})
				deltas.open(msg.ID, index, kind)
			}
		case "content_block_delta":
			switch ev.Delta.Type {
			case "text_delta":
				deltas.add(ev.Delta.Text)
			case "thinking_delta":
				deltas.add(ev.Delta.Thinking)
			case "input_json_delta":
				b := inputs[index]
				if b == nil {
					b = &strings.Builder{}
					inputs[index] = b
				}
				b.WriteString(ev.Delta.PartialJSON)
			}
		case "content_block_stop":
			deltas.flush()
		}
	}
	// What a stream said so far is billed like any answer, whole or not.
	if err := stream.Err(); err != nil {
		t.recordSpend(cutAnswer(&msg))
		if errors.Is(context.Cause(streamCtx), errStreamStalled) {
			return nil, errStreamStalled
		}
		return nil, err
	}
	if !stopped {
		t.recordSpend(cutAnswer(&msg))
		return nil, errStreamCut
	}
	return newAnswer(&msg, inputs), nil
}

// usageOf is what msg says it used so far.
func usageOf(msg *anthropic.BetaMessage) record.Usage {
	return record.Usage{
		Input:      msg.Usage.InputTokens,
		Output:     msg.Usage.OutputTokens,
		CacheRead:  msg.Usage.CacheReadInputTokens,
		CacheWrite: msg.Usage.CacheCreationInputTokens,
	}
}

// cutAnswer is an answer cut short, as far as its bill goes.
func cutAnswer(msg *anthropic.BetaMessage) *answer {
	return &answer{id: msg.ID, stop: "cut", model: string(msg.Model), usage: usageOf(msg)}
}

// recordSpend counts an answer no other record will, with its cost and tokens on record
// for a resume and the token views; nothing of it goes in the history. One that never
// said a word, a stream that failed at once, cost nothing.
func (t *thread) recordSpend(ans *answer) {
	if ans.usage == (record.Usage{}) {
		return
	}
	cost, total := t.a.account(ans)
	usage := ans.usage
	t.a.emit(record.Record{
		Type: record.Assistant, Msg: ans.id, Model: ans.model, Usage: &usage, Cost: cost, TotalCost: total,
		Stop: ans.stop, Sidechain: t.sidechain,
	})
}

// deltaBuffer holds a block's text until coalesceEvery has passed. It belongs to the
// goroutine reading the stream.
type deltaBuffer struct {
	emit  func(record.Record)
	msg   string
	block int
	kind  string
	text  strings.Builder
	last  time.Time
}

func (d *deltaBuffer) open(msg string, block int, kind string) {
	d.msg, d.block, d.kind = msg, block, kind
	d.last = time.Now()
}

func (d *deltaBuffer) add(s string) {
	d.text.WriteString(s)
	if time.Since(d.last) >= coalesceEvery {
		d.flush()
	}
}

func (d *deltaBuffer) flush() {
	if d.text.Len() == 0 {
		return
	}
	d.emit(record.Record{Type: record.Delta, Msg: d.msg, Block: d.block, Kind: d.kind, Text: d.text.String()})
	d.text.Reset()
	d.last = time.Now()
}

// newAnswer turns the accumulated message into what the loop keeps: each block's JSON as
// the API sent it. Empty text blocks go, the API refuses them back. After a model
// fallback, what came before the last switch stays out, text apart: those thinking
// blocks and calls belonged to the model that declined.
func newAnswer(msg *anthropic.BetaMessage, inputs map[int]*strings.Builder) *answer {
	ans := &answer{
		id:    msg.ID,
		stop:  string(msg.StopReason),
		model: string(msg.Model),
		usage: usageOf(msg),
	}
	boundary := -1
	for i, block := range msg.Content {
		if block.Type == "fallback" {
			boundary = i
		}
	}

	var kept []json.RawMessage
	var text strings.Builder
	for i, block := range msg.Content {
		switch block.Type {
		case "fallback":
			continue
		case "text":
			if block.Text == "" {
				continue
			}
			text.WriteString(block.Text)
		case "thinking", "redacted_thinking":
			if i < boundary {
				continue
			}
		case "tool_use":
			if i < boundary {
				continue
			}
			call := toolCall{ID: block.ID, Name: block.Name, Input: json.RawMessage(`{}`)}
			if b := inputs[i]; b != nil {
				if json.Valid([]byte(b.String())) {
					call.Input = json.RawMessage(b.String())
				} else {
					call.BadJSON = b.String()
				}
			} else if len(block.Input) > 0 && json.Valid(block.Input) {
				call.Input = block.Input
			}
			ans.calls = append(ans.calls, call)
		}
		kept = append(kept, json.RawMessage(block.RawJSON()))
	}
	ans.text = text.String()
	if len(kept) > 0 {
		ans.raw = marshalStable(struct {
			Role    string            `json:"role"`
			Content []json.RawMessage `json:"content"`
		}{"assistant", kept})
	}
	return ans
}
