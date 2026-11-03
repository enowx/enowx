//go:build !windows

package daemon

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/enowx/enowx/proto"
)

func goaPlatformProcesses(_ proto.SessionInfo) goaProcesses { return goaProcesses{} }

func (s *Server) goaOwnProcess(args []string, cmd string) bool {
	line := strings.Join(args, " ")
	if cmd != "" && strings.Contains(line, cmd) || s.HookBin != "" && strings.Contains(line, s.HookBin) {
		return true
	}
	exe, _ := os.Executable()
	if exe != "" && strings.Contains(line, exe) {
		return true
	}
	name := strings.TrimPrefix(filepath.Base(args[0]), "-")
	return len(args) == 1 && slices.Contains([]string{"sh", "bash", "zsh", "fish", "dash"}, name)
}
