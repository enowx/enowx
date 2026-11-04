package daemon

import (
	"io"
	"os"
	"time"

	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/core/selfprofile"
	"github.com/enowx/enowx/proto"
)

// logsSweepEvery is how often old logs are looked at, besides when the daemon starts.
const logsSweepEvery = 6 * time.Hour

func init() {
	slowMethods[proto.MethodDaemonDiagnostics] = true // it reads files
	slowMethods[proto.MethodDaemonRestartPlan] = true // it samples processes
}

// daemonLogWriter is where the daemon's logger writes: its log, and its output too
// unless that is the same file, as for a daemon the app started. As a service, its
// output goes to the service manager's journal, and the log was left empty.
func daemonLogWriter(l *diag.Log) io.Writer {
	if out, err := os.Stderr.Stat(); err == nil {
		if file, err := os.Stat(l.Path()); err == nil && os.SameFile(out, file) {
			return l
		}
	}
	return io.MultiWriter(os.Stderr, l)
}

// keepLogsSwept removes what is past the logs' limits until done.
func keepLogsSwept(dir string, done <-chan struct{}, l *diag.Log) {
	sweep := func() {
		if s := diag.Sweep(dir, time.Now()); s.Files > 0 {
			l.Event("logs.swept", "files", s.Files, "bytes", s.Bytes)
		}
	}
	sweep()
	tick := time.NewTicker(logsSweepEvery)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
			sweep()
		}
	}
}

// diagnostics reads this host's logs for a bug report.
func (s *Server) diagnostics() proto.Diagnostics {
	files := diag.Collect(s.dataDir)
	me := selfprofile.Take(selfprofile.Options{Kind: "daemon", Version: Version, Areas: s.profileAreas})
	files = append(files, proto.DiagFile{Name: diag.ProfileFile, Text: diag.Scrub(selfprofile.TextOf(me))})
	return proto.Diagnostics{Version: Version, Files: files}
}
