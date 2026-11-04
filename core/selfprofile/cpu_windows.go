package selfprofile

import (
	"os"
	"syscall"
	"time"
)

func pid() int { return os.Getpid() }

// cpuTime is the time this process has used, user and system.
func cpuTime() time.Duration {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	var created, exited, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil {
		return 0
	}
	return time.Duration(kernel.Nanoseconds() + user.Nanoseconds())
}
