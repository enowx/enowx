//go:build !windows

package tools

import (
	"os/exec"
	"syscall"

	"github.com/enowx/enowx/internal/quietcmd"
)

// shellCommand is command under bash, or sh where there's none, in a process group of
// its own so killGroup takes all it started.
func shellCommand(command string) (*exec.Cmd, error) {
	shell := "/bin/sh"
	if path, err := exec.LookPath("bash"); err == nil {
		shell = path
	}
	cmd := quietcmd.Command(shell, "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, nil
}

// killGroup ends the command and all in its group. The group outlives its leader while
// any member lives, so this also reaches what a shell that already ended left behind.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
