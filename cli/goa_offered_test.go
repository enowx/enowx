package cli

import (
	"slices"
	"testing"
)

// GOD of Agents is offered a few tools, every one that exists, the rest through enowx.
func TestGOAIsOfferedFewTools(t *testing.T) {
	var names []string
	for _, g := range goaOffered(mcpGroups()) {
		for _, tool := range g.tools {
			names = append(names, tool.name)
		}
	}
	for _, want := range goaOfferedTools {
		if !slices.Contains(names, want) {
			t.Errorf("%s isn't a tool", want)
		}
	}
	if len(names) != len(goaOfferedTools) {
		t.Errorf("offered %v", names)
	}
}
