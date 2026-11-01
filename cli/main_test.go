package cli

import (
	"os"
	"testing"

	"github.com/enowx/enowx/core/decide"
)

func TestMain(m *testing.M) {
	// No judge of this machine's (its Claude Code, its vault): the judge's
	// uses are on by default, and tests mustn't ask real models.
	os.Setenv(decide.EnvFindNone, "1")
	// Never the user's login shell: see AGENTS.md, Tests.
	os.Setenv("SHELL", "/bin/sh")
	os.Exit(m.Run())
}
