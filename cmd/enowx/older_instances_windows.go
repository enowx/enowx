//go:build windows

package main

import (
	"strconv"

	"github.com/enowx/enowx/internal/quietcmd"
)

// askToQuit asks app pid to close its windows, as closing them by hand does.
func askToQuit(pid int) { _ = quietcmd.Command("taskkill", "/PID", strconv.Itoa(pid)).Run() }
