package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// GOA can't restart itself, from its session or over its MCP server, which doesn't offer it.
func TestGOACantRestartItself(t *testing.T) {
	t.Setenv(proto.EnvGOA, "1")
	err := runner{}.goaRestart(nil)
	if err == nil || !strings.Contains(err.Error(), "can't restart itself") {
		t.Fatalf("got %v", err)
	}
	if slices.Contains(goaOfferedTools, "goa_restart") {
		t.Fatal("offered to GOA")
	}
}
