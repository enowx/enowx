package checks

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"sync"
	"time"

	"github.com/enowx/enowx/proto"
)

// Limits of a run: how long it may take, and how much of its output is
// kept (its end, where failures show).
const (
	runTimeout = 15 * time.Minute
	keepOutput = 256 << 10
)

// Runner runs sessions' checks, one at a time per session, and keeps how
// each check's last run went. It is safe for concurrent use; OnChange is
// called, without locks held, when a check starts or ends.
type Runner struct {
	OnChange func(session string)
	// Env is what a session's checks run with besides the daemon's own
	// environment (its traffic caught, say); nil for nothing.
	Env func(session string) []string
	// OnFailed is called, without locks held, with a check's run that just
	// failed (the judge may say whether it is flaky); nil for nothing.
	OnFailed func(session string, run proto.CheckRun)

	mu   sync.Mutex
	runs map[string]*sessionRuns
}

type sessionRuns struct {
	list   []proto.CheckRun
	cancel context.CancelFunc // stops the runs under way
	gen    int                // counts runs, so a stopped one doesn't report
}

// output collects a command's output, keeping its end.
type output struct {
	mu  sync.Mutex
	buf []byte
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	o.buf = append(o.buf, p...)
	if len(o.buf) > keepOutput {
		o.buf = append([]byte("…\n"), o.buf[len(o.buf)-keepOutput:]...)
	}
	o.mu.Unlock()
	return len(p), nil
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return string(o.buf)
}

// Run runs checks in dir for session, one after the other, stopping those
// still under way from before.
func (r *Runner) Run(session, dir string, checks []proto.Check) {
	if len(checks) == 0 {
		return
	}
	var env []string
	if r.Env != nil {
		env = r.Env(session)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	if r.runs == nil {
		r.runs = map[string]*sessionRuns{}
	}
	sr := r.runs[session]
	if sr == nil {
		sr = &sessionRuns{}
		r.runs[session] = sr
	}
	if sr.cancel != nil {
		sr.cancel()
	}
	sr.gen++
	gen := sr.gen
	sr.cancel = cancel
	for _, c := range checks {
		i := slices.IndexFunc(sr.list, func(run proto.CheckRun) bool { return run.Name == c.Name })
		run := proto.CheckRun{Name: c.Name, Command: c.Command, State: proto.CheckQueued}
		if i < 0 {
			sr.list = append(sr.list, run)
		} else {
			sr.list[i] = run
		}
	}
	r.mu.Unlock()
	r.changed(session)

	go func() {
		defer cancel()
		for _, c := range checks {
			if ctx.Err() != nil {
				break
			}
			out := &output{}
			started := time.Now()
			r.set(session, gen, c.Name, func(run *proto.CheckRun) { run.State, run.Started = proto.CheckRunning, started })
			r.changed(session)
			done := make(chan error, 1)
			go func() { done <- runCommand(ctx, dir, c.Command, out, env) }()
			var err error
		wait:
			for {
				select {
				case err = <-done:
					break wait
				case <-time.After(time.Second):
					// Show the output as it comes.
					r.set(session, gen, c.Name, func(run *proto.CheckRun) { run.Output = out.String() })
				}
			}
			r.set(session, gen, c.Name, func(run *proto.CheckRun) {
				run.Duration, run.Output = time.Since(started).Round(time.Millisecond), out.String()
				var exit *exec.ExitError
				switch {
				case ctx.Err() != nil:
					run.State = proto.CheckStopped
				case err == nil:
					run.State = proto.CheckPassed
				case errors.As(err, &exit):
					run.State, run.ExitCode = proto.CheckFailed, exit.ExitCode()
				default:
					run.State, run.ExitCode = proto.CheckFailed, -1
					run.Output += err.Error() + "\n"
				}
			})
			r.changed(session)
			if r.OnFailed != nil {
				if run, ok := r.find(session, gen, c.Name); ok && run.State == proto.CheckFailed {
					r.OnFailed(session, run)
				}
			}
		}
		// Those that never started were stopped.
		r.mu.Lock()
		if sr.gen == gen {
			for i := range sr.list {
				if sr.list[i].State == proto.CheckQueued {
					sr.list[i].State = proto.CheckStopped
				}
			}
			sr.cancel = nil
		}
		r.mu.Unlock()
		r.changed(session)
	}()
}

// set changes check name's run of session, unless a newer run took over.
func (r *Runner) set(session string, gen int, name string, fn func(*proto.CheckRun)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sr := r.runs[session]
	if sr == nil || sr.gen != gen {
		return
	}
	if i := slices.IndexFunc(sr.list, func(run proto.CheckRun) bool { return run.Name == name }); i >= 0 {
		fn(&sr.list[i])
	}
}

// find is check name's run of session, unless a newer run took over.
func (r *Runner) find(session string, gen int, name string) (proto.CheckRun, bool) {
	var out proto.CheckRun
	found := false
	r.set(session, gen, name, func(run *proto.CheckRun) { out, found = *run, true })
	return out, found
}

// Update changes check name's run of session that started at started, if
// it is still the last; it says whether it was.
func (r *Runner) Update(session, name string, started time.Time, fn func(*proto.CheckRun)) bool {
	r.mu.Lock()
	sr := r.runs[session]
	i := -1
	if sr != nil {
		i = slices.IndexFunc(sr.list, func(run proto.CheckRun) bool { return run.Name == name && run.Started.Equal(started) })
	}
	if i >= 0 {
		fn(&sr.list[i])
	}
	r.mu.Unlock()
	if i >= 0 {
		r.changed(session)
	}
	return i >= 0
}

func (r *Runner) changed(session string) {
	if r.OnChange != nil {
		r.OnChange(session)
	}
}

// Stop stops session's runs under way.
func (r *Runner) Stop(session string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sr := r.runs[session]; sr != nil && sr.cancel != nil {
		sr.cancel()
	}
}

// StopAll stops every session's runs, as the daemon stops.
func (r *Runner) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, sr := range r.runs {
		if sr.cancel != nil {
			sr.cancel()
		}
	}
}

// Results returns how session's checks last went.
func (r *Runner) Results(session string) []proto.CheckRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sr := r.runs[session]; sr != nil {
		return slices.Clone(sr.list)
	}
	return nil
}

// Forget drops session's runs, stopping those under way.
func (r *Runner) Forget(session string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sr := r.runs[session]; sr != nil && sr.cancel != nil {
		sr.cancel()
	}
	delete(r.runs, session)
}

// runCommand runs command in dir through the shell, as the user would
// type it, writing its output to out. It ends with ctx or runTimeout,
// taking the processes it started along.
func runCommand(ctx context.Context, dir, command string, out *output, env []string) error {
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	cmd := shellCommand(command)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = out, out
	// Test runners stop watching and asking when they see CI.
	cmd.Env = append(append(os.Environ(), "CI=true", "NO_COLOR=1", "TERM=dumb"), env...)
	if err := startGroup(cmd); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		killGroup(cmd)
		<-done
		return ctx.Err()
	}
}
