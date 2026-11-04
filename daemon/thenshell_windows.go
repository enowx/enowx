package daemon

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/shells"
)

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

// programGone says what session root runs was thenShell's, and its argv has ended: PowerShell
// staying on has no child, and one running another shell after it has that shell as its child.
func programGone(root int, kids map[int][]int, args func(int) []string) bool {
	line := args(root)
	if !slices.ContainsFunc(line, func(a string) bool { return strings.HasPrefix(a, psLegacy) }) {
		return false
	}
	if slices.Contains(line, "-NoExit") {
		return len(kids[root]) == 0
	}
	command := line[len(line)-1]
	_, after, ok := strings.Cut(command, "; & ")
	if !ok || len(kids[root]) == 0 {
		return len(kids[root]) == 0
	}
	child := args(kids[root][0])
	return len(child) > 0 && strings.Contains(strings.ToLower(after), strings.ToLower(filepath.Base(child[0])))
}
