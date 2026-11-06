package main

import (
	"slices"
	"testing"

	"github.com/enowx/enowx/core/procstat"
)

// Only this program's other apps are ended: not itself, its daemon, a hook or the
// command line, nor another program, nor one whose arguments can't be read.
func TestOtherAppsAreThisProgramsApps(t *testing.T) {
	procs := []procstat.Proc{
		{PID: 1, Name: "enowX.exe"},        // this app
		{PID: 2, Name: "enowX.exe"},        // an older app
		{PID: 3, Name: "enowx.exe"},        // an older app, another case
		{PID: 4, Name: "enowX.exe"},        // its command line
		{PID: 5, Name: "enowx-daemon.exe"}, // the daemon
		{PID: 6, Name: "enowX.exe"},        // a hook
		{PID: 7, Name: "notepad.exe"},      // another program
		{PID: 8, Name: "enowX.exe"},        // arguments unknown
		{PID: 9, Name: "enowX.exe"},        // an app opened with a flag
	}
	args := map[int][]string{
		1: {`C:\enowX\enowX.exe`}, 2: {`C:\enowX\enowX.exe`}, 3: {`C:\enowX\enowx.exe`},
		4: {`C:\enowX\enowX.exe`, "ls"}, 5: {`C:\enowX\enowx-daemon.exe`, "serve"},
		6: {`C:\enowX\enowX.exe`, "hook", "--event", "x"}, 7: {`notepad.exe`},
		9: {`C:\enowX\enowX.exe`, "-remote", "u@h"},
	}
	got := otherApps(procs, 1, "enowX.exe", func(pid int) []string { return args[pid] })
	if want := []int{2, 3, 9}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
