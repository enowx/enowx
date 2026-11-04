package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestArgv(t *testing.T) {
	cases := []struct {
		agent string
		l     Launch
		want  []string
	}{
		{"claude", Launch{Prompt: "fix it", SkipPermissions: true}, []string{"claude", "--dangerously-skip-permissions", "fix it"}},
		{"claude", Launch{SkipPermissions: true, Continue: true}, []string{"claude", "--continue", "--dangerously-skip-permissions"}},
		{"codex", Launch{SkipPermissions: true, Continue: true}, []string{"codex", "resume", "--last", "--dangerously-bypass-approvals-and-sandbox"}},
		{"gemini", Launch{Prompt: "hi"}, []string{"gemini", "--prompt-interactive", "hi"}},
		{"opencode", Launch{Prompt: "hi", SkipPermissions: true}, []string{"opencode", "--auto", "--prompt", "hi"}},
		{"kiro", Launch{SkipPermissions: true, Continue: true, Prompt: "ignored when continuing"}, []string{"kiro-cli", "chat", "--resume", "--trust-all-tools"}},
		{"copilot", Launch{Prompt: "hi", SkipPermissions: true}, []string{"copilot", "--yolo", "-i", "hi"}},
		{"grok", Launch{Prompt: "-v looks like a flag"}, []string{"grok", "--", "-v looks like a flag"}},
		{"hermes", Launch{Prompt: "hi"}, []string{"hermes", "--tui", "--query", "hi"}},
		{"rovo", Launch{SkipPermissions: true}, []string{"acli", "rovodev", "run", "--yolo"}},
		// Typed in once it shows its input, not on its command line.
		{"amp", Launch{Prompt: "hi", SkipPermissions: true}, []string{"amp", "--dangerously-allow-all"}},
		{"omp", Launch{SkipPermissions: true, Continue: true}, []string{"omp", "--continue", "--auto-approve"}},
		{"omp", Launch{Prompt: "test", Resume: "sess-123"}, []string{"omp", "--resume", "sess-123"}},
		{"omp", Launch{Continue: true, SessionDir: "/s/1"}, []string{"omp", "--continue", "--session-dir", "/s/1"}},
		// An agent without a folder of its own for them keeps its own.
		{"pi", Launch{SessionDir: "/s/1"}, []string{"pi"}},
	}
	for _, tc := range cases {
		p, ok := Find(tc.agent)
		if !ok {
			t.Fatalf("no preset %s", tc.agent)
		}
		if got := p.Argv(tc.l); !slices.Equal(got, tc.want) {
			t.Errorf("%s %+v: argv = %q, want %q", tc.agent, tc.l, got, tc.want)
		}
	}
}

func TestPresetsAreDistinct(t *testing.T) {
	seen := map[string]bool{Shell: true, Custom: true}
	for _, p := range Presets {
		if seen[p.ID] || p.Name == "" || p.Command == "" || p.Homepage == "" {
			t.Errorf("preset %+v: repeated ID, or no name, command or homepage", p)
		}
		seen[p.ID] = true
		if p.TypePrompt && len(p.PromptFlag) > 0 {
			t.Errorf("%s both types its prompt and passes it", p.ID)
		}
	}
	if len(Presets) < 40 {
		t.Fatalf("only %d agents", len(Presets))
	}
}

func TestEnvironSkipsThroughTheEnvironment(t *testing.T) {
	goose, _ := Find("goose")
	if env := goose.Environ(true); !slices.Contains(env, "GOOSE_MODE=auto") {
		t.Fatalf("skipping: %q", env)
	}
	if env := goose.Environ(false); len(env) != 0 {
		t.Fatalf("not skipping: %q", env)
	}
}

// The migration tool installed as goose isn't taken for the agent, whatever its version
// prints, and the agent is found even with that tool first on the PATH.
func TestNamesakeIsNotTheAgent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a shell script")
	}
	write := func(dir, help string) {
		t.Helper()
		script := "#!/bin/sh\nprintf '%s\\n' '" + help + "'\n"
		if err := os.WriteFile(filepath.Join(dir, "goose"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tool, real := t.TempDir(), t.TempDir()
	write(tool, "Usage: goose DRIVER DBSTRING [OPTIONS] COMMAND\nDrivers:\n  postgres\nCommands:\n  up\n  down")
	write(real, "An AI agent\nCommands:\n  session  Start or resume interactive chat sessions")
	goose, _ := Find("goose")

	t.Setenv("PATH", tool)
	if goose.Installed() {
		t.Error("the migration tool counted as installed")
	}
	if got := goose.NamesakeHere(); got != filepath.Join(tool, "goose") {
		t.Errorf("namesake: %q", got)
	}

	t.Setenv("PATH", tool+string(os.PathListSeparator)+real)
	if !goose.Installed() || goose.NamesakeHere() != "" || goose.Program() != filepath.Join(real, "goose") {
		t.Errorf("the agent behind it wasn't found: %q", goose.Program())
	}

	t.Setenv("PATH", real)
	if !goose.Installed() || goose.Program() != "goose" {
		t.Error("the agent wasn't confirmed")
	}
}

func TestProgramOf(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"/usr/local/bin/codex", "--yolo"}, "codex"},
		{[]string{"node", "--no-warnings", "/x/node_modules/.bin/opencode"}, "opencode"},
		{[]string{"bun", "/x/.bin/omp", "--session-dir", "/s"}, "omp"},
		{[]string{`C:\\bin\\claude.exe`}, "claude"},
		{[]string{"claude", "mcp", "serve"}, ""},
		{[]string{"codex", "mcp-server"}, ""},
		{[]string{"node", "server.js"}, ""},
		{[]string{"vite"}, ""},
		{[]string{"node"}, ""},
		{nil, ""},
	} {
		if got := ProgramOf(c.args); got != c.want {
			t.Errorf("%v: %q", c.args, got)
		}
	}
}
