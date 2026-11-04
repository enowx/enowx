package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/core/platformagent/tools"
)

// fakeHTTP is a Streamable HTTP server with sessions, paging, event-stream replies and
// the means to forget a session.
type fakeHTTP struct {
	mu        sync.Mutex
	sessions  map[string]bool
	made      int
	deleted   []string
	cancelled []string
	headers   []http.Header
	protocols []string
}

func (f *fakeHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("Mcp-Session-Id")
	if r.Method == http.MethodDelete {
		f.mu.Lock()
		f.deleted = append(f.deleted, session)
		f.mu.Unlock()
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Name      string          `json:"name"`
			Cursor    string          `json:"cursor"`
			RequestID json.RawMessage `json:"requestId"`
		} `json:"params"`
	}
	_ = json.Unmarshal(body, &req)
	f.mu.Lock()
	f.headers = append(f.headers, r.Header.Clone())
	if req.Method == "initialize" {
		f.made++
		session = fmt.Sprintf("session-%d", f.made)
		f.sessions[session] = true
		f.mu.Unlock()
		w.Header().Set("Mcp-Session-Id", session)
		writeJSON(w, req.ID, map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}}})
		return
	}
	known := f.sessions[session]
	f.protocols = append(f.protocols, r.Header.Get("MCP-Protocol-Version"))
	if req.Method == "notifications/cancelled" {
		f.cancelled = append(f.cancelled, string(req.Params.RequestID))
	}
	f.mu.Unlock()
	if !known {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	switch req.Method {
	case "tools/list":
		if req.Params.Cursor == "" {
			writeJSON(w, req.ID, map[string]any{"nextCursor": "2", "tools": []any{map[string]any{"name": "plain"}, map[string]any{"name": "stream"}}})
			return
		}
		writeJSON(w, req.ID, map[string]any{"tools": []any{map[string]any{"name": "block"}, map[string]any{"name": "cutoff"}, map[string]any{"name": "keepopen"}}})
	case "tools/call":
		result := map[string]any{"content": []any{map[string]any{"type": "text", "text": "from " + req.Params.Name}}}
		switch req.Params.Name {
		case "plain":
			writeJSON(w, req.ID, result)
		case "block":
			<-r.Context().Done()
		case "cutoff":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": nothing to say\n\n")
		case "stream", "keepopen":
			w.Header().Set("Content-Type", "text/event-stream")
			answer, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
			progress := `{"jsonrpc":"2.0","method":"notifications/progress","params":{}}`
			// The answer's data split over lines, as the format allows.
			split := strings.Replace(string(answer), `,"result"`, ",\ndata: \"result\"", 1)
			fmt.Fprintf(w, "event: message\ndata: %s\n\nevent: message\ndata: %s\n\n", progress, split)
			w.(http.Flusher).Flush()
			if req.Params.Name == "keepopen" {
				<-r.Context().Done()
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func connectHTTP(t *testing.T, kind string, h http.Handler, environ []string) (*Servers, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	spec, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"web": map[string]any{
		"type": kind, "url": srv.URL + "/mcp", "headers": map[string]string{"Authorization": "Bearer ${KEY}"},
	}}})
	s, err := Connect(context.Background(), string(spec), environ)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, srv
}

func newFakeHTTP() *fakeHTTP { return &fakeHTTP{sessions: map[string]bool{}} }

func TestHTTPListCallAndStreamedReply(t *testing.T) {
	f := newFakeHTTP()
	s, _ := connectHTTP(t, "http", f, []string{"KEY=" + testSecret})
	if len(s.Problems()) != 0 || len(s.Tools()) != 5 {
		t.Fatalf("tools %d problems %v", len(s.Tools()), s.Problems())
	}
	if r := run(find(t, s, "mcp__web__plain"), `{}`); r.Text != "from plain" {
		t.Errorf("plain: %+v", r)
	}
	if r := run(find(t, s, "mcp__web__stream"), `{}`); r.Text != "from stream" {
		t.Errorf("stream: %+v", r)
	}
	// An answered stream a server keeps open doesn't keep the call.
	if r := run(find(t, s, "mcp__web__keepopen"), `{}`); r.Text != "from keepopen" {
		t.Errorf("keepopen: %+v", r)
	}
	if r := run(find(t, s, "mcp__web__cutoff"), `{}`); !r.Failed || !strings.Contains(r.Text, "without an answer") {
		t.Errorf("cutoff: %+v", r)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	last := f.headers[len(f.headers)-1]
	if last.Get("Authorization") != "Bearer "+testSecret || last.Get("Mcp-Session-Id") != "session-1" {
		t.Error("the headers weren't as configured, or the session's id wasn't sent")
	}
	// After the handshake, the version the server chose.
	if got := f.protocols[len(f.protocols)-1]; got != "2025-03-26" {
		t.Errorf("protocol header: %s", got)
	}
}

func TestHTTPCancelTellsTheServer(t *testing.T) {
	f := newFakeHTTP()
	s, _ := connectHTTP(t, "http", f, []string{"KEY=x"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan tools.Result, 1)
	go func() { done <- find(t, s, "mcp__web__block").Run(ctx, json.RawMessage(`{}`), tools.Env{}) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case r := <-done:
		if !r.Failed || !strings.Contains(r.Text, "Interrupted") {
			t.Errorf("result: %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the call didn't return on cancel")
	}
	waitFor(t, "notifications/cancelled", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.cancelled) == 1
	})
}

func TestHTTPExpiredSessionStartsAnew(t *testing.T) {
	f := newFakeHTTP()
	s, _ := connectHTTP(t, "http", f, []string{"KEY=x"})
	f.mu.Lock()
	f.sessions = map[string]bool{}
	f.mu.Unlock()
	if r := run(find(t, s, "mcp__web__plain"), `{}`); r.Failed || r.Text != "from plain" {
		t.Fatalf("after expiry: %+v", r)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.made != 2 {
		t.Errorf("sessions made: %d", f.made)
	}
}

func TestHTTPCloseEndsTheSession(t *testing.T) {
	f := newFakeHTTP()
	s, _ := connectHTTP(t, "http", f, []string{"KEY=x"})
	_ = s.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.deleted) != 1 || f.deleted[0] != "session-1" {
		t.Errorf("deleted: %v", f.deleted)
	}
}

// A server that echoes what it got into its refusal: Problems has its status alone.
func TestHTTPRefusalHasNoSecret(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key "+r.Header.Get("Authorization"), http.StatusUnauthorized)
	})
	s, _ := connectHTTP(t, "http", h, []string{"KEY=" + testSecret})
	problems := strings.Join(s.Problems(), "\n")
	if !strings.Contains(problems, "MCP server web: initialize: HTTP 401") {
		t.Errorf("problems: %s", problems)
	}
	if strings.Contains(problems, testSecret) {
		t.Errorf("a secret in problems: %s", problems)
	}
}

// Neither an unreachable URL's key nor an RPC error repeating a header gets through.
func TestProblemsHideURLsAndEchoes(t *testing.T) {
	spec := `{"mcpServers":{"down":{"type":"http","url":"http://127.0.0.1:1/mcp?key=` + testSecret + `"}}}`
	s, err := Connect(context.Background(), spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if p := strings.Join(s.Problems(), "\n"); len(s.Problems()) != 1 || strings.Contains(p, testSecret) {
		t.Errorf("problems: %s", p)
	}

	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "error": map[string]any{"code": -1, "message": "refused " + r.Header.Get("Authorization")}})
	})
	s2, _ := connectHTTP(t, "http", echo, []string{"KEY=" + testSecret})
	if p := strings.Join(s2.Problems(), "\n"); !strings.Contains(p, "refused") || strings.Contains(p, testSecret) {
		t.Errorf("problems: %s", p)
	}
}

func TestRedirectToAnotherHostIsRefused(t *testing.T) {
	got := make(chan string, 1)
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got <- r.Header.Get("Authorization") }))
	defer other.Close()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1), http.StatusTemporaryRedirect)
	})
	s, _ := connectHTTP(t, "http", h, []string{"KEY=" + testSecret})
	if len(s.Problems()) != 1 {
		t.Errorf("problems: %v", s.Problems())
	}
	select {
	case <-got:
		t.Error("the request followed the redirect")
	default:
	}
}

// fakeSSE is the older transport: a GET stream names the endpoint and carries answers.
type fakeSSE struct {
	out chan string
}

func (f *fakeSSE) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: endpoint\ndata: /messages?session=1\n\n")
		w.(http.Flusher).Flush()
		for {
			select {
			case m := <-f.out:
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", m)
				w.(http.Flusher).Flush()
			case <-r.Context().Done():
				return
			}
		}
	}
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	w.WriteHeader(http.StatusAccepted)
	if len(req.ID) == 0 {
		return
	}
	var result any
	switch req.Method {
	case "initialize":
		result = map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{}}
	case "tools/list":
		result = map[string]any{"tools": []any{map[string]any{"name": "hello"}}}
	default:
		result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "hi"}}}
	}
	answer, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	f.out <- string(answer)
}

func TestSSETransport(t *testing.T) {
	s, _ := connectHTTP(t, "sse", &fakeSSE{out: make(chan string, 4)}, []string{"KEY=x"})
	if len(s.Problems()) != 0 {
		t.Fatalf("problems: %v", s.Problems())
	}
	if r := run(find(t, s, "mcp__web__hello"), `{}`); r.Failed || r.Text != "hi" {
		t.Errorf("call: %+v", r)
	}
}
