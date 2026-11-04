//go:build windows

package mcpclient

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/enowx/enowx/internal/quietcmd"
)

// ownGroup makes cmd start a process group of its own, keeping quietcmd's flags.
func ownGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

// terminateGroup is killGroup: a console-less process takes no polite signal here.
func terminateGroup(cmd *exec.Cmd) { killGroup(cmd) }

// killGroup ends the program and what it started.
func killGroup(cmd *exec.Cmd) {
	_ = quietcmd.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}

// envKey is a variable's name as the system compares it.
func envKey(name string) string { return strings.ToUpper(name) }
