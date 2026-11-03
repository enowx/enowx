//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// placeLauncher writes the launcher in data folder dir for exe, at each start of the
// daemon, and gives its path.
func placeLauncher(dir, exe string) (string, error) {
	if exe == "" {
		return "", fmt.Errorf("no program to start")
	}
	path := filepath.Join(dir, "bin", "enowx")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(launcherScript(exe)), 0o700); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// launcherScript runs exe, or the same app where it is usually kept once exe has
// gone from where it was.
func launcherScript(exe string) string {
	places := []string{shellWord(exe)}
	if i := strings.Index(exe, ".app/"); i >= 0 {
		inner := exe[i+len(".app"):]
		app := filepath.Base(exe[:i+len(".app")])
		places = append(places,
			shellWord(filepath.Join("/Applications", app)+inner),
			`"$HOME"/`+shellWord(filepath.Join("Applications", app)+inner))
	}
	var b strings.Builder
	b.WriteString("#!/bin/sh\n# enowX's program, wherever it is now. The daemon writes this at each start.\n")
	fmt.Fprintf(&b, "for exe in %s; do\n", strings.Join(places, " "))
	b.WriteString("\tif [ -x \"$exe\" ]; then exec \"$exe\" \"$@\"; fi\ndone\n")
	fmt.Fprintf(&b, "echo %s >&2\nexit 127\n", shellWord("enowX: its program is no longer at "+exe+". Open the app again from where it is now."))
	return b.String()
}

// shellWord is s as one word of a shell's command line.
func shellWord(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
