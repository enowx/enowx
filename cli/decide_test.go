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
