package daemon

import (
	"time"

	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/core/session"
)

// tag is an environment entry that marks the processes something started
// (procstat.Tagged): a session's ENOWX_SESSION, a display's app's
// ENOWX_DISPLAY. sessionless keeps to those no session started.
type tag struct {
	key, value  string
	sessionless bool
}

// reap stops the processes tagged with t that outlive what started them: a
// dev server an agent left in tmux after its session is deleted, a
// display's D-Bus after the display stops. They are asked to end (stopProcess), then
// forced after reapGrace, waited for on a goroutine of its own.
func reap(t tag) { reapBut(t, nil) }

// reapBut is reap sparing the processes in keep.
func reapBut(t tag, keep map[int]bool) {
	if pids := terminate(t, keep); len(pids) > 0 {
		go func() {
			time.Sleep(reapGrace)
			kill(t, pids)
		}()
	}
}

// reapAll is reap for every tag, returning once they are gone: the daemon
// is about to exit.
func reapAll(tags []tag) {
	left := map[tag][]int{}
	for _, t := range tags {
		if pids := terminate(t, nil); len(pids) > 0 {
			left[t] = pids
		}
	}
	if len(left) == 0 {
		return
	}
	time.Sleep(reapGrace)
	for t, pids := range left {
		kill(t, pids)
	}
}

func terminate(t tag, keep map[int]bool) []int {
	var pids []int
	for _, pid := range procstat.Tagged(t.key, t.value) {
		if !keep[pid] && !(t.sessionless && procstat.Env(pid, session.EnvSession) != "") {
			stopProcess(pid, false)
			pids = append(pids, pid)
		}
	}
	return pids
}

// kill ends those of pids that still run and are still tagged, not a new
// process given a pid freed meanwhile.
func kill(t tag, pids []int) {
	for _, pid := range pids {
		if procstat.Env(pid, t.key) == t.value {
			stopProcess(pid, true)
		}
	}
}
