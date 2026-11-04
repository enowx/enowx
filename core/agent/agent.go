// Package agent knows the coding agents enowX can start: how to hand them a
// first prompt, how to skip their permission prompts, and how to continue
// their last conversation after a restart.
package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/internal/quietcmd"
	"github.com/enowx/enowx/proto"
)

// IDs of the built-in presets. Shell runs the login shell; Custom runs a
// command the user types.
const (
	Shell  = "shell"
	Custom = "custom"
)

// ApproveKeys answer an agent asking to use a tool: allow it once, allow
// it from now on, or deny it.
type ApproveKeys struct {
	Allow, Always, Deny string
}

// Preset is a known agent CLI. Where each flag comes from is in
// docs/agents.md.
type Preset struct {
	ID      string
	Name    string
	Command string
	// Aliases are other names the same program installs as; the first on
	// the PATH of Command and Aliases runs.
	Aliases []string
	// Args always follow Command ("chat" for Kiro).
	Args []string
	// Env is added to the agent's environment.
	Env []string
	// PromptFlag precedes the first prompt; nil passes it as the last
	// argument.
	PromptFlag []string
	// TypePrompt types the first prompt into the agent once its screen
	// settles, for agents that take none on their command line.
	TypePrompt bool
	// SkipPermissions makes the agent run every tool without asking;
	// SkipEnv does it through the environment instead, or as well.
	SkipPermissions []string
	SkipEnv         []string
	// AllowSkip offers skipping permissions as a mode to switch to later,
	// without starting in it, so the chat can switch to it.
	AllowSkip []string
	// Continue starts the agent on its most recent conversation in the
	// working directory, which is how sessions survive a daemon restart.
	Continue []string
	// Resume starts the agent on the conversation whose id follows it:
	// a session copied from another host goes on with its own, even when
	// others ran in the same folder since. Empty: Continue serves.
	Resume []string
	// Fork starts the agent on a copy of the conversation whose id takes
	// the "%s", the original left as it is. Empty: it can't, and enowX
	// doesn't fork its sessions.
	Fork []string
	// SessionDir keeps the agent's conversations in the folder that follows
	// it: each session's own, so sessions in one folder don't take each
	// other's when continued.
	SessionDir []string
	// RefusesRootSkip: the agent won't skip permissions as root.
	RefusesRootSkip bool
	// SignIn is the full command line that signs the agent in without a
	// browser on the host (a link and a code to open elsewhere), which is
	// what works on a server; SignInEnv is added to its environment.
	SignIn    []string
	SignInEnv []string
	// Interrupt stops what the agent is doing and gives the prompt back,
	// as its user would press it; "" is Ctrl+C.
	Interrupt string
	// Approve are the keys that answer the agent asking to use a tool, as
	// its user would press them; empty, the chat leaves the answer to its
	// terminal.
	Approve ApproveKeys
	// Install is the official way to install the agent on macOS and Linux,
	// as a POSIX shell command run without root; "" when enowX doesn't
	// know one it has checked.
	Install string
	// Homepage is where the agent's documentation is, to install it from
	// when Install is empty.
	Homepage string
	// Namesake tells the agent from another program that installs under its name.
	Namesake *Namesake
}

// Namesake is another program with an agent's name. The agent is confirmed, not the
// other ruled out: what the other prints changes with its versions.
type Namesake struct {
	Args []string
	// AgentSays is what the agent prints when run with Args.
	AgentSays string
}

// Base is the agent whose ways a preset shares (its hooks, commands,
// settings, conversations): Claude Code's for its teams' preset.
func Base(id string) string {
	if id == "claude-agent-teams" {
		return "claude"
	}
	return id
}

// InterruptKey is what stops the agent's work: Interrupt, else Ctrl+C.
func (p Preset) InterruptKey() string {
	if p.Interrupt != "" {
		return p.Interrupt
	}
	return "\x03"
}

// Find returns the preset with the given ID.
func Find(id string) (Preset, bool) {
	for _, p := range Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// Launch is how to start an agent.
type Launch struct {
	Prompt          string // first prompt
	SkipPermissions bool
	// AllowSkip offers skipping permissions as a mode for later (see
	// Preset.AllowSkip); it does nothing when SkipPermissions is set.
	AllowSkip bool
	Continue  bool // continue the last conversation instead of starting one
	// Resume continues the conversation of this id, where the agent can
	// (Preset.Resume); else the last one, as Continue.
	Resume string
	// Fork, with Resume, goes on from a copy of that conversation
	// (Preset.Fork).
	Fork bool
	// SessionDir is the folder the agent keeps the session's conversations
	// in, where it can (Preset.SessionDir).
	SessionDir string
	// Hooks report the agent's state to enowX (HookArgs).
	Hooks []string
	// Mode starts an agent that AlwaysSkips in this mode, this once and with bypass still
	// one to switch to; "" is bypass, what it always comes back in.
	Mode string
}

// Argv returns the command line that starts the agent as l says.
func (p Preset) Argv(l Launch) []string {
	l = p.holdBypass(l)
	argv := append([]string{p.Program()}, p.Args...)
	switch {
	case l.Resume != "" && l.Fork && len(p.Fork) > 0:
		for _, a := range p.Fork {
			if a == "%s" {
				a = l.Resume
			}
			argv = append(argv, a)
		}
	case l.Resume != "" && len(p.Resume) > 0:
		argv = append(append(argv, p.Resume...), l.Resume)
	case l.Continue || l.Resume != "":
		argv = append(argv, p.Continue...)
	}
	if l.SessionDir != "" && len(p.SessionDir) > 0 {
		argv = append(append(argv, p.SessionDir...), l.SessionDir)
	}
	if l.SkipPermissions {
		argv = append(argv, p.SkipPermissions...)
	} else if l.AllowSkip {
		argv = append(argv, p.AllowSkip...)
	}
	argv = append(argv, l.Hooks...)
	if l.Prompt != "" && !l.Continue && l.Resume == "" && !p.TypePrompt {
		argv = append(append(argv, p.PromptFlag...), l.Prompt)
	}
	return argv
}

// Program is the name the agent runs as on this host: the first of Command and Aliases
// on the PATH that is the agent, else Command.
func (p Preset) Program() string {
	if name := p.found(); name != "" {
		return name
	}
	return p.Command
}

// Installed reports whether the agent's program is on this host's PATH, and is the
// agent rather than a namesake.
func (p Preset) Installed() bool { return p.found() != "" }

// NamesakeHere is the path of another program with the agent's name on this host, the
// reason a session can't start it, when it is all there is of the agent.
func (p Preset) NamesakeHere() string {
	if p.Namesake == nil || p.found() != "" {
		return ""
	}
	for _, name := range append([]string{p.Command}, p.Aliases...) {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// found is how to run the agent here, "" when it isn't: its name, or its path when a
// namesake comes first on the PATH and would run instead.
func (p Preset) found() string {
	for _, name := range append([]string{p.Command}, p.Aliases...) {
		if _, err := exec.LookPath(name); err != nil {
			continue
		}
		if p.Namesake == nil || p.Namesake.confirms(name) {
			return name
		}
		for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
			path, err := exec.LookPath(filepath.Join(dir, name))
			if err == nil && p.Namesake.confirms(path) {
				return path
			}
		}
	}
	return ""
}

// confirms reports whether the program name is the agent.
func (n Namesake) confirms(name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := quietcmd.CommandContext(ctx, name, n.Args...)
	cmd.WaitDelay = time.Second // what it left running mustn't hold the answer
	out, _ := cmd.CombinedOutput()
	return strings.Contains(string(out), n.AgentSays)
}

// Environ is what the agent's environment gets, skipping permissions or
// not.
func (p Preset) Environ(skip bool) []string {
	env := slices.Clone(p.Env)
	if skip {
		env = append(env, p.SkipEnv...)
	}
	return env
}

// List describes every preset, plus Shell and Custom, with whether its
// command is on this host's PATH.
func List() []proto.AgentInfo {
	out := make([]proto.AgentInfo, 0, len(Presets)+2)
	for _, p := range Presets {
		out = append(out, proto.AgentInfo{
			ID: p.ID, Name: p.Name, Command: p.Program(), Installed: p.Installed(),
			CanSkipPermissions: len(p.SkipPermissions)+len(p.SkipEnv) > 0, AlwaysSkips: AlwaysSkips(p.ID), Install: p.Install, Homepage: p.Homepage,
			SignIn: strings.Join(append(slices.Clone(p.SignInEnv), p.SignIn...), " "),
		})
	}
	return append(out,
		proto.AgentInfo{ID: Shell, Name: "Shell", Installed: true},
		proto.AgentInfo{ID: Custom, Name: "Custom command", Installed: true},
	)
}

// CompactFailed says what an agent's screen says of a /compact that failed,
// "" for nothing of the sort. Compacting's success is read from its
// conversation (transcript.Compacted).
func CompactFailed(screen string) string {
	low := strings.ToLower(screen)
	for _, say := range []string{"error during compaction", "compaction failed", "failed to compact", "not enough messages to compact"} {
		if strings.Contains(low, say) {
			return say
		}
	}
	return ""
}

// ShowsPrompt says an agent's screen is at its prompt, where a line typed is
// a message and not an answer to a question it asks. It knows the agents whose
// sessions fork (Preset.Fork).
func ShowsPrompt(id, screen string) bool {
	switch Base(id) {
	case "claude":
		return strings.Contains(screen, "❯") && !strings.Contains(screen, "Do you want") && !strings.Contains(screen, "Esc to cancel")
	case "codex":
		return strings.Contains(screen, "Ask Codex") || strings.Contains(screen, "for shortcuts")
	}
	return false
}
