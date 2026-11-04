package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/enowx/enowx/proto"
)

// browsers hands browser.act's actions to the app serving Browsers and
// waits for its replies. Safe from any goroutine.
type browsers struct {
	mu      sync.Mutex
	serving []*conn // apps that serve Browsers, the latest last
	pending map[string]chan proto.BrowserReply
}

// browserTimeout bounds an action: loading a page included.
const browserTimeout = 45 * time.Second

func (b *browsers) serve(c *conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.serving = append(slices.DeleteFunc(b.serving, func(o *conn) bool { return o == c }), c)
}

// drop forgets c, which closed.
func (b *browsers) drop(c *conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.serving = slices.DeleteFunc(b.serving, func(o *conn) bool { return o == c })
}

func (b *browsers) reply(r proto.BrowserReply) {
	b.mu.Lock()
	ch := b.pending[r.ID]
	delete(b.pending, r.ID)
	b.mu.Unlock()
	if ch != nil {
		ch <- r
	}
}

// act has the latest serving app do a, and returns what it replied.
func (s *Server) browserAct(a proto.BrowserAction) (proto.BrowserResult, error) {
	if _, err := s.sessions.Get(a.Session); err != nil {
		return proto.BrowserResult{}, err
	}
	b := &s.browsers
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	req := proto.BrowserRequest{ID: hex.EncodeToString(id), BrowserAction: a}
	ch := make(chan proto.BrowserReply, 1)
	b.mu.Lock()
	if len(b.serving) == 0 {
		b.mu.Unlock()
		if s.renderer() != nil {
			return proto.BrowserResult{}, errors.New("the enowX app only watches this host, and a Browser needs it showing this host: the user has to switch to this host in the app")
		}
		return proto.BrowserResult{}, errors.New("no enowX app is connected to this host to show the Browser: the user has to open the app")
	}
	app := b.serving[len(b.serving)-1]
	if b.pending == nil {
		b.pending = map[string]chan proto.BrowserReply{}
	}
	b.pending[req.ID] = ch
	b.mu.Unlock()
	raw, _ := json.Marshal(req)
	ev, _ := json.Marshal(proto.Event{Name: proto.EventBrowserRequest, Data: raw})
	app.send(proto.Frame{Type: proto.TypeEvent, Payload: ev})
	select {
	case r := <-ch:
		if r.Error != "" {
			return r.Result, errors.New(r.Error)
		}
		return r.Result, nil
	case <-time.After(browserTimeout):
		b.mu.Lock()
		delete(b.pending, req.ID)
		b.mu.Unlock()
		return proto.BrowserResult{}, errors.New("the app didn't answer in time")
	}
}
