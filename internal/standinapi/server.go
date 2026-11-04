// Package standinapi is a scriptable stand-in for Anthropic's Messages API, so that
// Claude Platform's loop is tested end to end with no key and no network. Tests queue
// replies (New, Enqueue) and read back what was asked (Requests); with nothing queued
// the built-in script answers (Respond), which is what cmd runs as a small server.
package standinapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
)

// Request is one POST /v1/messages as it arrived.
type Request struct {
	Path   string
	Header http.Header
	Body   []byte
}

// Raw is a top-level field of the body, its bytes as sent, nil when absent.
func (r Request) Raw(field string) json.RawMessage {
	var all map[string]json.RawMessage
	if json.Unmarshal(r.Body, &all) != nil {
		return nil
	}
	return all[field]
}

// Messages are the body's messages, each its bytes as sent.
func (r Request) Messages() []json.RawMessage {
	var list []json.RawMessage
	_ = json.Unmarshal(r.Raw("messages"), &list)
	return list
}

// Server is the stand-in. Its methods are safe for concurrent use; mu guards the queue
// and the record of requests, and is never held while answering.
type Server struct {
	// URL is the base address when started by New; a client's BaseURL.
	URL string

	ts *httptest.Server

	mu       sync.Mutex
	queue    []Reply
	requests []Request
	respond  func(Request) Reply
}

// New starts a server on a local port that gives script's replies in order, then the
// built-in script's. Close it when done.
func New(script ...Reply) *Server {
	s := NewUnstarted(script...)
	s.ts = httptest.NewServer(s.Handler())
	s.URL = s.ts.URL
	return s
}

// NewUnstarted is a Server to mount through Handler on a listener of the caller's.
func NewUnstarted(script ...Reply) *Server {
	return &Server{queue: script, respond: Respond}
}

// Close stops a server started by New.
func (s *Server) Close() {
	if s.ts != nil {
		s.ts.CloseClientConnections()
		s.ts.Close()
	}
}

// Enqueue adds replies after those still waiting.
func (s *Server) Enqueue(script ...Reply) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, script...)
}

// Requests are those received so far, in order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// Handler serves POST /v1/messages.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", s.serveMessages)
	return mux
}

func (s *Server) serveMessages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	req := Request{Path: r.URL.Path, Header: r.Header.Clone(), Body: body}
	reply := s.next(req)

	if reply.Status != 0 {
		writeError(w, reply)
		return
	}
	var flags struct {
		Stream bool   `json:"stream"`
		Model  string `json:"model"`
	}
	_ = json.Unmarshal(body, &flags)
	if reply.Model == "" {
		reply.Model = flags.Model
	}
	if flags.Stream {
		streamReply(w, r, reply)
		return
	}
	writeMessage(w, reply)
}

func (s *Server) next(req Request) Reply {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	n := len(s.requests)
	var reply Reply
	queued := len(s.queue) > 0
	if queued {
		reply, s.queue = s.queue[0], s.queue[1:]
	}
	respond := s.respond
	s.mu.Unlock()

	if !queued {
		reply = respond(req)
	}
	if reply.ID == "" {
		reply.ID = "msg_standin_" + strconv.Itoa(n)
	}
	return reply
}

func writeError(w http.ResponseWriter, reply Reply) {
	if reply.RetryAfter != "" {
		w.Header().Set("retry-after", reply.RetryAfter)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("request-id", "req_standin")
	w.WriteHeader(reply.Status)
	_ = json.NewEncoder(w).Encode(errorBody(errorType(reply.Status), reply.Message))
}

func errorBody(kind, message string) map[string]any {
	if message == "" {
		message = kind
	}
	return map[string]any{
		"type":       "error",
		"error":      map[string]any{"type": kind, "message": message},
		"request_id": "req_standin",
	}
}

// errorType is the API's name for a status.
func errorType(status int) string {
	switch status {
	case 400, 413:
		return "invalid_request_error"
	case 401:
		return "authentication_error"
	case 402:
		return "billing_error"
	case 403:
		return "permission_error"
	case 404:
		return "not_found_error"
	case 429:
		return "rate_limit_error"
	case 529:
		return "overloaded_error"
	}
	return "api_error"
}

func writeMessage(w http.ResponseWriter, reply Reply) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("request-id", "req_standin")
	content := []map[string]any{}
	for _, b := range reply.Blocks {
		content = append(content, b.final())
	}
	msg := messageHead(reply)
	msg["content"] = content
	msg["stop_reason"] = reply.stop()
	msg["stop_details"] = rawOrNil(reply.StopDetails)
	msg["usage"] = reply.Usage.fields(reply.outputTokens())
	_ = json.NewEncoder(w).Encode(msg)
}

func messageHead(reply Reply) map[string]any {
	return map[string]any{
		"id":            reply.ID,
		"type":          "message",
		"role":          "assistant",
		"model":         reply.Model,
		"content":       []any{},
		"stop_reason":   nil,
		"stop_sequence": nil,
	}
}

func rawOrNil(raw json.RawMessage) any {
	if raw == nil {
		return nil
	}
	return raw
}
