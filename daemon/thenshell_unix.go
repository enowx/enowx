//go:build !windows

package daemon

import (
	"os"
	"runtime"
)

// thenShell runs argv and, once it exits, the shell terminals start with
// (the user's login shell unless they picked another) in the same terminal, as when an agent is started from a terminal app: quitting
// it with Ctrl+C leaves a prompt to type into, not a dead screen.
//
// /bin/sh runs the wrapper whatever the user's shell is (fish takes no
// "$@"). Its INT trap keeps it alive when Ctrl+C kills argv; a trapped
// signal, unlike an ignored one, is back to default in argv. Its HUP trap
// ends it when the terminal goes (its daemon died): a login shell started on a
// terminal hung up gets stuck leaving, and holds that terminal's /dev node, and
// then every program on the machine asking its terminal's name hangs too.
func thenShell(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	shell := defaultShell()
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
		if runtime.GOOS == "darwin" {
			shell = "/bin/zsh"
		}
	}
	return append([]string{"/bin/sh", "-c", `trap : INT; trap 'exit 129' HUP; "$@"; exec "$0" -l`, shell}, argv...)
}

// scriptArgv runs a shell script: a project's worktree setup, say.
func scriptArgv(script string) []string { return []string{"/bin/sh", "-c", script} }
