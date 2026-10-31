package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hooks"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
)

// hookTimeout bounds reporting a state: the agent waits for it.
const hookTimeout = 3 * time.Second

// hookEventTimeout bounds handing over an event: the user's hooks run
// meanwhile, each bounded by its own timeout.
const hookEventTimeout = 10 * time.Minute

// Hook reports the state of the agent in this process's session to the
// daemon, and hands it the agent's events: `enowx hook <state>`, or
// `enowx hook --event E [--state S]` with the agent's JSON of the event on
// stdin (agent.HookArgs). It answers as agents read their hooks: exit
// hooks.ExitBlock with the reason on stderr to refuse, what to add to the
// agent's context on stdout. It never fails the agent otherwise: what
// can't be reported is dropped. It returns the exit code.
func Hook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	id := os.Getenv(session.EnvSession)
	var event, state string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--event" && i+1 < len(args):
			i++
			event = args[i]
		case args[i] == "--state" && i+1 < len(args):
			i++
			state = args[i]
		case !strings.HasPrefix(args[i], "--"):
			state = args[i]
		}
	}
	if id == "" || event == "" && state == "" {
		return 0
	}
	if event == "" {
		withDaemon(hookTimeout, func(c *client.Client) {
			_ = c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: id, State: state}, nil)
		})
		return 0
	}
	data, _ := io.ReadAll(io.LimitReader(stdin, 4<<20))
	ev := agentEvent(event, data)
	var d proto.HookDecision
	withDaemon(hookEventTimeout, func(c *client.Client) {
		err := c.Call(proto.MethodSessionHook, proto.SessionHookParams{ID: id, State: state, Event: ev}, &d)
		if err != nil && state != "" {
			_ = c.Call(proto.MethodSessionState, proto.SessionStateParams{ID: id, State: state}, nil) // an older daemon
		}
	})
	if d.Block && (event == proto.HookToolBefore || event == proto.HookPromptStart) {
		fmt.Fprintln(stderr, d.Reason)
		return hooks.ExitBlock
	}
	if d.Command != "" && event == proto.HookToolBefore {
		if out, ok := rewriteOutput(data, d); ok {
			fmt.Fprintln(stdout, out)
			return 0
		}
	}
	if d.Approve && event == proto.HookToolBefore {
		fmt.Fprintln(stdout, approveOutput())
		return 0
	}
	if d.Context != "" && (event == proto.HookPromptStart || event == proto.HookSessionStart) {
		fmt.Fprintln(stdout, d.Context)
	}
	return 0
}

// withDaemon calls f with a client of this machine's daemon, giving up
// after timeout.
func withDaemon(timeout time.Duration, f func(c *client.Client)) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := dialDaemon()
		if err != nil {
			return
		}
		defer c.Close()
		f(c)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// agentEvent reads event from what an agent's hook gets on stdin (Claude
// Code's and Codex's share their fields): the prompt, the tool, its
// command or the file it touches, and whether it failed.
func agentEvent(event string, data []byte) proto.HookEvent {
	var in struct {
		Event    string          `json:"hook_event_name"`
		Prompt   string          `json:"prompt"`
		Tool     string          `json:"tool_name"`
		Input    json.RawMessage `json:"tool_input"`
		Response json.RawMessage `json:"tool_response"`
		Error    string          `json:"error"`
		Cwd      string          `json:"cwd"`
	}
	_ = json.Unmarshal(data, &in)
	ev := proto.HookEvent{Event: event, Prompt: in.Prompt, Tool: in.Tool, Failed: in.Event == "PostToolUseFailure" || in.Error != ""}
	var input map[string]any
	if json.Unmarshal(in.Input, &input) == nil {
		for _, k := range []string{"command", "cmd"} {
			if v, ok := input[k].(string); ok {
				ev.Command = v
				break
			}
		}
		for _, k := range []string{"file_path", "path", "notebook_path"} {
			if v, ok := input[k].(string); ok && v != "" {
				ev.Paths = append(ev.Paths, relTo(in.Cwd, v))
			}
		}
		if ev.Command == "" && len(ev.Paths) == 0 {
			ev.Command = strings.TrimSpace(string(in.Input))
		}
	}
	return ev
}

// relTo is path p from folder dir, with slashes, when it is inside it.
func relTo(dir, p string) string {
	if dir != "" && filepath.IsAbs(p) {
		if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = rel
		}
	}
	return filepath.ToSlash(p)
}

// Display gives this process's session a virtual display and prints the
// shell command that sends a program's windows there, drawn sharp on the
// user's screen: `eval "$("$ENOWX_BIN" display)"`, which an agent runs
// before starting a desktop app (see agent.Guide).
func Display() error {
	id := os.Getenv(session.EnvSession)
	if id == "" {
		return errors.New("not in an enowX session")
	}
	c, err := dialDaemon()
	if err != nil {
		return err
	}
	defer c.Close()
	var d proto.DisplayInfo
	if err := c.Call(proto.MethodSessionDisplay, proto.SessionRef{ID: id}, &d); err != nil {
		return err
	}
	env := d.Env
	if len(env) == 0 {
		env = []string{"DISPLAY=" + d.Display} // an older daemon
	}
	fmt.Println(exportLine(env))
	fmt.Fprintf(os.Stderr, "Windows opened on %s show in enowX's Browser tool (port %d, display).\n", d.Display, d.Port)
	return nil
}

// dialDaemon connects to the daemon of this machine.
func dialDaemon() (*client.Client, error) {
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

// exportLine is a shell command exporting env (NAME=value entries).
func exportLine(env []string) string {
	var b strings.Builder
	b.WriteString("export")
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		b.WriteString(" " + name + "='" + strings.ReplaceAll(value, "'", `'\''`) + "'")
	}
	return b.String()
}

// rewriteOutput is what tells an agent to run d.Command in place of the
// tool's command it sent (data): its input with the command changed,
// approved when the session skips permissions, else for the user to
// approve. False when the input has no command to change.
func rewriteOutput(data []byte, d proto.HookDecision) (string, bool) {
	var in struct {
		Input map[string]any `json:"tool_input"`
	}
	if json.Unmarshal(data, &in) != nil || in.Input == nil {
		return "", false
	}
	key := ""
	for _, k := range []string{"command", "cmd"} {
		if _, ok := in.Input[k].(string); ok {
			key = k
			break
		}
	}
	if key == "" {
		return "", false
	}
	in.Input[key] = d.Command
	decision := "ask"
	if d.Approve {
		decision = "allow"
	}
	out := map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       decision,
		"permissionDecisionReason": "An enowX hook changed the command to: " + d.Command,
		"updatedInput":             in.Input,
	}}
	b, err := json.Marshal(out)
	return string(b), err == nil
}

// approveOutput tells an agent to run its tool without asking the user:
// the judge allowed it.
func approveOutput() string {
	b, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       "allow",
		"permissionDecisionReason": "enowX's judge allowed it",
	}})
	return string(b)
}
