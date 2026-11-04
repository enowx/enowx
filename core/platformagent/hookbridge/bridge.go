// Package hookbridge hands Claude Platform's events to the daemon of the session it
// runs in, over the same RPC `enowx hook` uses: GOD of Agents' rules, the judge and the
// user's hooks apply to it as to any agent, without a program started per event.
package hookbridge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/platformagent"
	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
)

const (
	// envSession names the session a program runs in (session.EnvSession).
	envSession = "ENOWX_SESSION"
	// shortTimeout bounds what a turn doesn't wait on the answer of, as `enowx hook`
	// bounds reporting a state.
	shortTimeout = 3 * time.Second
	// decisionTimeout bounds asking about a tool call: the user's hooks run meanwhile,
	// as long as Claude Code gives them (agent.waitTimeout).
	decisionTimeout = 300 * time.Second
)

// Conn is the part of a daemon's client the bridge uses.
type Conn interface {
	Call(method string, params, out any) error
	Close() error
}

// Bridge implements platformagent.Hooks. Each call has a connection of its own, which
// is what lets a timeout abandon it; the socket is local. Its mode and last message are
// locked: events come from the turn's goroutine and tools running side by side.
type Bridge struct {
	id   string
	goa  bool
	dir  string
	dial func() (Conn, error)

	mu   sync.Mutex
	mode string
	last string
}

// New is the bridge of this process's session, doing nothing outside one.
func New() *Bridge {
	dir, _ := os.Getwd()
	return &Bridge{
		id:   os.Getenv(envSession),
		goa:  os.Getenv(proto.EnvGOA) != "",
		dir:  dir,
		dial: dialDaemon,
	}
}

// NewOver is the bridge of session id reaching its daemon through dial: a test's.
func NewOver(id string, goa bool, dir string, dial func() (Conn, error)) *Bridge {
	return &Bridge{id: id, goa: goa, dir: dir, dial: dial}
}

// dialDaemon connects to the daemon of this machine.
func dialDaemon() (Conn, error) {
	path, err := transport.SocketPath()
	if err != nil {
		return nil, err
	}
	nc, err := transport.DialLocal(path, nil)
	if err != nil {
		return nil, err
	}
	c, err := client.New(nc, nil)
	if err != nil {
		nc.Close()
		return nil, err
	}
	return c, nil
}

// call makes a call within timeout or until ctx ends, closing the connection then.
func (b *Bridge) call(ctx context.Context, timeout time.Duration, method string, params, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := make(chan error, 1)
	conns := make(chan Conn, 1)
	go func() {
		c, err := b.dial()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		if ctx.Err() != nil {
			done <- ctx.Err()
			return
		}
		conns <- c
		done <- c.Call(method, params, out)
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		select {
		case c := <-conns:
			c.Close() // unblocks the call under way
		default:
		}
		return ctx.Err()
	}
}

// event hands the daemon an event, with the state it means ("" for none).
func (b *Bridge) event(ctx context.Context, timeout time.Duration, state string, ev proto.HookEvent) (proto.HookDecision, error) {
	b.mu.Lock()
	ev.PermissionMode = b.mode
	b.mu.Unlock()
	var out proto.HookDecision
	err := b.call(ctx, timeout, proto.MethodSessionHook, proto.SessionHookParams{ID: b.id, State: state, Event: ev}, &out)
	if err != nil {
		if state != "" {
			// An older daemon knows the state alone.
			_ = b.call(ctx, shortTimeout, proto.MethodSessionState, proto.SessionStateParams{ID: b.id, State: state}, nil)
		}
		return proto.HookDecision{}, err
	}
	return out, nil
}

// inSession says there is a daemon to tell.
func (b *Bridge) inSession() bool { return b.id != "" }

// State implements platformagent.Hooks.
func (b *Bridge) State(state string) {
	if !b.inSession() || state == "" {
		return
	}
	_ = b.call(context.Background(), shortTimeout, proto.MethodSessionState, proto.SessionStateParams{ID: b.id, State: state}, nil)
}

// ToolBefore implements platformagent.Hooks. When the daemon can't answer, GOD of
// Agents' session refuses the call, its rules being what keeps it delegating; any other
// session has no decision, and asks the user as usual.
func (b *Bridge) ToolBefore(ctx context.Context, tool string, input []byte) platformagent.HookAnswer {
	if !b.inSession() {
		return platformagent.HookAnswer{}
	}
	ev := b.toolEvent(proto.HookToolBefore, tool, input)
	d, err := b.event(ctx, decisionTimeout, "", ev)
	if err != nil {
		if b.goa {
			return platformagent.HookAnswer{Deny: true, Why: "enowX's daemon couldn't be reached to check this call against GOD of Agents' rules, so it isn't allowed. Try again."}
		}
		return platformagent.HookAnswer{}
	}
	if d.Block {
		return platformagent.HookAnswer{Deny: true, Why: d.Reason}
	}
	return platformagent.HookAnswer{Allow: d.Approve, Context: d.Context, Command: d.Command}
}

// ToolAfter implements platformagent.Hooks.
func (b *Bridge) ToolAfter(ctx context.Context, tool string, input []byte, failed bool) {
	if !b.inSession() {
		return
	}
	ev := b.toolEvent(proto.HookToolAfter, tool, input)
	ev.Failed = failed
	_, _ = b.event(ctx, shortTimeout, proto.StateWorking, ev)
}

// Prompt implements platformagent.Hooks.
func (b *Bridge) Prompt(ctx context.Context, text string) platformagent.HookAnswer {
	if !b.inSession() {
		return platformagent.HookAnswer{}
	}
	d, _ := b.event(ctx, shortTimeout, proto.StateWorking, proto.HookEvent{Event: proto.HookPromptStart, Prompt: text})
	return platformagent.HookAnswer{Deny: d.Block, Why: d.Reason, Context: d.Context}
}

// TurnDone implements platformagent.Hooks: what it returns are GOD of Agents' lines,
// which keep the turn going.
func (b *Bridge) TurnDone(ctx context.Context) string {
	if !b.inSession() {
		return ""
	}
	b.mu.Lock()
	last := b.last
	b.mu.Unlock()
	d, _ := b.event(ctx, shortTimeout, proto.StateDone, proto.HookEvent{Event: proto.HookTurnDone, LastMessage: last})
	return d.Continue
}

// Start says the agent started, before it opens the conversation, and is what the
// user's hooks have for its context.
func (b *Bridge) Start(ctx context.Context) string {
	if !b.inSession() {
		return ""
	}
	d, _ := b.event(ctx, shortTimeout, "", proto.HookEvent{Event: proto.HookSessionStart})
	return d.Context
}

// End says the agent's program is ending.
func (b *Bridge) End() {
	if !b.inSession() {
		return
	}
	_, _ = b.event(context.Background(), shortTimeout, "", proto.HookEvent{Event: proto.HookSessionEnd})
}

// Note reads what events go out with from a record: the mode a Meta says, and the
// answer TurnDone reports as the turn's last message.
func (b *Bridge) Note(r record.Record, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.Type == record.Meta && r.Mode != "" {
		b.mode = r.Mode
	}
	if r.Type == record.Assistant && r.Sidechain == "" && text != "" {
		b.last = text
	}
}

// SetMode says the permission mode the agent starts in.
func (b *Bridge) SetMode(mode string) {
	b.mu.Lock()
	b.mode = mode
	b.mu.Unlock()
}

// toolEvent reads a tool's input as agents' events say it, Claude's fields: its command,
// or the files it touches, or else what it was given.
func (b *Bridge) toolEvent(event, tool string, data []byte) proto.HookEvent {
	ev := proto.HookEvent{Event: event, Tool: tool}
	var input map[string]any
	if json.Unmarshal(data, &input) != nil {
		return ev
	}
	for _, k := range []string{"command", "cmd"} {
		if v, ok := input[k].(string); ok {
			ev.Command = v
			break
		}
	}
	for _, k := range []string{"file_path", "path", "notebook_path"} {
		if v, ok := input[k].(string); ok && v != "" {
			ev.Paths = append(ev.Paths, b.rel(v))
		}
	}
	if ev.Command == "" && len(ev.Paths) == 0 {
		ev.Command = strings.TrimSpace(string(data))
	}
	return ev
}

// rel is path p from the session's folder when it is inside it, with slashes.
func (b *Bridge) rel(p string) string {
	if b.dir != "" && filepath.IsAbs(p) {
		if rel, err := filepath.Rel(b.dir, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = rel
		}
	}
	return filepath.ToSlash(p)
}
