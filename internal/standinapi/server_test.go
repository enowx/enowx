package standinapi

import (
	"bufio"
	"io"
	"net/http"
	"strings"
	"testing"
)

func post(t *testing.T, s *Server, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(s.URL+"/v1/messages", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// events lists a stream's event names and the error a cut connection gave.
func events(resp *http.Response) (names []string, err error) {
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if name, ok := strings.CutPrefix(sc.Text(), "event: "); ok {
			names = append(names, name)
		}
	}
	return names, sc.Err()
}

func TestStreamHasTheAPIsShape(t *testing.T) {
	s := New(Reply{Blocks: []Block{
		Thinking("hmm", "sig"),
		TextBlock("hi"),
		ToolUse("t1", "Bash", `{"command":"ls"}`),
	}})
	defer s.Close()
	resp := post(t, s, `{"stream":true,"model":"m","messages":[]}`)
	names, err := events(resp)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(names, " ")
	if !strings.HasPrefix(joined, "message_start ping content_block_start content_block_delta") ||
		!strings.HasSuffix(joined, "content_block_stop message_delta message_stop") {
		t.Fatalf("events: %s", joined)
	}
	if len(s.Requests()) != 1 {
		t.Fatalf("requests: %d", len(s.Requests()))
	}
}

func TestDropCutsTheStream(t *testing.T) {
	s := New(Reply{Blocks: []Block{TextBlock(strings.Repeat("a", 100))}, Drop: true})
	defer s.Close()
	names, err := events(post(t, s, `{"stream":true,"model":"m"}`))
	if err == nil && strings.Contains(strings.Join(names, " "), "message_stop") {
		t.Fatalf("stream ended whole: %v", names)
	}
}

func TestErrorsAndRetryAfter(t *testing.T) {
	s := New(RateLimited("7"), Err(529))
	defer s.Close()
	resp := post(t, s, `{}`)
	if resp.StatusCode != 429 || resp.Header.Get("retry-after") != "7" {
		t.Fatalf("got %d %q", resp.StatusCode, resp.Header.Get("retry-after"))
	}
	resp = post(t, s, `{}`)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 529 || !strings.Contains(string(body), "overloaded_error") {
		t.Fatalf("got %d %s", resp.StatusCode, body)
	}
}

func TestBuiltInScriptCallsToolsWhenAsked(t *testing.T) {
	s := New()
	defer s.Close()
	body := `{"stream":false,"model":"m","tools":[{"name":"Bash","input_schema":{"required":["command"]}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"please use a tool"}]}]}`
	data, _ := io.ReadAll(post(t, s, body).Body)
	if !strings.Contains(string(data), `"tool_use"`) || !strings.Contains(string(data), "echo standin") {
		t.Fatalf("reply: %s", data)
	}
}
