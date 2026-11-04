package daemon

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/agent"
)

// A program started through PowerShell, as thenShell starts agents, gets
// its arguments as they were: quotes, $, backslashes, newlines and empty
// ones, in Windows PowerShell 5.1 and in PowerShell 7 where it is
// installed.
func TestPsQuotePreservesArguments(t *testing.T) {
	cases := [][]string{
		{"--append-system-prompt", agent.Guide},
		{"simple", "words"},
		{"don't", "it's 'great'"},
		{`say "hello"`, `nested "quotes" inside`},
		{`'hello' and "world"`, `"don't" do that`},
		{`eval "$("$ENOWX_BIN" display)"`},
		{"$VAR", "$1", "$@", "${VAR}"},
		{"line 1\nline 2\r\nline 3\ttab"},
		{`path\to\"file"`, `C:\Program Files\app`},
		{"", "next"},
	}
	for _, shell := range []string{"powershell.exe", "pwsh.exe"} {
		if _, err := exec.LookPath(shell); err != nil {
			t.Logf("%s isn't installed", shell)
			continue
		}
		for _, tc := range cases {
			run := psLegacy + "& " + psQuote(append([]string{os.Args[0]}, tc...))
			cmd := exec.Command(shell, "-NoLogo", "-NoProfile", "-Command", run)
			cmd.Env = append(os.Environ(), psArgsHelper+"=1")
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("%s: %v: %s", shell, err, out)
			}
			var got []string
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("%s: the program printed %q: %v", shell, out, err)
			}
			if !reflect.DeepEqual(got, tc) {
				t.Errorf("%s:\n got: %#v\nwant: %#v", shell, got, tc)
			}
		}
	}
}

// The bug itself, on real Windows: an agent installed by npm is a .cmd, which starts
// through cmd.exe. A command inline over its limit fails, and the one enowX builds, a
// GOA's with a guide of real size, gets through with its files named.
func TestAgentThroughCmdShim(t *testing.T) {
	useWindowsLimit(t)
	dir := t.TempDir()
	shim := "@echo off\r\n\"" + os.Args[0] + "\" %*\r\n"
	if err := os.WriteFile(filepath.Join(dir, "claude.cmd"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(psArgsHelper, "1")
	run := func(argv []string) ([]byte, error) {
		return exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-Command", psRun(argv)).CombinedOutput()
	}
	if out, err := run([]string{"claude", "--append-system-prompt", strings.Repeat("a", 9000)}); err == nil && !strings.Contains(string(out), "too long") {
		t.Fatalf("a command over the limit got through, so this tests nothing: %.200s", out)
	}
	preset, _ := agent.Find("claude")
	files := t.TempDir()
	guide := realGuide(true)
	l := agentLaunch(preset, true, "Start", false, chatFor{guide: guide, files: files, goa: true})
	out, err := run(preset.Argv(l))
	if err != nil {
		t.Fatalf("%v: %.500s", err, out)
	}
	var got []string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("the program printed %.500q: %v", out, err)
	}
	i := slices.Index(got, "--append-system-prompt-file")
	if i < 0 {
		t.Fatalf("no guide's file: %q", got)
	}
	data, err := os.ReadFile(got[i+1])
	if err != nil || !strings.HasSuffix(string(data), guide) {
		t.Fatalf("the guide's file: %d bytes of %d, %v", len(data), len(guide), err)
	}
}
