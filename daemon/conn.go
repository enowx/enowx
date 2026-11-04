package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/enowx/enowx/proto"
)

// conn is one client connection. Its read loop runs on the goroutine that
// called serve; writes come from many goroutines and are serialized by wmu.
type conn struct {
	srv *Server
	nc  net.Conn
	// self is the daemon calling as GOA (goaCall): what it reads for a line, GOA hasn't.
	self bool

	wmu sync.Mutex

	mu         sync.Mutex
	streams    map[uint32]*stream
	tunnels    map[uint32]*tunnel
	adb        adbBridge
	nextStream uint32
	closed     bool
	// gone closes with the connection, for a request waiting on something else.
	gone chan struct{}
	// greeted is set once the handshake is done: events reach the
	// connection only after, as its client reads none before.
	greeted atomic.Bool
	// parts: its client reads a frame too large for one as parts; lastPart
	// numbers those sent.
	parts    atomic.Bool
	lastPart atomic.Uint32
	// id names the connection in both sides' logs, kind says what connected;
	// opened is since when; closer why this side ended it, "" when it didn't.
	id, kind string
	opened   time.Time
	closer   atomic.Pointer[string]
	finished atomic.Bool
	// remote: it came through the team listener, a visitor's, served once
	// visitor is set (visitors.go).
	remote  bool
	visitor atomic.Pointer[proto.Visitor]
}

func newConn(s *Server, nc net.Conn) *conn {
	return &conn{srv: s, nc: nc, streams: map[uint32]*stream{}, gone: make(chan struct{})}
}

func (c *conn) serve() error {
	defer func() {
		c.finished.Store(true) // this close is the end of serving: not a reason of its own
		c.close()
	}()
	if c.remote {
		// A visitor has this long to say hello and hand their ticket.
		_ = c.nc.SetReadDeadline(time.Now().Add(visitorSignInWait))
	}
	if err := c.handshake(); err != nil {
		return err
	}
	var large proto.Assembler // this goroutine's
	for {
		f, err := c.readFrame()
		if err != nil {
			return err
		}
		if f.Type == proto.TypePart && !c.remote { // a visitor gets no room to hold much
			whole, done, err := large.Take(f)
			if err != nil {
				return err
			}
			if !done {
				continue
			}
			f = whole
		}
		if c.remote && c.visitor.Load() == nil {
			// Before their ticket: team.auth alone, one at a time, here;
			// anything else, or a ticket refused, ends the connection.
			if err := c.signIn(f); err != nil {
				return err
			}
			_ = c.nc.SetReadDeadline(time.Time{})
			continue
		}
		switch f.Type {
		case proto.TypeRequest:
			var req proto.Request
			if err := json.Unmarshal(f.Payload, &req); err != nil {
				return fmt.Errorf("bad request: %w", err)
			}
			// Requests run in order on this goroutine, so a resize sent
			// after an attach always finds its stream; slow ones, which
			// no other request depends on, run beside it.
			if slowMethods[req.Method] {
				go c.answer(req)
				continue
			}
			c.answer(req)
		case proto.TypeInput:
			// A visitor watching types nothing in, until they take over.
			if st := c.stream(f.Channel); st != nil && c.mayType(st.sess.ID()) {
				st.input(f.Payload)
			}
		case proto.TypeTunnelData, proto.TypeTunnelAck, proto.TypeTunnelClose:
			if t := c.tunnel(f.Channel); t != nil {
				t.take(f)
			}
		default:
			return fmt.Errorf("unexpected %s frame", f.Type)
		}
	}
}

// readFrame reads the connection's next frame: one of a visitor not yet
// signed in no bigger than a ticket needs.
func (c *conn) readFrame() (proto.Frame, error) {
	if c.remote && c.visitor.Load() == nil {
		return proto.ReadFrameMax(c.nc, visitorPreAuthMax)
	}
	return proto.ReadFrame(c.nc)
}

// signIn answers frame f of a visitor not signed in yet: their team.auth,
// or an error that ends the connection.
func (c *conn) signIn(f proto.Frame) error {
	if f.Type != proto.TypeRequest {
		return fmt.Errorf("unexpected %s frame before team.auth", f.Type)
	}
	var req proto.Request
	if err := json.Unmarshal(f.Payload, &req); err != nil {
		return fmt.Errorf("bad request: %w", err)
	}
	if req.Method != proto.MethodTeamAuth {
		c.reply(req.ID, nil, errSignInFirst)
		return errSignInFirst
	}
	value, err := c.handle(req)
	_ = c.reply(req.ID, value, err)
	return err
}

// isClosed says the connection was closed.
func (c *conn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *conn) handshake() error {
	f, err := c.readFrame()
	if err != nil {
		return err
	}
	if f.Type != proto.TypeHello {
		return fmt.Errorf("want hello, got %s", f.Type)
	}
	var h proto.Hello
	if err := json.Unmarshal(f.Payload, &h); err != nil {
		return fmt.Errorf("bad hello: %w", err)
	}
	v, ok := proto.Negotiate(h.Protocol)
	if !ok {
		return fmt.Errorf("protocol %d not supported (daemon speaks %d to %d)", h.Protocol, proto.MinVersion, proto.Version)
	}
	c.parts.Store(h.Parts)
	c.id, c.kind, c.opened = h.Conn, h.Kind, time.Now()
	if c.id == "" {
		c.id = "old-" + newRenderID()[:8] // a client from before they had names
	}
	if c.logged() {
		c.srv.Diag.Event("conn.open", "conn", c.id, "kind", c.kind, "client", h.Agent, "remote", c.remote)
	}
	payload, _ := json.Marshal(proto.Hello{Protocol: v, Agent: "enowxd/" + Version, Parts: !c.remote})
	// Greeted before the answer goes, under the lock events write with:
	// an event finding it set waits for the answer, and none raised once
	// the client has its answer is missed.
	c.wmu.Lock()
	c.greeted.Store(true)
	err = proto.WriteFrame(c.nc, proto.Frame{Type: proto.TypeHello, Payload: payload})
	c.wmu.Unlock()
	return err
}

func (c *conn) attach(id string) (uint32, error) {
	sess, err := c.srv.sessions.Get(id)
	if err != nil {
		return 0, err
	}
	m, err := sess.Terminal().NewMirror()
	if err != nil {
		return 0, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		m.Close()
		return 0, errors.New("connection closed")
	}
	c.nextStream++
	st := newStream(c, c.nextStream, sess, m)
	c.streams[st.id] = st
	c.mu.Unlock()
	go st.pump()
	return st.id, nil
}

func (c *conn) detach(id uint32) {
	c.mu.Lock()
	st := c.streams[id]
	delete(c.streams, id)
	c.mu.Unlock()
	if st != nil {
		st.stop()
	}
}

func (c *conn) stream(id uint32) *stream {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.streams[id]
}

// send writes a frame; a write that fails closes the socket, which the read loop
// sees. A frame too large for this connection is refused alone (proto.ErrTooLarge,
// which a caller waiting on an answer returns): the connection stays up for
// everything else it carries.
func (c *conn) send(f proto.Frame) error {
	if len(f.Payload) > proto.MaxPayload {
		return c.sendLarge(f)
	}
	return c.write(f)
}

func (c *conn) write(f proto.Frame) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	err := proto.WriteFrame(c.nc, f)
	if err != nil {
		c.closedBecause("daemon: write failed")
		_ = c.nc.Close()
	}
	return err
}

// sendLarge sends f as parts, other frames going between them, to a client that
// takes them.
func (c *conn) sendLarge(f proto.Frame) error {
	if !c.parts.Load() || !proto.FitsParts(len(f.Payload)) {
		c.srv.logger().Warn("frame too large, refused", "frame", frameName(f), "bytes", len(f.Payload), "parts", c.parts.Load())
		c.srv.Diag.Event("proto.too_large", "conn", c.id, "frame", frameName(f), "bytes", len(f.Payload), "takesParts", c.parts.Load())
		return fmt.Errorf("%w: %s of %d bytes", proto.ErrTooLarge, frameName(f), len(f.Payload))
	}
	c.srv.Diag.Every("parts:"+frameName(f), 10*time.Second, "proto.parts", "conn", c.id, "frame", frameName(f), "bytes", len(f.Payload))
	id := c.lastPart.Add(1)
	for part := range proto.Parts(f, id) {
		if err := c.write(part); err != nil {
			return err
		}
	}
	return nil
}

// frameName says which frame f is, for a log: an event's name, never its data.
func frameName(f proto.Frame) string {
	if f.Type == proto.TypeEvent {
		var ev struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(f.Payload, &ev) == nil && ev.Name != "" {
			return "event " + ev.Name
		}
	}
	return f.Type.String()
}

func (c *conn) close() {
	if !c.finished.Load() {
		c.closedBecause("daemon: ended it")
	}
	c.mu.Lock()
	if !c.closed && c.gone != nil {
		close(c.gone)
	}
	c.closed = true
	streams := c.streams
	c.streams = map[uint32]*stream{}
	tunnels := c.tunnels
	c.tunnels = nil
	c.mu.Unlock()
	for _, st := range streams {
		st.stop()
	}
	for _, t := range tunnels {
		t.close(false)
	}
	c.closeADB()
	_ = c.nc.Close()
}
