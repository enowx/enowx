//go:build !windows

package daemon

import "syscall"

// stopProcess asks process pid to end, or forces it.
func stopProcess(pid int, force bool) {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(pid, sig)
}
