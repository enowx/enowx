package tokens

import (
	"testing"
)

// A streamed Anthropic answer: message_start has the input, each
// message_delta the output so far.
const anthropicSSE = `event: message_start
data: {"type":"message_start","message":{"id":"msg_01Stream","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[],"usage":{"input_tokens":25,"cache_creation_input_tokens":1000,"cache_read_input_tokens":8000,"output_tokens":1}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":15}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":0,"output_tokens":42}}

event: message_stop
data: {"type":"message_stop"}

`

// A streamed OpenAI chat completion asked with include_usage: a last
// chunk with no choices and the usage.
const openaiChatSSE = `data: {"id":"chatcmpl-9x","object":"chat.completion.chunk","model":"gpt-4.1-2025-04-14","choices":[{"index":0,"delta":{"content":"Hi"}}],"usage":null}

data: {"id":"chatcmpl-9x","object":"chat.completion.chunk","model":"gpt-4.1-2025-04-14","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":null}

data: {"id":"chatcmpl-9x","object":"chat.completion.chunk","model":"gpt-4.1-2025-04-14","choices":[],"usage":{"prompt_tokens":1200,"completion_tokens":300,"total_tokens":1500,"prompt_tokens_details":{"cached_tokens":1000},"completion_tokens_details":{"reasoning_tokens":0}}}

data: [DONE]

`

// A streamed OpenAI Responses answer: response.completed has the usage.
const openaiResponsesSSE = `event: response.created
data: {"type":"response.created","response":{"id":"resp_77","model":"gpt-5.1","usage":null}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"Hi"}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_77","model":"gpt-5.1","usage":{"input_tokens":5000,"input_tokens_details":{"cached_tokens":4096},"output_tokens":800,"output_tokens_details":{"reasoning_tokens":512},"total_tokens":5800}}}

`

func TestMeterStreamedAnswers(t *testing.T) {
	cases := []struct {
		name, provider, body string
		want                 Call
	}{
		{"anthropic", "anthropic", anthropicSSE, Call{ID: "traffic:msg_01Stream", Model: "claude-sonnet-5-5", Input: 25, CacheWrite: 1000, CacheRead: 8000, Output: 42}},
		{"openai chat", "openai", openaiChatSSE, Call{ID: "traffic:chatcmpl-9x", Model: "gpt-4.1-2025-04-14", Input: 200, CacheRead: 1000, Output: 300}},
		{"openai responses", "openai", openaiResponsesSSE, Call{ID: "traffic:resp_77", Model: "gpt-5.1", Input: 904, CacheRead: 4096, Output: 800, Reasoning: 512}},
		{"anthropic json", "anthropic", `{"id":"msg_J","type":"message","model":"claude-haiku-5-5","usage":{"input_tokens":12,"output_tokens":3,"cache_read_input_tokens":0}}`,
			Call{ID: "traffic:msg_J", Model: "claude-haiku-5-5", Input: 12, Output: 3}},
		{"gemini stream", "google", `data: {"candidates":[{"content":{"parts":[{"text":"a"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":2},"modelVersion":"gemini-2.5-flash","responseId":"g1"}

data: {"candidates":[{"content":{"parts":[{"text":"b"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":9,"thoughtsTokenCount":20,"cachedContentTokenCount":60},"modelVersion":"gemini-2.5-flash","responseId":"g1"}

`, Call{ID: "traffic:g1", Model: "gemini-2.5-flash", Input: 40, CacheRead: 60, Output: 29, Reasoning: 20}},
	}
	for _, tc := range cases {
		got, ok := Meter(tc.provider, []byte(tc.body))
		if !ok {
			t.Errorf("%s: no usage read", tc.name)
			continue
		}
		got.Provider, got.Layer = "", ""
		if got != tc.want {
			t.Errorf("%s:\n got %+v\nwant %+v", tc.name, got, tc.want)
		}
	}
	if _, ok := Meter("openai", []byte(`{"error":{"message":"rate limited"}}`)); ok {
		t.Error("an error answer metered")
	}
}

func TestModelAPI(t *testing.T) {
	for _, tc := range []struct {
		host, path, provider string
		ok                   bool
	}{
		{"api.anthropic.com", "/v1/messages", "anthropic", true},
		{"api.anthropic.com", "/v1/messages/count_tokens", "anthropic", false},
		{"api.openai.com:443", "/v1/responses", "openai", true},
		{"generativelanguage.googleapis.com", "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse", "google", true},
		{"openrouter.ai", "/api/v1/chat/completions", "openrouter", true},
		{"localhost:11434", "/v1/chat/completions", "local", true},
		{"example.com", "/index.html", "", false},
	} {
		p, ok := ModelAPI(tc.host, tc.path)
		if p != tc.provider || ok != tc.ok {
			t.Errorf("%s%s: %q %v, want %q %v", tc.host, tc.path, p, ok, tc.provider, tc.ok)
		}
	}
}

func TestScreenCountsEachLineOnce(t *testing.T) {
	var s Screen
	screen := "> Tokens: 4.4k sent, 120 received. Cost: $0.01 message\n"
	first := s.Read("s1", screen, fixedNow)
	if len(first) != 1 || first[0].Input != 4400 || first[0].Output != 120 {
		t.Fatalf("first: %+v", first)
	}
	if again := s.Read("s1", screen, fixedNow); len(again) != 0 {
		t.Fatalf("read twice: %+v", again)
	}
	// The same line printed again is another answer.
	if more := s.Read("s1", screen+"#### again\n"+screen, fixedNow); len(more) != 1 || more[0].ID == first[0].ID {
		t.Fatalf("again: %+v", more)
	}
}
