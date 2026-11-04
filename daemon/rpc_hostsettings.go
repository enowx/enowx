package daemon

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/enowx/enowx/proto"
)

// maxLogRead bounds how much of the log's end is read for its last lines.
const maxLogRead = 256 << 10

var hostSettingHandlers = map[string]handler{
	proto.MethodSessionEnvList: bare(func(c *conn) (any, error) { return c.srv.vars.list(), nil }),
	proto.MethodSessionEnvSet: with(func(c *conn, p proto.SessionEnvVar) (any, error) {
		if err := c.srv.vars.set(p); err != nil {
			return nil, err
		}
		return c.srv.vars.list(), nil
	}),
	proto.MethodShellList: bare(func(c *conn) (any, error) { return c.srv.shellList(), nil }),
	proto.MethodShellSet: with(func(c *conn, p proto.ShellSetParams) (any, error) {
		if err := c.srv.setShell(p.Path); err != nil {
			return nil, err
		}
		return c.srv.shellList(), nil
	}),
	proto.MethodDaemonLog: with(func(c *conn, p proto.DaemonLogParams) (any, error) {
		return c.srv.logTail(max(p.Lines, 1)), nil
	}),
	proto.MethodDaemonDiagnostics: bare(func(c *conn) (any, error) { return c.srv.diagnostics(), nil }),
	proto.MethodDaemonRestart: bare(func(c *conn) (any, error) {
		// After the answer is out: the app reconnects, which starts the
		// daemon again, and it continues the sessions.
		go func() {
			time.Sleep(300 * time.Millisecond)
			stopSelf()
		}()
		return ok, nil
	}),
}

// logTail is the last lines of the daemon's log.
func (s *Server) logTail(lines int) proto.DaemonLog {
	path := filepath.Join(filepath.Dir(s.WorktreeRoot), "enowxd.log")
	out := proto.DaemonLog{Path: path}
	f, err := os.Open(path)
	if err != nil {
		out.Text = "No log: " + err.Error()
		return out
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > maxLogRead {
		_, _ = f.Seek(st.Size()-maxLogRead, io.SeekStart)
	}
	data, _ := io.ReadAll(f)
	all := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	out.Text = string(bytes.Join(all, []byte("\n")))
	return out
}
