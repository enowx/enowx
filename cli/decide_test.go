package cli

import (
	"bytes"
	"errors"
	"runtime"
	"strings"
	"testing"
)

// decide trim passes a short output on as it is and a long one whole when
// no judge answers (here no daemon), keeping the command's exit code.
func TestTrimKeepsOutputAndExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	t.Setenv("ENOWX_HOME", t.TempDir()) // no daemon to ask
	var out bytes.Buffer
	err := trim([]string{"--", "echo hi; echo oops >&2; exit 3"}, &out)
	var exit ExitError
	if !errors.As(err, &exit) || exit.Code != 3 || out.String() != "hi\noops\n" {
		t.Fatalf("short: %q %v", out.String(), err)
	}
	out.Reset()
	if err := trim([]string{"--", "seq 1 400"}, &out); err != nil || strings.Count(out.String(), "\n") != 400 {
		t.Fatalf("long, no judge: %d lines %v", strings.Count(out.String(), "\n"), err)
	}
}

// In GOD of Agents' session, what runs a program of its choosing is
// refused: a credential's command, a trimmed command, a project command.
func TestGOARunsNoCommand(t *testing.T) {
	t.Setenv("ENOWX_GOA", "1")
	t.Setenv("ENOWX_HOME", t.TempDir())
	var out bytes.Buffer
	if err := trim([]string{"--", "echo hi"}, &out); err == nil || out.Len() > 0 {
		t.Fatalf("trim ran: %q %v", out.String(), err)
	}
	if err := (runner{out: &out}).credsUse("", []string{"github", "--", "echo", "hi"}); err == nil || !strings.Contains(err.Error(), "GOD of Agents") {
		t.Fatalf("creds use: %v", err)
	}
	if err := (runner{out: &out}).run([]string{"dev"}); err == nil || !strings.Contains(err.Error(), "GOD of Agents") {
		t.Fatalf("run: %v", err)
	}
}
