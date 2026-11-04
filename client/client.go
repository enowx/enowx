// Package client talks to an enowxd daemon over any byte stream: a local
// socket or an SSH session.
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/version"
)

// Version is the client's version, reported in Hello.
var Version = version.String

// ErrClosed is returned by calls on a closed client.
var ErrClosed = errors.New("client: connection closed")

// Client is a connection to a daemon. It is safe for concurrent use.
type Client struct {
	rw io.ReadWriteCloser
	// Protocol is the protocol version the connection speaks: the lower
	// of the app's and the daemon's (proto.Negotiate).
	Protocol int

	wmu sync.Mutex

	mu      sync.Mutex
	nextID  uint64
	pending map[uint64]chan proto.Response
	streams map[uint32]*Stream
	tunnels map[uint32]*Tunnel
	// gone are the tunnels closed here, whose late frames are dropped.
	gone map[uint32]bool
	// early holds frames for streams whose attach response hasn't been
	// processed yet: the daemon may send frames before the caller registers.
	early   map[uint32][]proto.Frame
	onEvent func(proto.Event)
	// listener, as Listen set it, hears events too.
	listener atomic.Pointer[func(proto.Event)]
	// parts: the daemon reads a frame too large for one as parts; lastPart
	// numbers those sent.
	parts    bool
	lastPart atomic.Uint32
	// id names this connection in both sides' logs; opened is when it did;
	// closing, that this side ended it.
	id      string
	opened  time.Time
	closing atomic.Bool
	// adb are the ADB bridges listening on this daemon, by listener.
	adb  map[uint32]func(id uint32)
	err  error
	done chan struct{}
	// tunnel reaches the daemon's host's ports, for a daemon on another
	// machine (SetTunnel); nil for one on this machine.
	tunnel func(addr string) (net.Conn, error)
}

// SetTunnel says how to reach TCP addresses on the daemon's host, which
// is another machine. Set it before using the client.
func (c *Client) SetTunnel(dial func(addr string) (net.Conn, error)) { c.tunnel = dial }

// Remote reports whether the daemon runs on another machine, whose ports
// DialHost reaches through a tunnel.
func (c *Client) Remote() bool { return c.tunnel != nil }

// DialHost connects to addr as the daemon's host sees it: "127.0.0.1:3000"
// is a port on that machine.
func (c *Client) DialHost(addr string) (net.Conn, error) {
	if c.tunnel != nil {
		return c.tunnel(addr)
	}
	return net.Dial("tcp", addr)
}

// New performs the handshake over rw and starts reading. onEvent, if not
// nil, is called for each daemon event on the client's read goroutine; it
// must not block.
func New(rw io.ReadWriteCloser, onEvent func(proto.Event)) (*Client, error) {
	c := &Client{
		rw:      rw,
		pending: map[uint64]chan proto.Response{},
		streams: map[uint32]*Stream{},
		early:   map[uint32][]proto.Frame{},
		onEvent: onEvent,
		done:    make(chan struct{}),
	}
	c.id = newConnID()
	hello, _ := json.Marshal(proto.Hello{Protocol: proto.Version, Agent: "enowx/" + Version, Parts: true, Conn: c.id, Kind: Kind})
	if err := c.write(proto.Frame{Type: proto.TypeHello, Payload: hello}); err != nil {
		rw.Close()
		return nil, fmt.Errorf("client: handshake: %w", err)
	}
	f, err := proto.ReadFrame(rw)
	if err != nil {
		rw.Close()
		return nil, fmt.Errorf("client: handshake: %w", err)
	}
	if f.Type != proto.TypeHello {
		rw.Close()
		return nil, fmt.Errorf("client: handshake: unexpected %s frame", f.Type)
	}
	var theirs proto.Hello
	if err := json.Unmarshal(f.Payload, &theirs); err != nil {
		rw.Close()
		return nil, fmt.Errorf("client: handshake: %w", err)
	}
	ok := false
	if c.Protocol, ok = proto.Negotiate(theirs.Protocol); !ok {
		rw.Close()
		return nil, fmt.Errorf("client: the daemon speaks protocol %d, older than %d: reconnect to update it", theirs.Protocol, proto.MinVersion)
	}
	c.parts = theirs.Parts
	c.opened = time.Now()
	go c.read()
	return c, nil
}

// Listen has fn called for each event from now on, on the read goroutine: it must not
// block. For what connects before knowing it will want them, as a command does.
func (c *Client) Listen(fn func(proto.Event)) { c.listener.Store(&fn) }

// Done is closed when the connection ends.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err returns why the connection ended.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close closes the connection. Sessions keep running in the daemon.
func (c *Client) Close() error {
	c.closing.Store(true)
	return c.rw.Close()
}

// Call sends a request and decodes its result into out (which may be nil).
func (c *Client) Call(method string, params, out any) (err error) {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		raw = b
	}
	ch := make(chan proto.Response, 1)
	got := 0
	if t := Trace.Load(); t != nil {
		began := time.Now()
		defer func() { (*t)(CallTrace{Method: method, Sent: len(raw), Got: got, Took: time.Since(began), Err: err}) }()
	}
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return ErrClosed
	}
	c.nextID++
	id := c.nextID
	c.pending[id] = ch
	c.mu.Unlock()

	payload, _ := json.Marshal(proto.Request{ID: id, Method: method, Params: raw})
	if err := c.write(proto.Frame{Type: proto.TypeRequest, Payload: payload}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}
	select {
	case resp := <-ch:
		got = len(resp.Result)
		if resp.Error != "" {
			return fmt.Errorf("%s: %s", method, resp.Error)
		}
		if out != nil && len(resp.Result) > 0 {
			return json.Unmarshal(resp.Result, out)
		}
		return nil
	case <-c.done:
		return ErrClosed
	}
}

// Info returns information about the daemon.
func (c *Client) Info() (proto.DaemonInfo, error) {
	var info proto.DaemonInfo
	err := c.Call(proto.MethodDaemonInfo, nil, &info)
	return info, err
}

// SetDisplayScale tells the daemon the scale of this device's screen,
// which displays started later draw at. Older daemons answer an unknown
// method, which changes nothing.
func (c *Client) SetDisplayScale(scale int) error {
	return c.Call(proto.MethodDisplayScale, proto.DisplayScaleParams{Scale: scale}, nil)
}

// StopDisplay stops the display on port of session id or a tab of it, and
// the app on it with all it started; the session's agent or shell goes on.
func (c *Client) StopDisplay(id string, port int) error {
	return c.Call(proto.MethodSessionDisplayStop, proto.DisplayStopParams{ID: id, Port: port}, nil)
}

// ForkSession starts a new session from a compacted copy of session id's
// conversation, and returns once it is compacted.
func (c *Client) ForkSession(id string) (proto.SessionInfo, error) {
	var info proto.SessionInfo
	err := c.Call(proto.MethodSessionFork, proto.SessionForkParams{ID: id}, &info)
	return info, err
}

// ContinueSession starts ended session id's program again in it: its
// agent on its last conversation, or its shell.
func (c *Client) ContinueSession(id string) (proto.SessionInfo, error) {
	var info proto.SessionInfo
	err := c.Call(proto.MethodSessionContinue, proto.SessionRef{ID: id}, &info)
	return info, err
}

// SessionEnv lists the variables every session on the host gets, their
// values masked.
func (c *Client) SessionEnv() ([]proto.SessionEnvVar, error) {
	var out []proto.SessionEnvVar
	err := c.Call(proto.MethodSessionEnvList, nil, &out)
	return out, err
}

// SetSessionEnv sets a variable every session gets, or with value ""
// removes it; it answers the variables, masked.
func (c *Client) SetSessionEnv(name, value string) ([]proto.SessionEnvVar, error) {
	var out []proto.SessionEnvVar
	err := c.Call(proto.MethodSessionEnvSet, proto.SessionEnvVar{Name: name, Value: value}, &out)
	return out, err
}

// DaemonLog reads the daemon's log's last lines.
func (c *Client) DaemonLog(lines int) (proto.DaemonLog, error) {
	var out proto.DaemonLog
	err := c.Call(proto.MethodDaemonLog, proto.DaemonLogParams{Lines: lines}, &out)
	return out, err
}

// RestartDaemon stops the daemon; connecting again starts it.
func (c *Client) RestartDaemon() error { return c.Call(proto.MethodDaemonRestart, nil, nil) }

// Sessions lists the daemon's sessions.
func (c *Client) Sessions() ([]proto.SessionInfo, error) {
	var list []proto.SessionInfo
	err := c.Call(proto.MethodSessionList, nil, &list)
	return list, err
}

// CreateSession starts a session.
func (c *Client) CreateSession(p proto.SessionCreateParams) (proto.SessionInfo, error) {
	var info proto.SessionInfo
	err := c.Call(proto.MethodSessionCreate, p, &info)
	return info, err
}

// KillSession hangs up a session.
func (c *Client) KillSession(id string) error {
	return c.Call(proto.MethodSessionKill, proto.SessionRef{ID: id}, nil)
}

// Agents lists the agents the daemon's host can start.
func (c *Client) Agents() ([]proto.AgentInfo, error) {
	var list []proto.AgentInfo
	err := c.Call(proto.MethodAgentList, nil, &list)
	return list, err
}

// Projects lists the daemon's projects.
func (c *Client) Projects() ([]proto.ProjectInfo, error) {
	var list []proto.ProjectInfo
	err := c.Call(proto.MethodProjectList, nil, &list)
	return list, err
}

// AddProject registers a folder on the daemon's host as a project.
func (c *Client) AddProject(path string) (proto.ProjectInfo, error) {
	var info proto.ProjectInfo
	err := c.Call(proto.MethodProjectAdd, proto.ProjectRef{Path: path}, &info)
	return info, err
}

// RemoveProject forgets a project; its folder is left untouched.
func (c *Client) RemoveProject(path string) error {
	return c.Call(proto.MethodProjectRemove, proto.ProjectRef{Path: path}, nil)
}

// ArrangeProject puts a project in a group ("" for none), before another
// project ("" for last).
func (c *Client) ArrangeProject(path, group, before string) error {
	return c.Call(proto.MethodProjectArrange, proto.ProjectArrangeParams{Path: path, Group: group, Before: before}, nil)
}

// MoveProjectGroup moves a group's projects before a project of another
// group or none ("" for last).
func (c *Client) MoveProjectGroup(group, before string) error {
	return c.Call(proto.MethodProjectGroupMove, proto.ProjectGroupMoveParams{Group: group, Before: before}, nil)
}

// RenameProjectGroup renames a group; to "" ungroups its projects.
func (c *Client) RenameProjectGroup(from, to string) error {
	return c.Call(proto.MethodProjectGroupRename, proto.ProjectGroupRenameParams{From: from, To: to}, nil)
}

// InspectProject returns a project's branches and whether it uses git.
func (c *Client) InspectProject(path string) (proto.ProjectDetail, error) {
	var d proto.ProjectDetail
	err := c.Call(proto.MethodProjectInspect, proto.ProjectRef{Path: path}, &d)
	return d, err
}

// CreateWorktree creates (or reuses) a worktree.
func (c *Client) CreateWorktree(p proto.WorktreeCreateParams) (proto.WorktreeInfo, error) {
	var info proto.WorktreeInfo
	err := c.Call(proto.MethodWorktreeCreate, p, &info)
	return info, err
}

// StartPipeline starts a pipeline of agents on a task.
func (c *Client) StartPipeline(p proto.PipelineStartParams) (proto.Pipeline, error) {
	var out proto.Pipeline
	err := c.Call(proto.MethodPipelineStart, p, &out)
	return out, err
}

// Pipelines lists the host's pipelines, running or ended.
func (c *Client) Pipelines() ([]proto.Pipeline, error) {
	var out []proto.Pipeline
	err := c.Call(proto.MethodPipelineList, nil, &out)
	return out, err
}

// CancelPipeline stops pipeline id; its sessions stay.
func (c *Client) CancelPipeline(id string) error {
	return c.Call(proto.MethodPipelineCancel, proto.PipelineRef{ID: id}, nil)
}

// RemovePipeline forgets pipeline id, which has ended.
func (c *Client) RemovePipeline(id string) error {
	return c.Call(proto.MethodPipelineRemove, proto.PipelineRef{ID: id}, nil)
}

// Automations lists the host's automations.
func (c *Client) Automations() ([]proto.Automation, error) {
	var out []proto.Automation
	err := c.Call(proto.MethodAutomationList, nil, &out)
	return out, err
}

// SaveAutomation adds an automation (no ID) or changes one.
func (c *Client) SaveAutomation(a proto.Automation) (proto.Automation, error) {
	var out proto.Automation
	err := c.Call(proto.MethodAutomationSave, a, &out)
	return out, err
}

// RemoveAutomation removes automation id.
func (c *Client) RemoveAutomation(id string) error {
	return c.Call(proto.MethodAutomationRemove, proto.AutomationRef{ID: id}, nil)
}

// RunAutomation starts automation id now, in a new session.
func (c *Client) RunAutomation(id string) (proto.SessionInfo, error) {
	var out proto.SessionInfo
	err := c.Call(proto.MethodAutomationRun, proto.AutomationRef{ID: id}, &out)
	return out, err
}

// SetNote sets what session id is at, "" to clear it.
func (c *Client) SetNote(id, note string) error {
	return c.Call(proto.MethodSessionNote, proto.SessionNoteParams{ID: id, Note: note}, nil)
}

// Screen returns the text session id's terminal shows.
func (c *Client) Screen(id string) (string, error) {
	var out proto.SessionScreen
	err := c.Call(proto.MethodSessionScreen, proto.SessionRef{ID: id}, &out)
	return out.Text, err
}

// Input types text into session id: as a paste when paste is set, then
// Enter when enter is.
func (c *Client) Input(id, text string, paste, enter bool) error {
	return c.Call(proto.MethodSessionInput, proto.SessionInputParams{ID: id, Text: text, Paste: paste, Enter: enter}, nil)
}

// Attach attaches to a session and returns its stream, which a terminal
// uses as its Conn.
func (c *Client) Attach(id string) (*Stream, error) {
	var res proto.AttachResult
	if err := c.Call(proto.MethodSessionAttach, proto.SessionRef{ID: id}, &res); err != nil {
		return nil, err
	}
	st := newStream(c, res.Stream, id)
	c.mu.Lock()
	// Queue early frames before the stream becomes visible to the read
	// goroutine, so they stay ahead of the frames it delivers next.
	st.queue = c.early[res.Stream]
	delete(c.early, res.Stream)
	c.streams[res.Stream] = st
	c.mu.Unlock()
	return st, nil
}

// write sends f, as parts when too large for one frame (other frames going
// between them) and the daemon takes them; refused alone otherwise.
func (c *Client) write(f proto.Frame) error {
	if len(f.Payload) <= proto.MaxPayload {
		return c.writeFrame(f)
	}
	if !c.parts || !proto.FitsParts(len(f.Payload)) {
		return fmt.Errorf("%w: %s of %d bytes", proto.ErrTooLarge, f.Type, len(f.Payload))
	}
	id := c.lastPart.Add(1)
	for part := range proto.Parts(f, id) {
		if err := c.writeFrame(part); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) writeFrame(f proto.Frame) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return proto.WriteFrame(c.rw, f)
}

func (c *Client) read() {
	var err error
	var large proto.Assembler // this goroutine's
	for {
		var f proto.Frame
		f, err = proto.ReadFrame(c.rw)
		if err != nil {
			break
		}
		if f.Type == proto.TypePart {
			var done bool
			if f, done, err = large.Take(f); err != nil {
				break
			}
			if !done {
				continue
			}
		}
		switch f.Type {
		case proto.TypeResponse:
			var resp proto.Response
			if json.Unmarshal(f.Payload, &resp) != nil {
				continue
			}
			c.mu.Lock()
			ch := c.pending[resp.ID]
			delete(c.pending, resp.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- resp
			}
		case proto.TypeEvent:
			var ev proto.Event
			switch {
			case json.Unmarshal(f.Payload, &ev) != nil:
			case ev.Name == proto.EventAndroidADBAccepted:
				c.adbAccepted(ev)
			default:
				if fn := c.listener.Load(); fn != nil {
					(*fn)(ev)
				}
				if c.onEvent != nil {
					c.onEvent(ev)
				}
			}
		case proto.TypeTunnelData, proto.TypeTunnelAck, proto.TypeTunnelClose:
			c.mu.Lock()
			t := c.tunnels[f.Channel]
			if t == nil && !c.gone[f.Channel] {
				c.early[f.Channel] = append(c.early[f.Channel], f)
			}
			c.mu.Unlock()
			if t != nil {
				t.deliver(f)
			}
		case proto.TypeOutput, proto.TypeResize:
			c.mu.Lock()
			st := c.streams[f.Channel]
			if st == nil {
				c.early[f.Channel] = append(c.early[f.Channel], f)
			}
			c.mu.Unlock()
			if st != nil {
				st.deliver(f)
			}
		}
	}
	c.mu.Lock()
	c.err = err
	streams := c.streams
	c.streams = map[uint32]*Stream{}
	tunnels := c.tunnels
	c.tunnels = nil
	c.mu.Unlock()
	for _, t := range tunnels {
		t.hangup()
	}
	for _, st := range streams {
		st.hangup()
	}
	close(c.done)
}

func (c *Client) forget(id uint32) {
	c.mu.Lock()
	delete(c.streams, id)
	c.mu.Unlock()
}

// Settings returns the host's settings.
func (c *Client) Settings() (proto.DaemonSettings, error) {
	var st proto.DaemonSettings
	err := c.Call(proto.MethodDaemonSettings, nil, &st)
	return st, err
}

// SetSettings changes the host's settings.
func (c *Client) SetSettings(st proto.DaemonSettings) (proto.DaemonSettings, error) {
	var out proto.DaemonSettings
	err := c.Call(proto.MethodDaemonSetSettings, st, &out)
	return out, err
}

// FanOut gives one prompt to several agents, a session each on a branch
// of its own, grouped to compare (proto.MethodSessionFanOut).
func (c *Client) FanOut(p proto.FanOutParams) ([]proto.SessionInfo, error) {
	var out []proto.SessionInfo
	err := c.Call(proto.MethodSessionFanOut, p, &out)
	return out, err
}

// StopAgents stops every agent running on the host, keeping their
// sessions to continue, and says how many it stopped.
func (c *Client) StopAgents() (int, error) {
	var out proto.StopAgentsResult
	err := c.Call(proto.MethodDaemonStopAgents, nil, &out)
	return out.Stopped, err
}

// SetBudget sets how long session id's agent may work; 0 lifts it.
func (c *Client) SetBudget(id string, budget time.Duration) error {
	return c.Call(proto.MethodSessionBudget, proto.SessionBudgetParams{ID: id, Seconds: int64(budget / time.Second)}, nil)
}

// IsUnknownMethod reports whether err is a daemon's answer to a method it
// doesn't know: it is older than the app.
func IsUnknownMethod(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unknown method")
}

// Explain says what err means to a user. A method the daemon doesn't know
// means it is older than the app, which reconnecting replaces or upgrades.
func Explain(err error) string {
	if err == nil {
		return ""
	}
	if IsUnknownMethod(err) {
		return "The enowX daemon on this host is older than the app and can't do this yet. Reconnect (command palette → Reconnect) to update it."
	}
	return err.Error()
}

// Shells lists the host's shells and the one its terminals start with.
func (c *Client) Shells() (proto.ShellList, error) {
	var l proto.ShellList
	err := c.Call(proto.MethodShellList, nil, &l)
	return l, err
}

// SetShell picks the shell the host's terminals start with; "" the login
// shell.
func (c *Client) SetShell(path string) (proto.ShellList, error) {
	var l proto.ShellList
	err := c.Call(proto.MethodShellSet, proto.ShellSetParams{Path: path}, &l)
	return l, err
}

// RenameSession names session id, "" for the name it takes by itself.
func (c *Client) RenameSession(id, name string) error {
	return c.Call(proto.MethodSessionRename, proto.SessionRenameParams{ID: id, Name: name}, nil)
}

// AccountsInfo is the host's agents' accounts, their state and preferences.
func (c *Client) AccountsInfo() (proto.AccountsInfo, error) {
	var out proto.AccountsInfo
	err := c.Call(proto.MethodAccountInfo, nil, &out)
	return out, err
}

// SetAccountPrefs keeps an agent's account preferences.
func (c *Client) SetAccountPrefs(agentID string, p proto.AccountPrefs) (proto.AccountsInfo, error) {
	var out proto.AccountsInfo
	err := c.Call(proto.MethodAccountSetPrefs, proto.AccountPrefsParams{Agent: agentID, Prefs: p}, &out)
	return out, err
}

// SwitchAccount has a session go on as another account, or agent.
func (c *Client) SwitchAccount(p proto.AccountSwitchParams) (proto.SessionInfo, error) {
	var out proto.SessionInfo
	err := c.Call(proto.MethodAccountSwitch, p, &out)
	return out, err
}

// AccountLog is the accounts used, the latest first.
func (c *Client) AccountLog() ([]proto.AccountLogEntry, error) {
	var out []proto.AccountLogEntry
	err := c.Call(proto.MethodAccountLog, nil, &out)
	return out, err
}

// AddKeyAccount adds an account of an agent that is the vault's credential.
func (c *Client) AddKeyAccount(agentID, name, cred string) (proto.Account, error) {
	var out proto.Account
	err := c.Call(proto.MethodAccountAdd, proto.AccountAddParams{Agent: agentID, Name: name, Cred: cred}, &out)
	return out, err
}
