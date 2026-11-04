//go:build windows

package tools

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/enowx/enowx/internal/quietcmd"
)

// shellCommand is command under Git's bash where it's installed, else PowerShell, else
// cmd. System32's bash.exe is WSL's launcher, another machine as far as paths go.
func shellCommand(command string) (*exec.Cmd, error) {
	if path, err := exec.LookPath("bash"); err == nil && !strings.EqualFold(filepath.Base(filepath.Dir(path)), "System32") {
		return quietcmd.Command(path, "-c", command), nil
	}
	if path, err := exec.LookPath("powershell"); err == nil {
		return quietcmd.Command(path, "-NoProfile", "-NonInteractive", "-Command", command), nil
	}
	path, err := exec.LookPath("cmd")
	if err != nil {
		return nil, fmt.Errorf("no shell found: %w", err)
	}
	cmd := quietcmd.Command(path)
	// cmd has its own quoting: os/exec's escaping of the command would reach it as is.
	cmd.SysProcAttr.CmdLine = `cmd /S /C "` + command + `"`
	return cmd, nil
}

// killGroup ends the command and the tree it started.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = quietcmd.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}
