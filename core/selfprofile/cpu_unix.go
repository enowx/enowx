//go:build !windows

package selfprofile

import (
	"os"
	"syscall"
	"time"
)

func pid() int { return os.Getpid() }

// cpuTime is the time this process has used, user and system.
func cpuTime() time.Duration {
	var u syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &u) != nil {
		return 0
	}
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}
