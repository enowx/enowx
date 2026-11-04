package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/internal/watchdog"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/version"
)

// Run serves the per-user daemon socket until interrupted. Both `enowx
// daemon` and `enowxd serve` call it.
func Run() error {
	dir, err := transport.Dir()
	if err != nil {
		return err
	}
	path, err := transport.SocketPath()
	if err != nil {
		return err
	}
	// One daemon per user: two would each run the saved sessions. The lock
	// goes with the process, after its sessions are hung up.
	lock, err := transport.LockDaemon(dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	l, err := transport.Listen(path)
	if err != nil {
		return err
	}
	logFile := diag.Open(dir, diag.DaemonLog)
	defer logFile.End("daemon")
	logFile.Start("daemon", Version, version.Build())
	log := slog.New(slog.NewTextHandler(daemonLogWriter(logFile), nil))
	useLoginPath(log)
	exe, _ := os.Executable()
	srv, err := newServer(dir, useLauncher(dir, exe, log.Warn))
	if err != nil {
		l.Close()
		return err
	}
	srv.Log, srv.Diag = log, logFile
	go keepLogsSwept(dir, srv.done, logFile)
	go srv.watchProgram(exe)
	dog := watchdog.Start(filepath.Join(dir, "watchdog"), "daemon", nil, nil) // its memory: it has no one thread to stall
	dog.SetDoing(logFile.Doing)
	defer dog.Stop()
	go srv.convertCanvasHistories()
	srv.procs.load(procsFile(dir))
	go func() {
		srv.endProcsLeftBefore()
		srv.watchProcs(srv.done)
	}()
	if exe != "" {
		srv.Hash = fileHash(exe)
	}
	srv.resumeShares()
	srv.migrateAPIsOn = true
	go srv.migrateAPIs() // model APIs set in agents' files become configs
	log.Info("enowxd listening", "socket", path, "version", Version, "pid", os.Getpid())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		l.Close()
	}()
	err = srv.Serve(l)
	srv.Close()
	_ = os.Remove(path)
	log.Info("enowxd stopped")
	return err
}

// fileHash is the SHA-256 of the file at path, "" when unreadable.
func fileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Bridge connects stdin/stdout to the daemon socket, starting the daemon
// (this executable with serveArgs) if it isn't running. It lets a client
// reach the daemon through any byte pipe, such as an SSH session.
func Bridge(serveArgs ...string) error {
	path, err := transport.SocketPath()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	c, err := transport.DialLocal(path, exec.Command(exe, serveArgs...))
	if err != nil {
		return err
	}
	defer c.Close()
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(c, os.Stdin)
		if uc, ok := c.(*net.UnixConn); ok {
			_ = uc.CloseWrite()
		}
		errc <- err
	}()
	go func() {
		_, err := io.Copy(os.Stdout, c)
		errc <- err
	}()
	if err := <-errc; err != nil {
		return fmt.Errorf("bridge: %w", err)
	}
	return nil
}
