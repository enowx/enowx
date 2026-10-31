package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/proto"
)

// The app alone reaches other hosts: what an agent asks across hosts
// (copying a project to one, syncing it now) goes to the app attached
// here last (app.serve), which does it and answers.

// appRequests are the apps attached and the requests sent to them,
// waiting for answers.
type appRequests struct {
	mu      sync.Mutex
	serving []*conn // the latest last
	pending map[string]chan proto.AppReply
}

func (r *appRequests) serve(c *conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.serving = append(slices.DeleteFunc(r.serving, func(o *conn) bool { return o == c }), c)
}

// drop forgets c, which closed.
func (r *appRequests) drop(c *conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.serving = slices.DeleteFunc(r.serving, func(o *conn) bool { return o == c })
}

// errNoApp is askApp's error with no app attached.
var errNoApp = errors.New("no enowX app is connected to this host: only the app reaches other hosts, open it")

// askApp has the latest app attached do kind with params, and returns
// its result, waiting at most timeout.
func (s *Server) askApp(kind string, params any, timeout time.Duration) (json.RawMessage, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	r := &s.appReqs
	r.mu.Lock()
	var app *conn
	if n := len(r.serving); n > 0 {
		app = r.serving[n-1]
	}
	r.mu.Unlock()
	if app == nil {
		return nil, errNoApp
	}
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	req := proto.AppRequest{ID: hex.EncodeToString(id), Kind: kind, Params: raw}
	ch := make(chan proto.AppReply, 1)
	r.mu.Lock()
	if r.pending == nil {
		r.pending = map[string]chan proto.AppReply{}
	}
	r.pending[req.ID] = ch
	r.mu.Unlock()
	data, _ := json.Marshal(req)
	ev, _ := json.Marshal(proto.Event{Name: proto.EventAppRequest, Data: data})
	app.send(proto.Frame{Type: proto.TypeEvent, Payload: ev})
	select {
	case rep := <-ch:
		if rep.Error != "" {
			return nil, errors.New(rep.Error)
		}
		return rep.Result, nil
	case <-time.After(timeout):
		r.mu.Lock()
		delete(r.pending, req.ID)
		r.mu.Unlock()
		return nil, errors.New("the app didn't answer in time")
	}
}

func (r *appRequests) reply(rep proto.AppReply) {
	r.mu.Lock()
	ch := r.pending[rep.ID]
	delete(r.pending, rep.ID)
	r.mu.Unlock()
	if ch != nil {
		ch <- rep
	}
}

// daemonIDFile names this daemon's folder for good (DaemonInfo.ID).
const daemonIDFile = "daemon-id"

// loadDaemonID reads the daemon's ID in dir, made the first time.
func loadDaemonID(dir string) string {
	path := filepath.Join(dir, daemonIDFile)
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); len(id) == 16 {
			return id
		}
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(path, []byte(id+"\n"), 0o600)
	return id
}
