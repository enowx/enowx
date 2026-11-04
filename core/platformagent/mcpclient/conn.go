package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// restartCooldown keeps one restart attempt per crash from becoming a loop: a server
// that dies again this soon stays down until a later call.
const restartCooldown = time.Minute

// maxToolPages bounds a server's paging through tools/list.
const maxToolPages = 100

var errClosed = errors.New("it was closed")

// errSessionExpired is an HTTP server not knowing the session: the request never ran.
var errSessionExpired = errors.New("the server ended the session")

// conn is one configured server over its life: its running session and, when that ends,
// the next one. mu guards only these fields, never held over I/O.
type conn struct {
	cfg     serverConfig
	timeout time.Duration
	// base ends a start under way when the conn closes.
	base   context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	current     *session
	starting    chan struct{} // closed when the start under way is over
	lastRestart time.Time
	expired     bool // an HTTP server dropped the session
	closed      bool
}

func newConn(cfg serverConfig, timeout time.Duration) *conn {
	base, cancel := context.WithCancel(context.Background())
	return &conn{cfg: cfg, timeout: timeout, base: base, cancel: cancel}
}

// dial starts the transport for the server's kind.
func (c *conn) dial(ctx context.Context) (transport, error) {
	switch c.cfg.kind {
	case kindStdio:
		return startStdio(c.cfg)
	case kindSSE:
		return dialSSE(ctx, c.cfg)
	default:
		return newHTTPTransport(c.cfg), nil
	}
}

// open starts one session, bounded by the conn's timeout, ctx and the conn's closing.
func (c *conn) open(ctx context.Context) (*session, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	defer context.AfterFunc(c.base, cancel)()
	tr, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	s := newSession(tr)
	if err := s.handshake(ctx); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

// live is the running session, starting it, or starting it again when it has ended and
// that's allowed. Calls side by side share one start.
func (c *conn) live(ctx context.Context) (*session, error) {
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, errClosed
		}
		if c.current != nil && !c.current.over() {
			s := c.current
			c.mu.Unlock()
			return s, nil
		}
		if wait := c.starting; wait != nil {
			c.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		previous := c.lastRestart
		if c.current != nil {
			if err := c.restartRefusal(); err != nil {
				c.mu.Unlock()
				return nil, err
			}
			c.lastRestart = time.Now()
		}
		done := make(chan struct{})
		c.starting = done
		c.mu.Unlock()

		s, err := c.open(ctx)

		c.mu.Lock()
		c.starting = nil
		if err == nil && c.closed {
			err = errClosed
		}
		if err == nil {
			c.current, c.expired = s, false
		} else if ctx.Err() != nil {
			c.lastRestart = previous // the caller gave up: that was no attempt
		}
		c.mu.Unlock()
		close(done)
		if err != nil {
			if errors.Is(err, errClosed) && s != nil {
				s.close()
			}
			return nil, err
		}
		return s, nil
	}
}

// restartRefusal says why a session that ended can't start again now; c.mu is held.
func (c *conn) restartRefusal() error {
	if c.cfg.kind == kindHTTP && !c.expired {
		return errors.New("its connection ended") // nothing to start: each call connects anew
	}
	if !c.lastRestart.IsZero() && time.Since(c.lastRestart) < restartCooldown {
		return errors.New("it stopped again soon after a restart, and won't be started again yet")
	}
	return nil
}

// call sends a request to the running session. In-flight calls aren't sent again when a
// server dies under them: one may have partly run. One the server never took is.
func (c *conn) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s, err := c.live(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := s.request(ctx, method, params)
	if !errors.Is(err, errSessionExpired) {
		return raw, err
	}
	c.mu.Lock()
	c.expired = true
	c.mu.Unlock()
	s.close()
	if s, err = c.live(ctx); err != nil {
		return nil, err
	}
	return s.request(ctx, method, params)
}

// close ends the running session and any start under way: its program is gone on return.
func (c *conn) close() {
	c.cancel()
	c.mu.Lock()
	c.closed = true
	s, wait := c.current, c.starting
	c.mu.Unlock()
	if wait != nil {
		<-wait
	}
	if s != nil {
		s.close()
	}
}

// listedTool is one tool as a server lists it.
type listedTool struct {
	conn *conn
	Name string `json:"name"`
	// Description, InputSchema and Annotations are its entry's own.
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations struct {
		ReadOnlyHint bool `json:"readOnlyHint"`
	} `json:"annotations"`
}

// startAndList starts the server and lists its tools within the conn's timeout.
func (c *conn) startAndList(ctx context.Context) ([]listedTool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if _, err := c.live(ctx); err != nil {
		return nil, err
	}
	var out []listedTool
	cursor := ""
	for range maxToolPages {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.call(ctx, "tools/list", params)
		if err != nil {
			var rpc *rpcError
			if errors.As(err, &rpc) && rpc.Code == rpcMethodNotFound {
				return nil, nil // a server without tools
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		var page struct {
			Tools      []listedTool `json:"tools"`
			NextCursor string       `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, errors.New("tools/list: its answer isn't a list of tools")
		}
		for i := range page.Tools {
			page.Tools[i].conn = c
		}
		out = append(out, page.Tools...)
		if page.NextCursor == "" {
			return out, nil
		}
		cursor = page.NextCursor
	}
	return nil, errors.New("tools/list: it never reached the end of its list")
}
