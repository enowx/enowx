package platformagent

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/tools"
)

const hardenSentinel = "sk-ant-standin-harden-0123456789"

// A test binary standing in for the agent: it starts as the agent does, then has a command
// read its parent's environment, as the Bash tool's would.
func TestHelperReadsParentEnviron(t *testing.T) {
	if os.Getenv("ENOWX_TEST_HARDEN") == "" {
		t.Skip("a helper")
	}
	if os.Getenv("ENOWX_TEST_HARDEN") == "on" {
		if err := HardenProcess(); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("/bin/sh", "-c", `cat /proc/$PPID/environ 2>&1 | tr '\0' '\n'`)
	cmd.Env = tools.CleanEnviron(os.Environ())
	out, _ := cmd.CombinedOutput()
	os.Stdout.Write(out)
}

func TestHardenProcessHidesTheEnvironmentFromCommands(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any process")
	}
	run := func(mode string) string {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperReadsParentEnviron$", "-test.v")
		cmd.Env = append(os.Environ(), "ENOWX_TEST_HARDEN="+mode, "ANTHROPIC_API_KEY="+hardenSentinel)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("helper: %v\n%s", err, out)
		}
		return string(out)
	}
	// Without it the hole is there, or this test proves nothing.
	if out := run("off"); !strings.Contains(out, hardenSentinel) {
		t.Fatalf("a command couldn't read its parent's environment even unhardened: %s", out)
	}
	if out := run("on"); strings.Contains(out, hardenSentinel) {
		t.Fatalf("the key is readable: %s", out)
	}
}
