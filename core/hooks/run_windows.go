//go:build windows

package hooks

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/enowx/enowx/internal/quietcmd"
)

func shellCommand(command string) *exec.Cmd { return quietcmd.Command("cmd", "/C", command) }

// killGroup ends the command and what it started.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = quietcmd.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}

// shellQuote quotes s for cmd, which runs hooks' commands.
func shellQuote(s string) string { return `"` + s + `"` }

// quotable says shellQuote keeps s as it is: cmd has no way to quote these.
func quotable(s string) bool { return !strings.ContainsAny(s, "\"\r\n%") }
