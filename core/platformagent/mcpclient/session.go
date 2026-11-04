package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// protocolVersion is the MCP version the client speaks; servers answer with theirs.
const protocolVersion = "2025-06-18"

// maxMessage bounds one message from a server: images come base64 inside a line.
const maxMessage = 64 << 20

// noticeTimeout bounds a notification sent when the caller has already gone.
const noticeTimeout = 2 * time.Second

// transport carries JSON-RPC messages to one running server and back.
type transport interface {
	// send hands one message to the server, id its request's (0 for what has no
	// answer). It returns when the message has gone, and for HTTP when the reply has
	// come; a failure means no answer will come. Answers arrive on inbound. ctx ends
	// the send and, over HTTP, the reply's stream.
	send(ctx context.Context, msg []byte, id int64) error
	// inbound are the messages from the server. Its reader hands each in order.
	inbound() <-chan []byte
	// ended closes when no more will arrive; why then says what happened.
	ended() <-chan struct{}
	why() string
	// negotiated tells it the version the server chose.
	negotiated(version string)
	close()
}

// rpcError is a JSON-RPC error answer.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

const rpcMethodNotFound = -32601

// goneError says the server stopped with a request open.
type goneError struct{ why string }

func (e *goneError) Error() string { return e.why }

type reply struct {
	result json.RawMessage
	err    error
}

type registration struct {
	answer chan reply // buffered: the owner never waits on it
	id     chan int64 // buffered
}

// session is one running server: a transport and its open requests.
//
// run is the owner goroutine: it alone touches the table of open requests and the next
// id, so calls side by side only meet through its channels, and never holds anything
// while a message is written. Callers write from their own goroutines.
type session struct {
	tr       transport
	register chan registration
	forget   chan int64
	stopped  chan struct{} // closed when run has answered every open request
	gone     *goneError    // set before stopped closes
}

func newSession(tr transport) *session {
	s := &session{tr: tr, register: make(chan registration), forget: make(chan int64), stopped: make(chan struct{})}
	go s.run()
	return s
}

func (s *session) run() {
	defer close(s.stopped)
	open := map[int64]chan reply{}
	next := int64(0)
	for {
		select {
		case r := <-s.register:
			next++
			open[next] = r.answer
			r.id <- next
		case id := <-s.forget:
			delete(open, id)
		case msg := <-s.tr.inbound():
			s.route(msg, open)
		case <-s.tr.ended():
			s.gone = &goneError{why: s.tr.why()}
			for _, answer := range open {
				answer <- reply{err: s.gone}
			}
			return
		}
	}
}

// route gives an answer to its request and replies to what the server itself asks.
func (s *session) route(msg []byte, open map[int64]chan reply) {
	var m struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if json.Unmarshal(msg, &m) != nil {
		return // a stray line
	}
	if m.Method != "" {
		if len(m.ID) > 0 {
			go s.answerServer(m.ID, m.Method)
		}
		return // notifications (progress, logs, list changes) have nothing to do here
	}
	id, err := strconv.ParseInt(string(m.ID), 10, 64)
	if err != nil {
		return
	}
	answer, ok := open[id]
	if !ok {
		return // a call the caller gave up on
	}
	delete(open, id)
	if m.Error != nil {
		answer <- reply{err: m.Error}
	} else {
		answer <- reply{result: m.Result}
	}
}

// answerServer replies to a request from the server: the client offers no capability,
// so only ping has an answer. It runs apart from run, whose work never waits on a write.
func (s *session) answerServer(id json.RawMessage, method string) {
	m := map[string]any{"jsonrpc": "2.0", "id": id}
	if method == "ping" {
		m["result"] = map[string]any{}
	} else {
		m["error"] = rpcError{Code: rpcMethodNotFound, Message: "method not found"}
	}
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), noticeTimeout)
	defer cancel()
	_ = s.tr.send(ctx, data, 0)
}

// request sends method and waits for its answer. When ctx ends first the server is told
// and request returns ctx's error at once, the answer ignored should it come.
func (s *session) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	encodedParams, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("its input isn't JSON: %w", err)
	}
	reg := registration{answer: make(chan reply, 1), id: make(chan int64, 1)}
	select {
	case s.register <- reg:
	case <-s.stopped:
		return nil, s.gone
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	id := <-reg.id
	msg, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": json.RawMessage(encodedParams)})
	if err != nil {
		s.drop(id)
		return nil, err
	}

	// Ending this ends the send, and an HTTP reply's stream with it, once answered.
	sendCtx, endSend := context.WithCancel(ctx)
	defer endSend()
	sent := make(chan error, 1)
	go func() { sent <- s.tr.send(sendCtx, msg, id) }()
	for {
		select {
		case r := <-reg.answer:
			return r.result, r.err
		case err := <-sent:
			sent = nil
			if err != nil && ctx.Err() == nil {
				s.drop(id)
				return nil, err
			}
		case <-ctx.Done():
			s.drop(id)
			if method != "initialize" { // which the protocol says isn't to be cancelled
				go s.tellCancelled(id)
			}
			return nil, ctx.Err()
		}
	}
}

func (s *session) drop(id int64) {
	select {
	case s.forget <- id:
	case <-s.stopped:
	}
}

func (s *session) tellCancelled(id int64) {
	ctx, cancel := context.WithTimeout(context.Background(), noticeTimeout)
	defer cancel()
	s.notify(ctx, "notifications/cancelled", map[string]any{"requestId": id, "reason": "interrupted"})
}

// notify sends what has no answer; a failure of it changes nothing for the caller.
func (s *session) notify(ctx context.Context, method string, params any) {
	m := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		m["params"] = params
	}
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = s.tr.send(ctx, data, 0)
}

// handshake opens the MCP session.
func (s *session) handshake(ctx context.Context) error {
	raw, err := s.request(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "enowx", "version": "1"},
	})
	if err != nil {
		var gone *goneError
		if errors.As(err, &gone) {
			return err
		}
		return fmt.Errorf("initialize: %w", err)
	}
	var info struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return errors.New("initialize: its answer isn't an MCP one")
	}
	if info.ProtocolVersion != "" {
		s.tr.negotiated(info.ProtocolVersion)
	}
	s.notify(ctx, "notifications/initialized", nil)
	return nil
}

// close ends the transport and waits for run to answer what is open.
func (s *session) close() {
	s.tr.close()
	<-s.stopped
}

func (s *session) over() bool {
	select {
	case <-s.stopped:
		return true
	default:
		return false
	}
}
