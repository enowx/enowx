package platformagent

import (
	"fmt"
	"syscall"
)

// HardenProcess keeps other programs of this user, the tools' commands first among them,
// out of this process: non-dumpable, its /proc/PID/environ and mem are root's and it
// can't be attached to. It holds the API key in memory and, on a key given by hand, in
// the environment it started with, which unsetting a variable doesn't change.
func HardenProcess() error {
	if _, _, errno := syscall.Syscall(syscall.SYS_PRCTL, syscall.PR_SET_DUMPABLE, 0, 0); errno != 0 {
		return fmt.Errorf("platformagent: PR_SET_DUMPABLE: %w", errno)
	}
	return nil
}
