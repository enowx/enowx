package mcpclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// httpTransport is a server at a URL (Streamable HTTP): each message a POST, answered as
// JSON or as an event stream. The optional GET stream for what the server starts by
// itself isn't opened: the client offers nothing it could ask, and tools are listed once.
//
// Each send runs on its caller's goroutine, to the end of its reply. mu guards the
// session's id and version.
type httpTransport struct {
	cfg    serverConfig
	client *http.Client
	in     chan []byte
	done   chan struct{}
	once   sync.Once

	mu       sync.Mutex
	session  string
	protocol string
}

func newHTTPTransport(cfg serverConfig) *httpTransport {
	return &httpTransport{cfg: cfg, client: httpClient(), in: make(chan []byte), done: make(chan struct{}), protocol: protocolVersion}
}

// httpClient refuses a redirect to another host: the server's headers hold its keys, and
// only the standard ones are dropped on the way.
func httpClient() *http.Client {
	return &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		if req.URL.Host != via[0].URL.Host {
			return errors.New("it redirects to another host")
		}
		return nil
	}}
}

// plainError is what a request failed on, without the address, which may hold a key.
func plainError(err error) error {
	var u *url.Error
	if errors.As(err, &u) {
		return u.Err
	}
	return err
}

func (t *httpTransport) request(ctx context.Context, method string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, t.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, plainError(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	t.mu.Lock()
	req.Header.Set("MCP-Protocol-Version", t.protocol)
	if t.session != "" {
		req.Header.Set("Mcp-Session-Id", t.session)
	}
	t.mu.Unlock()
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

func (t *httpTransport) send(ctx context.Context, msg []byte, id int64) error {
	req, err := t.request(ctx, http.MethodPost, msg)
	if err != nil {
		return err
	}
	sentWith := req.Header.Get("Mcp-Session-Id")
	resp, err := t.client.Do(req)
	if err != nil {
		return plainError(err)
	}
	defer resp.Body.Close()
	if s := resp.Header.Get("Mcp-Session-Id"); s != "" {
		t.mu.Lock()
		t.session = s
		t.mu.Unlock()
	}
	switch {
	case resp.StatusCode == http.StatusNotFound && sentWith != "":
		t.mu.Lock()
		t.session = ""
		t.mu.Unlock()
		return errSessionExpired
	case resp.StatusCode >= 300:
		// Only the status: a body can repeat what was sent, keys included.
		return errors.New(statusText(resp))
	case id == 0:
		return nil
	}
	answered := false
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		err = readEvents(resp.Body, func(event string, data []byte) bool {
			if event != "" && event != "message" {
				return true
			}
			if !t.deliver(ctx, data) {
				return false
			}
			answered = answersRequest(data, id)
			return !answered // the answer closes it: a server may keep the stream open
		})
	} else {
		var body []byte
		body, err = io.ReadAll(io.LimitReader(resp.Body, maxMessage))
		if err == nil && t.deliver(ctx, body) {
			answered = answersRequest(body, id)
		}
	}
	if answered {
		return nil
	}
	if err != nil {
		return plainError(err)
	}
	return errors.New("it ended its reply without an answer")
}

func statusText(resp *http.Response) string {
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "HTTP " + resp.Status + ": it refused its credentials, or wants a sign-in"
	}
	return "HTTP " + resp.Status
}

// answersRequest says whether msg is the answer to request id.
func answersRequest(msg []byte, id int64) bool {
	var m struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	return json.Unmarshal(msg, &m) == nil && m.Method == "" && string(m.ID) == strconv.FormatInt(id, 10)
}

func (t *httpTransport) deliver(ctx context.Context, msg []byte) bool {
	select {
	case t.in <- msg:
		return true
	case <-ctx.Done():
		return false
	case <-t.done:
		return false
	}
}

func (t *httpTransport) inbound() <-chan []byte { return t.in }
func (t *httpTransport) ended() <-chan struct{} { return t.done }
func (t *httpTransport) why() string            { return "its connection was closed" }

func (t *httpTransport) negotiated(version string) {
	t.mu.Lock()
	t.protocol = version
	t.mu.Unlock()
}

// close ends the session with the server, if it has one, then the transport.
func (t *httpTransport) close() {
	t.once.Do(func() {
		t.endSession()
		close(t.done)
	})
}

func (t *httpTransport) endSession() {
	t.mu.Lock()
	session := t.session
	t.mu.Unlock()
	if session == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), noticeTimeout)
	defer cancel()
	req, err := t.request(ctx, http.MethodDelete, nil)
	if err != nil {
		return
	}
	if resp, err := t.client.Do(req); err == nil {
		resp.Body.Close()
	}
}

// readEvents reads an event stream, calling fn with each event until it returns false.
func readEvents(r io.Reader, fn func(event string, data []byte) bool) error {
	lines := bufio.NewScanner(r)
	lines.Buffer(make([]byte, 64<<10), maxMessage)
	event := ""
	var data []byte
	flush := func() bool {
		if len(data) == 0 {
			event = ""
			return true
		}
		ok := fn(event, bytes.TrimSuffix(data, []byte("\n")))
		event, data = "", nil
		return ok
	}
	for lines.Scan() {
		line := lines.Text()
		if line == "" {
			if !flush() {
				return nil
			}
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			data = append(append(data, value...), '\n')
		}
	}
	if err := lines.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("it sent a message over %d MB", maxMessage>>20)
		}
		return err
	}
	flush() // a last event without its blank line
	return nil
}
