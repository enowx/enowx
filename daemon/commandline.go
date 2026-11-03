package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/rag"
)

// agentFilesDir is session id's own folder for what its agent's command line would
// otherwise hold (agent.MoveToFiles). The daemon writes it on its host, where the agent
// runs; it's written anew at each start, and goes when the session is closed.
func (s *Server) agentFilesDir(id string) string {
	return filepath.Join(s.dataDir, "agent-files", id)
}

func (s *Server) agentFilesForget(id string) { _ = os.RemoveAll(s.agentFilesDir(id)) }

// guideWithoutSecrets is guide with no secret in it, as it goes in a file and to a model: the
// project's rules and GOA's are the user's text.
func guideWithoutSecrets(guide string) string {
	if !rag.HoldsSecret(guide) {
		return guide
	}
	slog.Warn("a secret in the project's or GOA's rules was kept from the agent's guide")
	return rag.MaskSecrets(guide)
}

// fitCommandLine is the arguments of l, preset's, with what they can hold in files of
// dir; and, where commandLimit stands and they are over it, the guide of an agent with
// no file form replaced by a line naming its file, so that the session still starts.
func fitCommandLine(preset agent.Preset, l agent.Launch, dir string) []string {
	hooks, err := agent.MoveToFiles(preset.ID, dir, l.Hooks)
	if err != nil {
		slog.Warn("could not write the agent's files: its command line holds them", "agent", preset.ID, "err", err)
		hooks = l.Hooks
	}
	l.Hooks = hooks
	if !overCommandLimit(preset.Argv(l)) {
		return hooks
	}
	short, did, err := agent.ShortenGuide(preset.ID, dir, hooks)
	if err != nil {
		slog.Warn("could not write the agent's guide to a file", "agent", preset.ID, "err", err)
		return hooks
	}
	if did {
		slog.Warn("the agent's guide doesn't fit this system's command line: it is told to read a file instead", "agent", preset.ID)
	}
	return short
}

// commandWarning says what is off in how l starts preset, for the session's note.
func commandWarning(preset agent.Preset, l agent.Launch) string {
	argv := preset.Argv(l)
	if overCommandLimit(argv) {
		slog.Warn("the agent's command is too long for this system's command line", "agent", preset.ID, "length", commandLength(argv))
		return fmt.Sprintf("%s's command is %d characters, over what this system's command line takes: it may not start.", preset.Name, commandLength(argv))
	}
	if agent.GuideShortened(l.Hooks) {
		return preset.Name + " takes enowX's guide only on its command line, too short for it here: it is told to read it from a file."
	}
	return ""
}

// overCommandLimit says argv is too long for this system's command line.
func overCommandLimit(argv []string) bool {
	return commandLimit > 0 && commandLength(argv) > commandLimit
}

// commandLength is how long argv's command is, as thenShell gives it to PowerShell.
func commandLength(argv []string) int { return len(psRun(argv)) }

var backslashesBeforeQuote = regexp.MustCompile(`(\\*)(")`)

// psLegacy has PowerShell 7.3+ pass a program its arguments as Windows
// PowerShell 5.1 does, which psQuote escapes for: their own way ("Windows")
// would pass psQuote's escapes on as backslashes. Older PowerShells ignore
// it.
const psLegacy = "$PSNativeCommandArgumentPassing = 'Legacy'; "

// psRun is PowerShell's command that runs argv.
func psRun(argv []string) string { return psLegacy + "& " + psQuote(argv) }

// psThenShell is thenShell for Windows, with shell the one picked ("" for none) and
// shellArgv how it starts. PowerShell runs argv: the one picked, staying on, or
// Windows' own when none is; for another shell (the Command Prompt, Git Bash, WSL),
// Windows' runs it after, as the Command Prompt can't be given a command line safely.
func psThenShell(shell string, argv []string, shellArgv func(string) []string) []string {
	run := psRun(argv)
	if shell == "" {
		return []string{"powershell.exe", "-NoLogo", "-NoExit", "-Command", run}
	}
	name := strings.ToLower(shell[strings.LastIndexAny(shell, `\/`)+1:])
	if name == "pwsh.exe" || name == "powershell.exe" {
		return []string{shell, "-NoLogo", "-NoExit", "-Command", run}
	}
	return []string{"powershell.exe", "-NoLogo", "-Command", run + "; & " + psQuote(shellArgv(shell))}
}

// psQuote is argv as PowerShell's words.
func psQuote(argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		if a == "" {
			quoted[i] = `'""'`
			continue
		}
		// In PowerShell, single quotes are escaped by doubling them (' -> '').
		// When PowerShell 5.1 passes arguments to a native executable, double
		// quotes are stripped unless escaped with \". Any preceding backslashes
		// must also be doubled so they don't escape the backslash.
		s := backslashesBeforeQuote.ReplaceAllString(a, `$1$1\"`)
		s = strings.ReplaceAll(s, "'", "''")
		quoted[i] = "'" + s + "'"
	}
	return strings.Join(quoted, " ")
}
