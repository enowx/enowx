// Package rpccount counts what goes over a client's connection, for tests saying how
// often a method is called and how much comes back.
package rpccount

import (
	"io"
	"regexp"
	"sync"
)

var method = regexp.MustCompile(`"method":"([^"]+)"`)

// Conn is a connection counting the calls written to it and the bytes read from it.
type Conn struct {
	io.ReadWriteCloser
	mu    sync.Mutex
	calls map[string]int
	read  int64
}

// Wrap counts on rw.
func Wrap(rw io.ReadWriteCloser) *Conn { return &Conn{ReadWriteCloser: rw, calls: map[string]int{}} }

func (c *Conn) Write(p []byte) (int, error) {
	if m := method.FindSubmatch(p); m != nil {
		c.mu.Lock()
		c.calls[string(m[1])]++
		c.mu.Unlock()
	}
	return c.ReadWriteCloser.Write(p)
}

func (c *Conn) Read(p []byte) (int, error) {
	n, err := c.ReadWriteCloser.Read(p)
	c.mu.Lock()
	c.read += int64(n)
	c.mu.Unlock()
	return n, err
}

// Calls is how many times method was called.
func (c *Conn) Calls(method string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[method]
}

// BytesRead is how many bytes came back.
func (c *Conn) BytesRead() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.read
}
