package daemon

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/enowx/enowx/proto"
)

// Calls this slow or this large are logged, each method at most once in rpcLogEvery.
const (
	rpcLogSlow  = 2 * time.Second
	rpcLogLarge = 4 << 20
	rpcLogEvery = 30 * time.Second
)

// logged says this connection's opening and ending are logged: an app's, or a
// client from before Kind. A command's are too many to say anything, unless it fails.
func (c *conn) logged() bool { return !c.self && c.kind != "cli" }

// closedBecause says why this side is ending the connection; the first reason stays.
func (c *conn) closedBecause(why string) { c.closer.CompareAndSwap(nil, &why) }

// answer runs req and replies, noting a call slow or large, and a panic before it
// takes the daemon down.
func (c *conn) answer(req proto.Request) {
	began := time.Now()
	defer func() {
		if r := recover(); r != nil {
			c.srv.Diag.Event("proto.panic", "conn", c.id, "method", req.Method)
			c.srv.Diag.Flush()
			panic(r) // its stack goes to the log as the program ends (diag.Log.Start)
		}
	}()
	value, err := c.handle(req)
	size := c.reply(req.ID, value, err)
	took := time.Since(began)
	if took >= rpcLogSlow || size >= rpcLogLarge || len(req.Params) >= rpcLogLarge {
		c.srv.Diag.Every("rpc:"+req.Method, rpcLogEvery, "rpc.slow", "conn", c.id, "method", req.Method, "took", took, "replyBytes", size, "paramBytes", len(req.Params))
	}
}

// logClosed logs the connection's end: how long it lasted, who ended it and why.
func (c *conn) logClosed(err error) {
	if c.opened.IsZero() {
		c.srv.Diag.Every("conn.failed", 10*time.Second, "conn.failed", "remote", c.remote, "err", err)
		return
	}
	who, why := closedWho(c.closer.Load(), err)
	if !c.logged() && who == "client" {
		return
	}
	c.srv.Diag.Event("conn.closed", "conn", c.id, "kind", c.kind, "lasted", time.Since(c.opened), "by", who, "why", why)
}

// closedWho says who ended a connection, from why this side did, if it did, and what
// its read loop ended with.
func closedWho(ours *string, err error) (who, why string) {
	if ours != nil {
		return "daemon", *ours
	}
	switch {
	case err == nil, errors.Is(err, io.EOF):
		return "client", "EOF"
	case errors.Is(err, net.ErrClosed):
		return "daemon", err.Error()
	}
	return "connection", fmt.Sprint(err)
}
