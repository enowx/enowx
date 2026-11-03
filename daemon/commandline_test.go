package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/rag"
	"github.com/enowx/enowx/proto"
)

// Windows' cmd.exe stops at 8,191 characters, and an agent's own .cmd takes some.
const windowsCommandMax = 8000

// sessionGuide is what session id's agent was told, from its folder.
func sessionGuide(t *testing.T, srv *Server, id string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(srv.agentFilesDir(id), "system-prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// realGuide is a guide of real size: every feature's, as a session has, with GOA's
// and a project's long rules.
func realGuide(goa bool) string {
	guide := agent.Guide + "\n\n" + strings.Repeat("A rule of this project, as long as they get. ", 300)
	if goa {
		guide = agent.GOAGuide + guide
	}
	return guide
}

// useWindowsLimit has this test run under Windows' limit, on any system.
func useWindowsLimit(t *testing.T) {
	t.Helper()
	was := commandLimit
	commandLimit = 7500
	t.Cleanup(func() { commandLimit = was })
}

// Every agent's command, as a worker and as GOA, as the Notes and Canvas pages', fits
// Windows' command line with a guide of real size. That they wouldn't inline is why.
func TestCommandsFitOnWindows(t *testing.T) {
	useWindowsLimit(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	if n := len(realGuide(false)); n <= windowsCommandMax {
		t.Fatalf("the guide is %d characters: this tests nothing", n)
	}
	for _, preset := range agent.Presets {
		dir := filepath.Join(t.TempDir(), "s1")
		for what, chat := range map[string]chatFor{
			"worker": {guide: realGuide(false), files: dir},
			"goa":    {guide: realGuide(true), files: dir, goa: true},
			"notes":  {notes: "n1", guide: realGuide(false), files: dir},
			"canvas": {canvas: "c1", dirs: []string{`C:\Users\someone\projects\one`, `C:\Users\someone\projects\two`}, guide: realGuide(false), files: dir},
		} {
			for _, cont := range []bool{false, true} {
				l := agentLaunch(preset, true, "", cont, chat)
				if n := commandLength(preset.Argv(l)); n > windowsCommandMax {
					t.Errorf("%s as %s: %d characters", preset.ID, what, n)
				}
			}
		}
	}
}

// Claude Code gets the same text and tools from files as it had inline, on every
// system, and nothing long stays on its command line.
func TestClaudeCodeReadsItsArgumentsFromFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	preset, _ := agent.Find("claude")
	dir := t.TempDir()
	guide := realGuide(false)
	l := agentLaunch(preset, false, "", false, chatFor{guide: guide, files: dir})
	if slices.Contains(l.Hooks, "--append-system-prompt") {
		t.Fatalf("inline: %.200q", l.Hooks)
	}
	data, err := os.ReadFile(filepath.Join(dir, "system-prompt.md"))
	if err != nil || string(data) != guide {
		t.Fatalf("the file holds %d bytes of %d, %v", len(data), len(guide), err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "mcp.json"))
	if err != nil || !strings.Contains(string(cfg), `"mcpServers"`) {
		t.Fatalf("MCP: %s, %v", cfg, err)
	}
	if i := slices.Index(l.Hooks, "--mcp-config"); i < 0 || l.Hooks[i+1] != filepath.Join(dir, "mcp.json") {
		t.Fatalf("MCP's flag: %.300q", l.Hooks)
	}
}

// A first prompt too long for the command line is typed once the agent shows its input,
// whole; it stays on the command line where there's no such limit.
func TestLongPromptIsTyped(t *testing.T) {
	useWindowsLimit(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	prompt := strings.Repeat("Do this part of the task, carefully. ", 400)
	l, err := launchFor(proto.SessionCreateParams{Agent: "claude", Prompt: prompt}, chatFor{guide: realGuide(false), files: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if n := commandLength(l.command); n > windowsCommandMax+len(psLegacy) && strings.Contains(strings.Join(l.command, " "), "Do this part") {
		t.Fatalf("the prompt is on a command of %d characters", n)
	}
	if !strings.Contains(l.typed, strings.TrimSpace(prompt)) {
		t.Fatalf("typed: %.100q", l.typed)
	}
	short, err := launchFor(proto.SessionCreateParams{Agent: "claude", Prompt: "A short one"}, chatFor{guide: realGuide(false), files: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if short.typed != "" || !strings.Contains(strings.Join(short.command, " "), "A short one") {
		t.Fatalf("a short prompt: typed %q", short.typed)
	}
}

// An agent with no file form still starts under the limit: told where its guide is,
// which the session's note says.
func TestAgentWithNoFileFormIsWarned(t *testing.T) {
	useWindowsLimit(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	dir := t.TempDir()
	l, err := launchFor(proto.SessionCreateParams{Agent: "qwen-code"}, chatFor{guide: realGuide(false), files: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(l.warn, "read it from a file") {
		t.Fatalf("no warning: %q", l.warn)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "system-prompt.md")); string(data) != realGuide(false) {
		t.Fatalf("the file holds %d bytes", len(data))
	}
	// With no limit, it keeps its guide inline.
	commandLimit = 0
	l, err = launchFor(proto.SessionCreateParams{Agent: "qwen-code"}, chatFor{guide: realGuide(false), files: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if l.warn != "" || !strings.Contains(strings.Join(l.command, " "), "A rule of this project") {
		t.Fatalf("shortened with no limit: %q", l.warn)
	}
}

// Without a picked shell, PowerShell runs the agent and stays: no second command of an
// empty shell's after it, the `& '""'` Windows users saw.
func TestPsThenShell(t *testing.T) {
	argv := []string{"claude", "--x"}
	never := func(string) []string { t.Fatal("asked for a shell's command"); return nil }
	got := psThenShell("", argv, never)
	if got[0] != "powershell.exe" || !slices.Contains(got, "-NoExit") || strings.Contains(got[len(got)-1], `'""'`) {
		t.Errorf("no shell picked: %q", got)
	}
	for _, shell := range []string{`C:\Program Files\PowerShell\7\pwsh.exe`, `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`} {
		if got := psThenShell(shell, argv, never); got[0] != shell || !slices.Contains(got, "-NoExit") {
			t.Errorf("%s: %q", shell, got)
		}
	}
	got = psThenShell(`C:\Windows\System32\cmd.exe`, argv, func(s string) []string { return []string{s} })
	if got[0] != "powershell.exe" || slices.Contains(got, "-NoExit") || !strings.HasSuffix(got[len(got)-1], `; & 'C:\Windows\System32\cmd.exe'`) {
		t.Errorf("another shell: %q", got)
	}
}

// The guides enowX writes itself pass the secret check whole; a secret in the user's
// rules goes no further than the guide's text.
func TestGuideKeepsNoSecret(t *testing.T) {
	for _, g := range []string{agent.Guide, agent.DefaultGuide, agent.GOAGuide, agent.NotesGuide, agent.CanvasChatGuide + agent.CanvasGuide} {
		if rag.HoldsSecret(g) {
			t.Fatalf("a built-in guide looks like it holds a secret: %.80q", g)
		}
		if guideWithoutSecrets(g) != g {
			t.Fatal("a built-in guide changed")
		}
	}
	secret := "AKIA" + "ABCDEFGHIJKLMNOP"
	t.Setenv("HOME", t.TempDir())
	preset, _ := agent.Find("claude")
	dir := t.TempDir()
	agentLaunch(preset, false, "", false, chatFor{guide: "Deploy with " + secret + " please", files: dir})
	data, err := os.ReadFile(filepath.Join(dir, "system-prompt.md"))
	if err != nil || strings.Contains(string(data), secret) || !strings.Contains(string(data), "Deploy with") {
		t.Fatalf("%q, %v", data, err)
	}
}

// Through the daemon: a session's files are written in its own folder under the data
// directory, none in its project, anew when it continues, and gone when it's closed.
func TestSessionFilesFollowTheSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	project := t.TempDir()
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: project, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(srv.agentFilesDir(info.ID), "system-prompt.md")
	if fi, err := os.Stat(path); err != nil || (fi.Mode().Perm() != 0o600) {
		t.Fatalf("the guide's file: %v, %v", fi, err)
	}
	if entries, _ := os.ReadDir(project); len(entries) != 0 {
		t.Fatalf("left in the project: %v", entries)
	}

	// Removed meanwhile, as a cleaner might: continuing writes it again.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := watch(st)
	seen.waitFor(t, "$ ") // the agent ended, its shell stays
	if _, err := st.Write([]byte("exit\r")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the session ended", func() bool {
		sess, err := srv.sessions.Get(info.ID)
		return err == nil && sess.Info().Exited
	})
	if _, err := srv.continueSession(info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("after continuing: %v", err)
	}

	if err := srv.deleteSession(info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(srv.agentFilesDir(info.ID)); !os.IsNotExist(err) {
		t.Fatalf("after closing: %v", err)
	}
}
