//go:build !windows

package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The launcher runs the program, and goes on working once a bundle has moved.
func TestLauncherRunsTheProgram(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "it's here", "program")
	if err := os.MkdirAll(filepath.Dir(exe), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho ran \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := placeLauncher(dir, exe)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(path, "hook").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "ran hook" {
		t.Fatalf("%v: %s", err, out)
	}
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	out, err = exec.Command(path, "hook").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "no longer at") {
		t.Fatalf("with the program gone: %v: %s", err, out)
	}
}

// A bundle's launcher also looks in the places such an app is kept.
func TestLauncherLooksInApplications(t *testing.T) {
	s := launcherScript("/tmp/old/enowX.app/Contents/MacOS/enowX")
	for _, want := range []string{"'/tmp/old/enowX.app/Contents/MacOS/enowX'", "'/Applications/enowX.app/Contents/MacOS/enowX'", `"$HOME"/'Applications/enowX.app/Contents/MacOS/enowX'`} {
		if !strings.Contains(s, want) {
			t.Errorf("no %s in\n%s", want, s)
		}
	}
}
