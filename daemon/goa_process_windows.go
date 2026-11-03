package daemon

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
	"golang.org/x/sys/windows"
)

func (s *Server) goaOwnProcess(args []string, cmd string) bool {
	if len(args) == 0 {
		return false
	}
	// An argument mentioning Codex or enowX isn't the program itself.
	exe, _ := os.Executable()
	path := args[0]
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".exe")
	return cmd != "" && name == strings.TrimSuffix(strings.ToLower(filepath.Base(cmd)), ".exe") ||
		s.HookBin != "" && strings.EqualFold(path, s.HookBin) || exe != "" && strings.EqualFold(path, exe)
}

func goaPlatformProcesses(info proto.SessionInfo) goaProcesses {
	list, _ := procstat.Snapshot()
	out := goaProcesses{starts: map[int]string{}}
	for _, p := range list {
		out.starts[p.PID] = p.Start
	}
	out.known = agent.WindowsGOAProcesses(agent.Base(info.Agent), info.ID, list, procstat.Args, processImage,
		func(pid int) bool { return procstat.Env(pid, session.EnvSession) == info.ID },
		os.Getenv("LOCALAPPDATA"), os.Getenv("SystemRoot"))
	for _, p := range list {
		// Only the daemon's exact launch command is a wrapper. A shell
		// started later by the agent isn't infrastructure.
		if p.PPID == os.Getpid() && procstat.Env(p.PID, session.EnvSession) == info.ID &&
			matchesSessionCommand(procstat.Args(p.PID), info.Command) {
			out.known[p.PID] = true
		}
	}
	return out
}

func matchesSessionCommand(argv, command []string) bool {
	return len(argv) > 0 && len(command) > 0 &&
		strings.EqualFold(filepath.Base(argv[0]), filepath.Base(command[0])) && slices.Equal(argv[1:], command[1:])
}

// processImage queries the executable, rather than trusting argv[0].
func processImage(pid int) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	b := make([]uint16, 32768)
	n := uint32(len(b))
	if windows.QueryFullProcessImageName(h, 0, &b[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(b[:n])
}
