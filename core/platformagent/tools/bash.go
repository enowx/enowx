package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	bashDefaultTimeout = 2 * time.Minute
	bashMaxTimeout     = 10 * time.Minute
	bashOutputMost     = 30000
	// How long after the shell ends its output may still arrive: what it left running
	// in the background holds the pipe open, and the call doesn't wait for that.
	bashDrainWait = 250 * time.Millisecond
	// How long a killed command may take to go before the call stops waiting for it.
	bashKillWait = 5 * time.Second
)

const bashSchema = `{"type":"object","properties":{"command":{"type":"string","description":"The command to run"},"description":{"type":"string","description":"What it does, in a few words"},"timeout":{"type":"integer","minimum":1,"maximum":600000,"description":"Milliseconds it may take (default 120000, at most 600000)"}},"required":["command"],"additionalProperties":false}`

type bashTool struct{}

func (bashTool) Name() string { return "Bash" }
func (bashTool) Description() string {
	return "Run a command with bash in the working folder (on Windows without bash, PowerShell or cmd). Returns stdout and stderr together, keeping the last 30000 bytes, and the exit code when not 0. Times out after 2 minutes unless timeout says otherwise. Each call is a new shell. What it leaves running in the background keeps running, its later output unseen."
}
func (bashTool) Schema() json.RawMessage { return json.RawMessage(bashSchema) }
func (bashTool) ReadOnly() bool          { return false }
func (bashTool) Edits() bool             { return false }

type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

func (bashTool) Detail(input json.RawMessage) string {
	in, err := decode[bashInput](input)
	if err != nil {
		return "Bash"
	}
	return oneLine(in.Command, 300)
}

// bashTimeout is how long a call may run for the milliseconds asked.
func bashTimeout(ms int) time.Duration {
	if ms <= 0 {
		return bashDefaultTimeout
	}
	return min(time.Duration(ms)*time.Millisecond, bashMaxTimeout)
}

func (bashTool) Run(ctx context.Context, input json.RawMessage, env Env) Result {
	in, err := decode[bashInput](input)
	if err != nil {
		return failf("%v", err)
	}
	if strings.TrimSpace(in.Command) == "" {
		return failf("command is required")
	}
	if ctx.Err() != nil {
		return failf("interrupted")
	}
	cmd, err := shellCommand(in.Command)
	if err != nil {
		return failf("%v", err)
	}
	cmd.Dir = env.Dir
	cmd.Env = env.Environ
	if cmd.Env == nil {
		// os/exec reads nil as "inherit", which would hand over the agent's key.
		cmd.Env = CleanEnviron(os.Environ())
	}
	r, w, err := os.Pipe()
	if err != nil {
		return failf("run: %v", err)
	}
	cmd.Stdout, cmd.Stderr = w, w // one pipe keeps the two in order
	err = cmd.Start()
	w.Close()
	if err != nil {
		r.Close()
		return failf("run: %v", err)
	}
	return superviseShell(ctx, cmd, r, env.Progress, bashTimeout(in.Timeout))
}

// superviseShell owns a started command until the call ends: this goroutine alone
// holds its output and calls progress, so progress never runs after the call returned.
func superviseShell(ctx context.Context, cmd *exec.Cmd, r *os.File, progress func(string), timeout time.Duration) Result {
	stop := make(chan struct{})
	defer close(stop)
	chunks := readChunks(r, stop)
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	expired := deadline.C

	var (
		out     tailBuffer
		partial []byte // the start of a character cut between two reads
		ended   string // set once the command is being killed: why
		waitErr error
		exited  bool
		cutoff  <-chan time.Time // ends the call whatever else is pending
		done    = ctx.Done()
	)
	kill := func(why string) {
		ended = why
		killGroup(cmd)
		done, expired = nil, nil
		cutoff = time.After(bashKillWait)
	}
loop:
	for {
		select {
		case b, ok := <-chunks:
			if !ok {
				chunks = nil
				if exited {
					break loop
				}
				continue
			}
			out.Write(b)
			if progress != nil {
				var text string
				text, partial = wholeChars(partial, b)
				if text != "" {
					progress(text)
				}
			}
		case waitErr = <-waited:
			exited = true
			if chunks == nil {
				break loop
			}
			if ended != "" {
				continue // cutoff is running; the pipe closes as the group goes
			}
			cutoff = time.After(bashDrainWait)
		case <-done:
			kill("interrupted")
		case <-expired:
			kill(fmt.Sprintf("timed out after %s", timeout))
		case <-cutoff:
			break loop
		}
	}
	return bashResult(&out, waitErr, ended, exited, chunks != nil)
}

// readChunks passes on what r gives until it ends. After stop it goes on reading and
// dropping: what the command left running would die of a broken pipe at its next write.
func readChunks(r *os.File, stop <-chan struct{}) <-chan []byte {
	chunks := make(chan []byte)
	go func() {
		defer r.Close()
		defer close(chunks)
		buf := make([]byte, 32<<10)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				select {
				case chunks <- append([]byte(nil), buf[:n]...):
				case <-stop:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return chunks
}

func bashResult(out *tailBuffer, waitErr error, ended string, exited, leftRunning bool) Result {
	text := out.String()
	note := func(s string) {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "[" + s + "]"
	}
	if ended != "" {
		note(ended + ": killed")
		return Result{Text: text, Failed: true}
	}
	r := Result{}
	var exit *exec.ExitError
	switch {
	case !exited || waitErr == nil:
	case errors.As(waitErr, &exit) && exit.ExitCode() >= 0:
		note(fmt.Sprintf("exit code %d", exit.ExitCode()))
		r.Failed = true
	default:
		note(waitErr.Error())
		r.Failed = true
	}
	if leftRunning {
		note("still running in the background, its later output unseen")
	}
	if text == "" {
		text = "(no output)"
	}
	r.Text = text
	return r
}

// wholeChars is held+b up to the last whole character, and what to hold for the next.
func wholeChars(held, b []byte) (string, []byte) {
	all := append(held, b...)
	end := len(all)
	for i := 1; i <= 3 && i <= len(all); i++ {
		at := len(all) - i
		if utf8.RuneStart(all[at]) {
			if !utf8.FullRune(all[at:]) {
				end = at
			}
			break
		}
	}
	return strings.ToValidUTF8(string(all[:end]), "\uFFFD"), append([]byte(nil), all[end:]...)
}

// tailBuffer keeps the last bashOutputMost bytes of what is written. One goroutine owns it.
type tailBuffer struct {
	data []byte
	cut  int64
}

func (t *tailBuffer) Write(b []byte) {
	t.data = append(t.data, b...)
	if len(t.data) >= 2*bashOutputMost {
		drop := len(t.data) - bashOutputMost
		t.cut += int64(drop)
		t.data = append(t.data[:0], t.data[drop:]...)
	}
}

func (t *tailBuffer) String() string {
	data, cut := t.data, t.cut
	if len(data) > bashOutputMost {
		drop := len(data) - bashOutputMost
		data, cut = data[drop:], cut+int64(drop)
	}
	if cut == 0 {
		return strings.ToValidUTF8(string(data), "\uFFFD")
	}
	for len(data) > 0 && !utf8.RuneStart(data[0]) {
		data, cut = data[1:], cut+1
	}
	return fmt.Sprintf("[%d bytes of earlier output cut]\n%s", cut, strings.ToValidUTF8(string(data), "\uFFFD"))
}
