package main

import (
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/version"
)

// An update starts the new app, and an app of the version before can stay running: on
// Windows two did, and each took the daemon back to its own version, over and over
// (issue #49). On its version's first run, the app ends the others of its program left
// running, asking first and forcing after endWait.

// endWait is how long an older app has to quit when asked, before it is ended.
const endWait = 5 * time.Second

// endOlderInstances ends this program's other apps, on this version's first run here.
func endOlderInstances() {
	dir, err := transport.Dir()
	if err != nil {
		return
	}
	marker := filepath.Join(dir, "app-version")
	if prev, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(prev)) == version.String {
		return // this version ran here before: others are the user's
	}
	_ = os.WriteFile(marker, []byte(version.String+"\n"), 0o600)
	exe, err := os.Executable()
	if err != nil {
		return
	}
	procs, err := procstat.Snapshot()
	if err != nil {
		return
	}
	pids := otherApps(procs, os.Getpid(), filepath.Base(exe), procstat.Args)
	for _, pid := range pids {
		log.Printf("enowx: ending an app left running by the update (pid %d)", pid)
		askToQuit(pid)
	}
	deadline := time.Now().Add(endWait)
	for _, pid := range pids {
		for stillRuns(pid) && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
		}
		if stillRuns(pid) {
			if p, err := os.FindProcess(pid); err == nil {
				_ = p.Kill()
			}
		}
	}
}

// otherApps are the processes of program name that are its app, other than self: run
// with no argument, or only flags. Its daemon, hooks and command line take a word.
func otherApps(procs []procstat.Proc, self int, name string, args func(int) []string) []int {
	var out []int
	for _, p := range procs {
		if p.PID == self || !strings.EqualFold(p.Name, name) {
			continue
		}
		argv := args(p.PID)
		if len(argv) == 0 || !strings.EqualFold(programName(argv[0]), name) {
			continue // can't tell what it is: left alone
		}
		if len(argv) > 1 && !strings.HasPrefix(argv[1], "-") {
			continue
		}
		out = append(out, p.PID)
	}
	slices.Sort(out)
	return out
}

// programName is a command line's program without its folder, whichever separator it
// uses.
func programName(path string) string { return path[strings.LastIndexAny(path, `/\`)+1:] }

// stillRuns says process pid is there.
func stillRuns(pid int) bool {
	procs, err := procstat.Snapshot()
	return err == nil && slices.ContainsFunc(procs, func(p procstat.Proc) bool { return p.PID == pid })
}
