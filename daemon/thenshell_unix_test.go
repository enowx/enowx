//go:build !windows

package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A terminal hanging up, as its daemon dying does, ends the wrapper rather than
// starting a login shell on a dead terminal, which gets stuck and holds it.
func TestHangUpStartsNoShell(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "shell-started")
	shell := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	run := func(program string, hangUp bool) error {
		argv := thenShell([]string{"sh", "-c", program})
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		if hangUp {
			time.Sleep(300 * time.Millisecond)
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP); err != nil {
				t.Fatal(err)
			}
		}
		return cmd.Wait()
	}
	if err := run("exit 0", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("a program ending starts the shell, as it should:", err)
	}
	os.Remove(marker)
	_ = run("sleep 30", true)
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the shell started on a terminal hung up")
	}
}
