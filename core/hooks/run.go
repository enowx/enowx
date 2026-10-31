package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// DefaultTimeout bounds a hook's command that sets none.
const DefaultTimeout = 30 * time.Second

// outputMax is how much of a command's output is kept: the end of it.
const outputMax = 8 << 10

// ExitBlock is the exit code with which a hook's command refuses the
// action, what it printed to stderr being the reason, as agents' own hooks
// do.
const ExitBlock = 2

// Runner is what hooks run with: Env added to their commands'
// environment (or, Clean, all of it), Bin enowX's program
// (HookActionEnowX, HookActionTrim), Notify what shows a notification in
// the apps, Judge what asks the judge about a command (HookActionJudge:
// its decision, and what to log).
type Runner struct {
	Env    []string
	Clean  bool
	Bin    string
	Notify func(kind, text string)
	Judge  func(ev proto.HookEvent) (proto.HookDecision, string)
}

// trimPrefix starts a command HookActionTrim wrapped: it isn't wrapped
// twice.
const trimPrefix = " decide trim -- "

// Run runs hook h on event ev: a refusal or context at once, a command in
// the event's folder with ev as JSON on its stdin and r's environment. A
// command printing and exiting 0 gives context (or, printing
// {"command": …} before a tool, the command to run instead); exiting
// ExitBlock refuses.
func (r Runner) Run(h proto.Hook, ev proto.HookEvent) (proto.HookRun, proto.HookDecision) {
	run := proto.HookRun{Hook: h.ID, Name: h.Name, Event: ev.Event, Session: ev.Session, Agent: ev.Agent, Start: time.Now()}
	var d proto.HookDecision
	switch h.Action {
	case proto.HookActionBlock:
		d.Block, d.Reason = true, h.Reason
		if d.Reason == "" {
			d.Reason = "Refused by the hook “" + name(h) + "”."
		}
		run.Blocked, run.Output = true, d.Reason
	case proto.HookActionContext:
		d.Context, run.Output = h.Context, h.Context
	case proto.HookActionRewrite:
		re, _ := compile(h.Find)
		if re != nil && ev.Command != "" && re.MatchString(ev.Command) {
			d.Command = re.ReplaceAllString(ev.Command, h.Replace)
			run.Output = d.Command
		}
	case proto.HookActionJudge:
		if r.Judge == nil {
			run.Output = "no judge on this host"
			break
		}
		d, run.Output = r.Judge(ev)
		run.Blocked = d.Block
	case proto.HookActionTrim:
		if ev.Command != "" && !strings.Contains(ev.Command, trimPrefix) {
			bin := r.Bin
			if bin == "" {
				bin = "enowx"
			}
			d.Command = shellQuote(bin) + trimPrefix + shellQuote(ev.Command)
			run.Output = d.Command
		}
	case proto.HookActionNotify:
		text := Expand(h.Message, ev)
		if r.Notify != nil {
			r.Notify(kindOr(h.Kind), text)
		}
		run.Output = text
	case proto.HookActionWebhook:
		code, err := webhook(h, ev)
		run.Exit = code
		if u, perr := url.Parse(h.URL); perr == nil {
			run.Output = u.Scheme + "://" + u.Host + "/…" // its path may be its token
		}
		if err != nil {
			run.Error = err.Error()
		}
	case proto.HookActionRun, proto.HookActionEnowX:
		cmd := h.Command
		if h.Action == proto.HookActionEnowX {
			bin := r.Bin
			if bin == "" {
				bin = "enowx"
			}
			cmd = shellQuote(bin) + " " + h.Command
		}
		stdout, stderr, code, err := r.command(cmd, h, ev)
		run.Exit, run.Output = code, tail(strings.TrimSpace(stdout+"\n"+stderr))
		switch {
		case err != nil:
			run.Error = err.Error()
		case code == ExitBlock:
			d.Block, d.Reason, run.Blocked = true, strings.TrimSpace(stderr), true
			if d.Reason == "" {
				d.Reason = strings.TrimSpace(stdout)
			}
			if d.Reason == "" {
				d.Reason = "Refused by the hook “" + name(h) + "”."
			}
		case code == 0:
			out := strings.TrimSpace(stdout)
			var rewrite struct {
				Command *string `json:"command"`
			}
			if strings.HasPrefix(out, "{") && json.Unmarshal([]byte(out), &rewrite) == nil && rewrite.Command != nil {
				d.Command = *rewrite.Command
			} else {
				d.Context = out
			}
		default:
			run.Error = fmt.Sprintf("exited with %d", code)
		}
	}
	run.Ms = time.Since(run.Start).Milliseconds()
	return run, d
}

func kindOr(kind string) string {
	if kind == "" {
		return "done"
	}
	return kind
}

// command runs command line for hook h on ev, within its timeout.
func (r Runner) command(line string, h proto.Hook, ev proto.HookEvent) (stdout, stderr string, code int, err error) {
	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	enc.SetEscapeHTML(false) // commands read as typed: > and & stay
	if err := enc.Encode(ev); err != nil {
		return "", "", -1, err
	}
	timeout := DefaultTimeout
	if h.Timeout > 0 {
		timeout = time.Duration(h.Timeout) * time.Second
	}
	cmd := shellCommand(line)
	if fi, err := os.Stat(ev.Dir); err == nil && fi.IsDir() {
		cmd.Dir = ev.Dir
	}
	env := r.Env
	if !r.Clean {
		env = append(os.Environ(), env...)
	}
	cmd.Env = append(env,
		"ENOWX_HOOK_EVENT="+ev.Event, "ENOWX_HOOK_NAME="+name(h), "ENOWX_HOOK_SESSION="+ev.Session, "ENOWX_HOOK_AGENT="+ev.Agent)
	cmd.Stdin = &in
	cmd.WaitDelay = time.Second // a child still holding its output
	var out, errOut capped
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Start(); err != nil {
		return "", "", -1, fmt.Errorf("start: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(timeout):
		killGroup(cmd)
		<-done
		return out.String(), errOut.String(), -1, fmt.Errorf("timed out after %s", timeout)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.String(), errOut.String(), exit.ExitCode(), nil
	}
	if err != nil {
		return out.String(), errOut.String(), -1, err
	}
	return out.String(), errOut.String(), 0, nil
}

func name(h proto.Hook) string {
	if h.Name != "" {
		return h.Name
	}
	return h.Event
}

// capped keeps what is written up to a bound, dropping the rest.
type capped struct{ b bytes.Buffer }

func (c *capped) Write(p []byte) (int, error) {
	if room := 64<<10 - c.b.Len(); room > 0 {
		c.b.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (c *capped) String() string { return c.b.String() }

// tail is the end of s, at most outputMax.
func tail(s string) string {
	if len(s) <= outputMax {
		return s
	}
	return "…" + s[len(s)-outputMax:]
}
