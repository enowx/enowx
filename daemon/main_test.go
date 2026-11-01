package daemon

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/internal/testlisten"
)

// psArgsHelper, set in the test binary's environment, makes it print its
// arguments as JSON and exit: the program PowerShell starts in
// TestPsQuotePreservesArguments.
const psArgsHelper = "ENOWX_PS_ARGS_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(psArgsHelper) == "1" {
		_ = json.NewEncoder(os.Stdout).Encode(os.Args[1:])
		os.Exit(0)
	}
	testlisten.Serve()
	// No judge of this machine's (its Claude Code, its vault): the judge's
	// uses are on by default, and tests mustn't ask real models.
	os.Setenv(decide.EnvFindNone, "1")
	// Never the user's login shell: see AGENTS.md, Tests.
	os.Setenv("SHELL", "/bin/sh")
	os.Exit(m.Run())
}
