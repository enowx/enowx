package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/hostcontrol"
	"github.com/enowx/enowx/core/project"
	"github.com/enowx/enowx/proto"
)

// A session controlling sessions and settings of a host (host.call): this
// host's at once, another's through the app, which alone reaches other
// hosts. What can't be undone, asked by an agent, goes through the app
// too, even for this host, for the user to say yes first; with no app
// open, this host's other requests still work. Every request is logged
// (host.controlLog, and the daemon's log).

const (
	controlTimeout = 2 * time.Minute
	confirmTimeout = 5 * time.Minute // the user reads the question
	controlKept    = 200             // the log's last entries kept
)

var controlHandlers = map[string]handler{
	proto.MethodHostControlLog: bare(func(c *conn) (any, error) { return c.srv.control.entries(), nil }),
	proto.MethodAppSettings: with(func(c *conn, p proto.AppSettingsParams) (any, error) {
		raw, err := c.srv.askApp("app.settings", p, controlTimeout)
		if err != nil {
			return nil, fmt.Errorf("app settings: %w", err)
		}
		return raw, nil
	}),
	proto.MethodProjectConfig: with(func(c *conn, p proto.ProjectRef) (any, error) { return c.srv.projectConfig(p.Path) }),
	proto.MethodProjectConfigSave: with(func(c *conn, p proto.ProjectConfig) (any, error) {
		return c.srv.saveProjectConfig(p)
	}),
}

func init() {
	// host.call runs other methods' handlers: set here, as the handlers
	// table can't name a handler that reads it.
	handlers[proto.MethodHostCall] = with(func(c *conn, p proto.HostCallParams) (any, error) { return c.hostCall(p) })
	slowMethods[proto.MethodHostCall] = true
	slowMethods[proto.MethodAppSettings] = true
}

// controlLog keeps the last requests made through host.call.
type controlLog struct {
	mu   sync.Mutex
	last []proto.ControlLogEntry
}

func (l *controlLog) add(e proto.ControlLogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.last = append(l.last, e)
	if n := len(l.last); n > controlKept {
		l.last = append([]proto.ControlLogEntry(nil), l.last[n-controlKept:]...)
	}
}

func (l *controlLog) entries() []proto.ControlLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]proto.ControlLogEntry{}, l.last...)
}

// thisHost says host names this daemon: "", its ID or its name.
func (s *Server) thisHost(host string) bool {
	return host == "" || host == s.daemonID || strings.EqualFold(host, s.userdata.Name)
}

// hostCall does p on its host, as far as hostcontrol lets a session.
func (c *conn) hostCall(p proto.HostCallParams) (result any, err error) {
	s := c.srv
	level, err := hostcontrol.Check(p.Method, "")
	local := s.thisHost(p.Host)
	defer func() {
		e := proto.ControlLogEntry{Time: time.Now(), From: p.From, Method: p.Method, Level: level.String()}
		if !local {
			e.Host = p.Host
		}
		if err != nil {
			e.Err = err.Error()
		}
		s.control.add(e)
		s.logger().Info("control", "from", e.From, "host", e.Host, "method", e.Method, "level", e.Level, "err", e.Err)
	}()
	if err != nil {
		return nil, err
	}
	confirm := level == hostcontrol.Confirm && p.From != ""
	if local && !confirm {
		return c.handle(proto.Request{Method: p.Method, Params: p.Params})
	}
	timeout := controlTimeout
	if confirm {
		timeout = confirmTimeout
	}
	if local {
		p.Host = s.daemonID // the app asks the user, then does it here
	}
	raw, err := s.askApp("host.call", p, timeout)
	switch {
	case errors.Is(err, errNoApp) && local:
		return nil, fmt.Errorf("%s from a session needs the user's yes in the enowX app, and none is connected to this host: open it", p.Method)
	case errors.Is(err, errNoApp):
		return nil, fmt.Errorf("%q is another host: only the enowX app reaches other hosts, and none is connected to this one: open it (this host's sessions work without it)", p.Host)
	case err != nil:
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// projectOf is the project of this host at path.
func (s *Server) projectOf(path string) (string, error) {
	clean := filepath.Clean(path)
	for _, p := range s.projects.List() {
		if filepath.Clean(p.Path) == clean {
			return p.Path, nil
		}
	}
	return "", fmt.Errorf("project settings: %s isn't a project of this host (project add makes it one)", path)
}

func (s *Server) projectConfig(path string) (proto.ProjectConfig, error) {
	dir, err := s.projectOf(path)
	if err != nil {
		return proto.ProjectConfig{}, err
	}
	cfg, err := project.LoadConfig(dir)
	if err != nil {
		return proto.ProjectConfig{}, err
	}
	out := proto.ProjectConfig{Path: dir, Copy: cfg.Worktree.Copy, Setup: cfg.Worktree.Setup}
	if out.Copy == nil {
		out.Copy = []string{}
	}
	return out, nil
}

func (s *Server) saveProjectConfig(p proto.ProjectConfig) (proto.ProjectConfig, error) {
	dir, err := s.projectOf(p.Path)
	if err != nil {
		return proto.ProjectConfig{}, err
	}
	var cfg project.Config
	cfg.Worktree.Copy, cfg.Worktree.Setup = p.Copy, p.Setup
	if err := project.SaveConfig(dir, cfg); err != nil {
		return proto.ProjectConfig{}, err
	}
	return s.projectConfig(dir)
}
