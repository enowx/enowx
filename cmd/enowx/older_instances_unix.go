//go:build !windows

package main

import "syscall"

// askToQuit asks app pid to quit, as the system does at logout.
func askToQuit(pid int) { _ = syscall.Kill(pid, syscall.SIGTERM) }
