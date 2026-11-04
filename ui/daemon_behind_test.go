package ui

import (
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// The card says exactly what would stop, and that nothing would when nothing would.
func TestRestartCostSaysWhat(t *testing.T) {
	if got := restartCost(proto.RestartPlan{}); !strings.Contains(got, "stops nothing") {
		t.Fatal(got)
	}
	got := restartCost(proto.RestartPlan{
		Agents:   []proto.RestartItem{{Session: "a", Name: "enowx", What: "claude"}},
		Commands: []proto.RestartItem{{Session: "b", What: "npm run dev"}},
	})
	for _, want := range []string{"claude (enowx)", "npm run dev"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in %q", want, got)
		}
	}
	var many []proto.RestartItem
	for range 8 {
		many = append(many, proto.RestartItem{What: "sleep"})
	}
	if got := restartItems(many); !strings.Contains(got, "and 3 more") {
		t.Fatal(got)
	}
}
