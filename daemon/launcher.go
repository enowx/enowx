package daemon

import (
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/enowx/enowx/proto"
)

// A session keeps the path of enowX's program for as long as it lives, in its
// environment and in the hooks and MCP servers its agent started with. Were that the
// app's own path, moving or replacing the bundle under a running daemon would fail
// all of them with "No such file or directory". So they get a launcher in the data
// folder (launcher_unix.go), whose path never changes.

// programLauncher is the launcher's path once Run placed it.
var programLauncher atomic.Pointer[string]

// programPath is what a session's agent runs to reach enowX: the launcher, or this
// program where there is none (Windows, whose daemon runs from a copy in the data
// folder, and tests).
func programPath() string {
	if p := programLauncher.Load(); p != nil {
		return *p
	}
	exe, _ := os.Executable()
	return exe
}

// useLauncher has sessions reach the program through a launcher in data folder dir,
// and says what to give them: exe itself if it can't be placed.
func useLauncher(dir, exe string, warn func(msg string, args ...any)) string {
	path, err := placeLauncher(dir, exe)
	if err != nil {
		warn("the program's launcher can't be written: sessions use its path as it is", "err", err)
		return exe
	}
	if path != exe {
		programLauncher.Store(&path)
	}
	return path
}

// linkTarget is what a link standing for enowX's program points at, for one
// whose name says what to do: the launcher would lose that name.
func (s *Server) linkTarget() string {
	if p := programLauncher.Load(); p != nil && *p == s.HookBin {
		if exe, err := os.Executable(); err == nil {
			return exe
		}
	}
	return s.HookBin
}

// bundleLook is how often the daemon checks that its program is still there.
const bundleLook = time.Minute

// watchProgram tells the user once when the daemon's program goes from where it
// started: sessions go on through the launcher, but the daemon can't restart from
// there.
func (s *Server) watchProgram(exe string) {
	if exe == "" {
		return
	}
	tick := time.NewTicker(bundleLook)
	defer tick.Stop()
	told := false
	for {
		select {
		case <-s.done:
			return
		case <-tick.C:
		}
		_, err := os.Stat(exe)
		if err == nil {
			told = false
			continue
		}
		if !told && os.IsNotExist(err) {
			told = true
			s.logger().Warn("the daemon's program moved", "from", exe)
			s.broadcast(proto.EventNotifySound, proto.NotifySoundParams{Kind: proto.NotifyInput,
				Text: "enowX was moved from " + appBundleOf(exe) + " while it runs. Open it from where it is now, ideally Applications, to pick up from there."})
		}
	}
}

// appBundleOf is exe's app bundle, or exe where it has none.
func appBundleOf(exe string) string {
	if i := strings.Index(exe, ".app/"); i >= 0 {
		return exe[:i+len(".app")]
	}
	return exe
}
