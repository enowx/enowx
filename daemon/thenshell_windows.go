package daemon

import "github.com/enowx/enowx/core/shells"

// thenShell runs argv and, once it exits, leaves the shell terminals
// start with running in the same terminal, as when an agent is started
// from a terminal app.
func thenShell(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	return psThenShell(defaultShell(), argv, shells.Argv)
}

// scriptArgv runs a shell script: a project's worktree setup, say.
func scriptArgv(script string) []string { return []string{"cmd", "/c", script} }
