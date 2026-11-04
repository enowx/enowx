package agent

import (
	"path/filepath"
	"slices"
	"strings"
)

// Flags kept of an agent found running in a terminal when it is brought back: its mode,
// model and permissions. Anything else may be a prompt, already sent, or a secret.
var (
	tabValueFlags = []string{"--model", "-m", "--permission-mode", "--effort", "--settings", "--mcp-config",
		"--append-system-prompt-file", "--system-prompt-file", "--session-dir", "--max-budget", "--add-dir", "--resume", "-r", "--sandbox", "-s", "--ask-for-approval", "-a"}
	tabBareFlags = []string{"--dangerously-skip-permissions", "--allow-dangerously-skip-permissions",
		"--dangerously-bypass-approvals-and-sandbox", "--yolo", "--full-auto"}
	// tabOneShot marks a run that does its work and ends: nothing to bring back.
	tabOneShot = []string{"-p", "--print", "exec", "run"}
)

// TabAgentCommand is the command bringing back the agent running as args in a terminal,
// on its conversation and in its mode (bypass, for an agent that AlwaysSkips); false for what isn't an agent enowX can continue,
// which ends with the terminal. It never holds a prompt.
func TabAgentCommand(args []string) ([]string, bool) {
	if len(args) == 0 {
		return nil, false
	}
	rest := args[1:]
	preset, ok := findByProgram(args[0])
	if p, builtin := findBuiltin(args); builtin {
		preset, ok, rest = p, true, args[2:]
	} else if !ok && len(args) > 1 && slices.Contains([]string{"node", "bun", "deno", "sh", "bash", "python3"}, strings.TrimSuffix(filepath.Base(args[0]), ".exe")) {
		preset, ok = findByProgram(args[1])
		rest = args[2:]
	}
	if !ok || len(preset.Continue) == 0 {
		return nil, false
	}
	var kept []string
	resumes := false
	for i := 0; i < len(rest); i++ {
		flag, value, hasValue := strings.Cut(rest[i], "=")
		switch {
		case slices.Contains(tabOneShot, flag):
			return nil, false
		case slices.Contains(tabBareFlags, flag):
			kept = append(kept, flag)
		case slices.Contains(tabValueFlags, flag):
			resumes = resumes || flag == "--resume" || flag == "-r"
			if hasValue {
				kept = append(kept, flag, value)
			} else if i+1 < len(rest) {
				kept = append(kept, flag, rest[i+1])
				i++
			}
		}
	}
	out := []string{preset.Command}
	if preset.Builtin {
		out = append([]string{preset.Program()}, preset.Args...)
	}
	if !resumes {
		out = append(out, preset.Continue...) // first: some are subcommands
	}
	return TabAgentBypass(append(out, kept...)), true
}

// findBuiltin is the agent args run of enowX's own program.
func findBuiltin(args []string) (Preset, bool) {
	if len(args) < 2 || !strings.HasPrefix(filepath.Base(args[0]), "enowx") {
		return Preset{}, false
	}
	for _, p := range Presets {
		if p.Builtin && slices.Equal(p.Args, args[1:2]) {
			return p, true
		}
	}
	return Preset{}, false
}

// findByProgram is the preset whose command program runs.
func findByProgram(program string) (Preset, bool) {
	base := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(program), ".exe"), ".js")
	for _, p := range Presets {
		if p.Command == base {
			return p, true
		}
	}
	return Preset{}, false
}
