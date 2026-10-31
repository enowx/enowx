package tokens

import (
	"bytes"
	"cmp"
	"encoding/json"
	"strings"

	"github.com/enowx/enowx/proto"
)

// The traffic layer: an agent that keeps no record of its tokens runs
// with the host's interceptor as its proxy, and each answer of a model
// API it gets is read for its usage, only the counts:
//   - Anthropic's Messages: usage on the message; streamed, message_start
//     has the input and message_delta the output so far;
//   - OpenAI's chat completions (usage; streamed, a last chunk with it when
//     the client asked) and Responses (response.usage; streamed, in
//     response.completed), and the APIs speaking them (OpenRouter, local
//     servers, most others);
//   - Gemini's generateContent: usageMetadata, streamed in every chunk,
//     the last one complete.

// ModelAPI reports whether a request to host and path is a model API
// call the meter reads, and its provider.
func ModelAPI(host, path string) (string, bool) {
	h := strings.ToLower(host)
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i:], "]") {
		h = h[:i]
	}
	p := strings.ToLower(path)
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	switch {
	case strings.HasSuffix(h, "anthropic.com"):
		return "anthropic", strings.HasSuffix(p, "/messages")
	case strings.HasSuffix(h, "openai.com"):
		return "openai", strings.HasSuffix(p, "/chat/completions") || strings.HasSuffix(p, "/responses")
	case strings.HasSuffix(h, "generativelanguage.googleapis.com"), strings.HasSuffix(h, "aiplatform.googleapis.com"):
		return "google", strings.Contains(p, ":generatecontent") || strings.Contains(p, ":streamgeneratecontent")
	case strings.HasSuffix(h, "openrouter.ai"):
		return "openrouter", strings.HasSuffix(p, "/chat/completions") || strings.HasSuffix(p, "/responses") || strings.HasSuffix(p, "/messages")
	}
	if strings.HasSuffix(p, "/chat/completions") || strings.HasSuffix(p, "/responses") || strings.HasSuffix(p, "/v1/messages") {
		if h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "[::1]" {
			return "local", true
		}
		return h, true
	}
	return "", false
}

// meterUsage is what a usage object says; set reports which fields it
// had, a stream's later events overriding only those.
type meterUsage struct {
	id, model                                         string
	input, cacheRead, cacheWrite, output, reasoning   int64
	hasInput, hasOutput, inputHasCache, cacheReadSeen bool
}

// Meter reads a model API's answer (a JSON body, or server-sent events)
// for its usage, as a call of provider; ok false when it says none.
func Meter(provider string, body []byte) (Call, bool) {
	var u meterUsage
	if looksLikeSSE(body) {
		for _, line := range bytes.Split(body, []byte("\n")) {
			data, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:"))
			if !ok {
				continue
			}
			data = bytes.TrimSpace(data)
			if len(data) == 0 || data[0] != '{' {
				continue
			}
			u.event(data)
		}
	} else {
		trimmed := bytes.TrimSpace(body)
		if len(trimmed) > 0 && trimmed[0] == '[' { // Gemini's streamed array
			var list []json.RawMessage
			if json.Unmarshal(trimmed, &list) == nil {
				for _, item := range list {
					u.event(item)
				}
			}
		} else {
			u.event(trimmed)
		}
	}
	if !u.hasInput && !u.hasOutput {
		return Call{}, false
	}
	input := u.input
	if u.inputHasCache {
		input = max(input-u.cacheRead-u.cacheWrite, 0)
	}
	c := Call{Provider: provider, Model: u.model, Input: input, CacheRead: u.cacheRead, CacheWrite: u.cacheWrite,
		Output: u.output, Reasoning: u.reasoning, Layer: proto.TokenLayerTraffic}
	if u.id != "" {
		c.ID = "traffic:" + u.id
	}
	return c, !c.Empty()
}

func looksLikeSSE(body []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(body), []byte("event:")) || bytes.HasPrefix(bytes.TrimSpace(body), []byte("data:")) ||
		bytes.Contains(body, []byte("\ndata:"))
}

// wireUsage holds every provider's usage fields.
type wireUsage struct {
	// Anthropic
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
	CacheRead    *int64 `json:"cache_read_input_tokens"`
	CacheCreate  *int64 `json:"cache_creation_input_tokens"`
	// OpenAI chat completions
	PromptTokens     *int64 `json:"prompt_tokens"`
	CompletionTokens *int64 `json:"completion_tokens"`
	PromptDetails    *struct {
		Cached int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionDetails *struct {
		Reasoning int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	// OpenAI Responses
	InputDetails *struct {
		Cached int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputDetails *struct {
		Reasoning int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	// Gemini (usageMetadata)
	PromptCount     *int64 `json:"promptTokenCount"`
	CandidatesCount *int64 `json:"candidatesTokenCount"`
	CachedCount     *int64 `json:"cachedContentTokenCount"`
	ThoughtsCount   *int64 `json:"thoughtsTokenCount"`
}

type wireEvent struct {
	ID            string     `json:"id"`
	Model         string     `json:"model"`
	ModelVersion  string     `json:"modelVersion"`
	ResponseID    string     `json:"responseId"`
	Usage         *wireUsage `json:"usage"`
	UsageMetadata *wireUsage `json:"usageMetadata"`
	Message       *struct {
		ID    string     `json:"id"`
		Model string     `json:"model"`
		Usage *wireUsage `json:"usage"`
	} `json:"message"`
	Response *struct {
		ID    string     `json:"id"`
		Model string     `json:"model"`
		Usage *wireUsage `json:"usage"`
	} `json:"response"`
}

func (u *meterUsage) event(data []byte) {
	var ev wireEvent
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	u.id = cmp.Or(ev.ResponseID, ev.ID, u.id)
	u.model = cmp.Or(ev.Model, ev.ModelVersion, u.model)
	if ev.Message != nil { // Anthropic's message_start
		u.id, u.model = cmp.Or(ev.Message.ID, u.id), cmp.Or(ev.Message.Model, u.model)
		u.take(ev.Message.Usage)
	}
	if ev.Response != nil { // OpenAI's response.completed
		u.id, u.model = cmp.Or(ev.Response.ID, u.id), cmp.Or(ev.Response.Model, u.model)
		u.take(ev.Response.Usage)
	}
	u.take(ev.Usage)
	u.take(ev.UsageMetadata)
}

// take applies the fields w has: a stream's later counts replace the
// earlier ones (Anthropic's output and Gemini's counts are running
// totals), a zero input in a delta not erasing message_start's.
func (u *meterUsage) take(w *wireUsage) {
	if w == nil {
		return
	}
	set := func(dst *int64, v *int64, flag *bool) {
		if v != nil && (*v > 0 || !*flag) {
			*dst, *flag = *v, true
		}
	}
	switch {
	case w.PromptTokens != nil || w.CompletionTokens != nil: // chat completions
		set(&u.input, w.PromptTokens, &u.hasInput)
		set(&u.output, w.CompletionTokens, &u.hasOutput)
		u.inputHasCache = true
		if w.PromptDetails != nil {
			u.cacheRead = w.PromptDetails.Cached
		}
		if w.CompletionDetails != nil {
			u.reasoning = w.CompletionDetails.Reasoning
		}
	case w.PromptCount != nil || w.CandidatesCount != nil: // Gemini
		set(&u.input, w.PromptCount, &u.hasInput)
		u.inputHasCache = true
		if w.CachedCount != nil {
			u.cacheRead = *w.CachedCount
		}
		var out, thoughts int64
		if w.CandidatesCount != nil {
			out = *w.CandidatesCount
		}
		if w.ThoughtsCount != nil {
			thoughts = *w.ThoughtsCount
		}
		total := out + thoughts
		set(&u.output, &total, &u.hasOutput)
		u.reasoning = max(u.reasoning, thoughts)
	default: // Anthropic, or OpenAI's Responses
		set(&u.input, w.InputTokens, &u.hasInput)
		set(&u.output, w.OutputTokens, &u.hasOutput)
		if w.InputDetails != nil { // Responses: input includes the cache
			u.inputHasCache = true
			u.cacheRead = w.InputDetails.Cached
		}
		if w.OutputDetails != nil {
			u.reasoning = w.OutputDetails.Reasoning
		}
		if w.CacheRead != nil && (*w.CacheRead > 0 || !u.cacheReadSeen) {
			u.cacheRead, u.cacheReadSeen = *w.CacheRead, true
		}
		if w.CacheCreate != nil && *w.CacheCreate > 0 {
			u.cacheWrite = *w.CacheCreate
		}
	}
}
