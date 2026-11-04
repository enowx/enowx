//go:build !windows

package hooks

import (
	"os/exec"
	"strings"
	"syscall"
)

func shellCommand(command string) *exec.Cmd {
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// killGroup ends the command and what it started.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// shellQuote quotes s for the shell that runs hooks' commands.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// quotable says shellQuote keeps s as it is.
func quotable(string) bool { return true }
