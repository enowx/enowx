// Command enowxd is the enowX daemon for servers. On desktops the app runs
// the same daemon as `enowx daemon`.
//
//	enowxd serve    serve the per-user socket (~/.enowx-app/enowxd.sock)
//	enowxd bridge   pipe stdin/stdout to the socket, starting the daemon if needed
//	enowxd stop     stop the running daemon, ending its sessions
//	enowxd version  print the version
//	enowxd hook S   report an agent's state (run by its hooks)
//	enowxd ls|spawn|send|wait|…   the command line (enowxd help)
package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/cli"
	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/userservice"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/transport"
)

func main() {
	if err := daemon.MoveOldHome(); err != nil {
		fmt.Fprintln(os.Stderr, "enowxd:", err)
	}
	if cli.IsDockerHelper(os.Args[0]) {
		if err := cli.DockerHelper(os.Args[1:], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "enowxd:", err)
			os.Exit(1)
		}
		return
	}
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = daemon.Run()
	case "bridge":
		startService()
		err = daemon.Bridge("serve")
	case "stop":
		err = stop()
	case "version":
		fmt.Println(daemon.Version)
	case "hook":
		os.Exit(daemon.Hook(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	case "display":
		err = daemon.Display()
	default:
		if cli.Is(os.Args[1:]) {
			err = cli.Run(os.Args[1:], os.Stdout)
			break
		}
		fmt.Fprintf(os.Stderr, "usage: enowxd [serve|bridge|stop|version|hook|display|%s]\n", strings.Join(cli.Commands, "|"))
		os.Exit(2)
	}
	if err != nil {
		var exit cli.ExitError
		if !errors.As(err, &exit) {
			fmt.Fprintln(os.Stderr, "enowxd:", err)
		}
		os.Exit(cli.Exit(err))
	}
}

// startService starts the daemon as the user's systemd service when none
// runs, so it outlives reboots; where that can't be, Bridge starts it as a
// process. Why a service didn't start goes to the daemon's log, since
// stdout is the protocol.
func startService() {
	path, err := transport.SocketPath()
	if err != nil {
		return
	}
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		c.Close()
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	ok, err := userservice.Start(exe)
	if err != nil {
		logLine("enowxd: as a service: " + err.Error())
	}
	if !ok || err != nil {
		return
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
			c.Close()
			return
		}
	}
	logLine("enowxd: the service started but its socket didn't show up")
}

// logLine appends line to the daemon's log.
func logLine(line string) {
	dir, err := transport.Dir()
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "enowxd.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, time.Now().Format(time.RFC3339), line)
}

// stop asks the daemon on this machine for its PID and stops it. It works
// with daemons of other versions, which is how upgrades replace them.
func stop() error {
	path, err := transport.SocketPath()
	if err != nil {
		return err
	}
	conn, err := transport.DialLocal(path, nil)
	if err != nil {
		return nil // nothing is running
	}
	c, err := client.New(conn, nil)
	if err != nil {
		return err
	}
	info, err := c.Info()
	c.Close()
	if err != nil {
		return err
	}
	return transport.StopDaemon(info.PID, path)
}
