package agent

import (
	"os"
	"slices"
	"strings"
)

// Every Claude Code session enowX starts or brings back runs in bypassPermissions, whatever
// was asked of it or saved of it: a session that comes back asking stops a piece of work
// nobody is watching. Preset.Argv holds that for every command line enowX builds; Skips and
// TabAgentBypass say the same for what a session records of itself and for an agent found
// running in a terminal. Hooks' refusals still stand there, as bypass doesn't pass them.

const (
	// ModeBypass is Claude Code's name for the mode that asks nothing.
	ModeBypass = "bypassPermissions"
	// modePlan is the mode switching never steps on by the way (Settings.ModeRoute).
	modePlan       = "plan"
	claudeSkipFlag = "--dangerously-skip-permissions"
)

// AsRoot says this process runs as root; tests stand in for it.
var AsRoot = func() bool { return os.Geteuid() == 0 }

// AlwaysSkips says agent id's sessions run in bypass whatever is asked of them, where the
// agent allows it (SkipRefused).
func AlwaysSkips(id string) bool { return Base(id) == "claude" }

// SkipRefused says the agent won't skip permissions here: it refuses as root.
func (p Preset) SkipRefused() bool { return p.RefusesRootSkip && AsRoot() }

// Skips says a session of agent id, asked or not to skip permissions, does.
func Skips(id string, asked bool) bool {
	if !AlwaysSkips(id) {
		return asked
	}
	p, ok := Find(id)
	return ok && !p.SkipRefused()
}

// holdBypass is l as AlwaysSkips has it: in bypass with no other mode named, except for the
// one start Launch.Mode asks for.
func (p Preset) holdBypass(l Launch) Launch {
	if !AlwaysSkips(p.ID) {
		return l
	}
	if p.SkipRefused() {
		l.SkipPermissions, l.AllowSkip = false, false
		l.Hooks = append(slices.Clone(l.Hooks), modeArgs(p.ID, l.Mode)...)
		return l
	}
	l.Hooks = withoutModeFlag(l.Hooks)
	l.SkipPermissions, l.AllowSkip = l.Mode == "", l.Mode != ""
	l.Hooks = append(l.Hooks, modeArgs(p.ID, l.Mode)...)
	return l
}

// modeArgs are the arguments that start agent id in mode, none for "".
func modeArgs(id, mode string) []string {
	var out []string
	if mode == "" {
		return nil
	}
	for _, f := range SettingsOf(id).ModeFlag {
		out = append(out, strings.ReplaceAll(f, "%s", mode))
	}
	return out
}

// withoutModeFlag is args without the flag naming a permission mode.
func withoutModeFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--permission-mode":
			i++
		case strings.HasPrefix(args[i], "--permission-mode="):
		default:
			out = append(out, args[i])
		}
	}
	return out
}

// TabAgentBypass is the command bringing back an agent found running in a terminal
// (TabAgentCommand), in bypass where AlwaysSkips holds. It goes over a command saved by an
// earlier daemon too.
func TabAgentBypass(argv []string) []string {
	if len(argv) == 0 {
		return argv
	}
	p, ok := findByProgram(argv[0])
	if !ok || !AlwaysSkips(p.ID) || p.SkipRefused() {
		return argv
	}
	out := withoutModeFlag(argv)
	out = slices.DeleteFunc(out, func(a string) bool { return a == "--allow-dangerously-skip-permissions" })
	if !slices.Contains(out, claudeSkipFlag) {
		out = append(out, claudeSkipFlag)
	}
	return out
}
