package standinapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// chunkSize is how much text one delta carries, so tests see several.
const chunkSize = 16

// streamReply writes reply as the API's server-sent events. Dropping panics with
// http.ErrAbortHandler, which is how a handler cuts its connection without an ending.
func streamReply(w http.ResponseWriter, r *http.Request, reply Reply) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("request-id", "req_standin")
	w.WriteHeader(http.StatusOK)
	send := func(kind string, data map[string]any) {
		data["type"] = kind
		raw, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, raw)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	head := messageHead(reply)
	head["usage"] = reply.Usage.fields(1)
	send("message_start", map[string]any{"message": head})
	send("ping", map[string]any{})

	for i, b := range reply.Blocks {
		pieces := b.deltas()
		interruptHere := (reply.Drop || reply.StreamError != "") && i == reply.DropAfter
		holdHere := reply.Hold != nil && i == reply.HoldAfter

		send("content_block_start", map[string]any{"index": i, "content_block": b.start()})
		if interruptHere || holdHere {
			pieces = pieces[:len(pieces)/2]
		}
		for _, d := range pieces {
			send("content_block_delta", map[string]any{"index": i, "delta": d})
		}
		if holdHere {
			select {
			case <-reply.Hold:
			case <-r.Context().Done():
				panic(http.ErrAbortHandler)
			}
			for _, d := range b.deltas()[len(pieces):] {
				send("content_block_delta", map[string]any{"index": i, "delta": d})
			}
		}
		if interruptHere {
			interrupt(send, reply)
		}
		send("content_block_stop", map[string]any{"index": i})
	}
	if (reply.Drop || reply.StreamError != "") && reply.DropAfter >= len(reply.Blocks) {
		interrupt(send, reply)
	}

	send("message_delta", map[string]any{
		"delta": map[string]any{
			"stop_reason":   reply.stop(),
			"stop_sequence": nil,
			"stop_details":  rawOrNil(reply.StopDetails),
		},
		"usage": map[string]any{"output_tokens": reply.outputTokens()},
	})
	send("message_stop", map[string]any{})
}

func interrupt(send func(string, map[string]any), reply Reply) {
	if reply.StreamError != "" {
		send("error", errorBody(reply.StreamError, reply.Message))
	}
	panic(http.ErrAbortHandler)
}

// start is the block as content_block_start opens it.
func (b Block) start() map[string]any {
	switch b.Type {
	case "thinking":
		return map[string]any{"type": "thinking", "thinking": "", "signature": ""}
	case "redacted_thinking":
		return map[string]any{"type": "redacted_thinking", "data": b.Text}
	case "tool_use":
		return map[string]any{"type": "tool_use", "id": b.ID, "name": b.Name, "input": map[string]any{}}
	}
	if b.Raw != nil {
		return b.rawFields()
	}
	return map[string]any{"type": "text", "text": ""}
}

// deltas are the block's content_block_delta bodies.
func (b Block) deltas() []map[string]any {
	var out []map[string]any
	switch b.Type {
	case "thinking":
		for _, c := range chunks(b.Text) {
			out = append(out, map[string]any{"type": "thinking_delta", "thinking": c})
		}
		out = append(out, map[string]any{"type": "signature_delta", "signature": b.Signature})
	case "tool_use":
		for _, c := range chunks(b.Input) {
			out = append(out, map[string]any{"type": "input_json_delta", "partial_json": c})
		}
	case "text":
		for _, c := range chunks(b.Text) {
			out = append(out, map[string]any{"type": "text_delta", "text": c})
		}
	}
	return out
}

// chunks cuts s in pieces, whole runes each.
func chunks(s string) []string {
	var out []string
	runes := []rune(s)
	for len(runes) > 0 {
		n := min(chunkSize, len(runes))
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}
