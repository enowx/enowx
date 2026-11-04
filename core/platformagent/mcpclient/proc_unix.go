//go:build !windows

package mcpclient

import (
	"os/exec"
	"syscall"
)

// ownGroup makes cmd start a process group of its own, so what it starts ends with it
// and a terminal's Ctrl-C doesn't reach it.
func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func terminateGroup(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }

func killGroup(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

// envKey is a variable's name as the system compares it.
func envKey(name string) string { return name }
