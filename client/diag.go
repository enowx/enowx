package client

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"sync/atomic"
	"time"
)

// Kind says what connects, to the daemon's log: the app sets it to "app" as it starts.
var Kind = "cli"

// CallTrace is one call: its size, never what it holds.
type CallTrace struct {
	Method    string
	Sent, Got int
	Took      time.Duration
	Err       error
}

// Trace, when set, is told of every call as it ends, on the caller's goroutine: it
// must not block.
var Trace atomic.Pointer[func(CallTrace)]

// ID names this connection in both sides' logs.
func (c *Client) ID() string { return c.id }

// Since is how long the connection has been up.
func (c *Client) Since() time.Duration { return time.Since(c.opened) }

// ClosedBy says who ended a connection that has ended, and how: "client", "daemon", or
// "connection" when it failed on the way.
func (c *Client) ClosedBy() string {
	switch err := c.Err(); {
	case c.closing.Load():
		return "client"
	case err == nil:
		return ""
	case errors.Is(err, io.EOF):
		return "daemon"
	}
	return "connection"
}

func newConnID() string {
	id := make([]byte, 6)
	_, _ = rand.Read(id)
	return hex.EncodeToString(id)
}
