package agent

import (
	"strings"
	"testing"
)

// The rule is in what every session reads, and in GOA's own prompt.
func TestToDoRuleIsMandatory(t *testing.T) {
	if !strings.Contains(GuideFor(map[string]bool{}, true), "same turn as the commit") {
		t.Error("not in the session guide")
	}
	if !strings.Contains(GOAGuide, "same turn as the commit") || !strings.Contains(GOAGuide, "never tell one to leave a to-do open") {
		t.Error("not in GOA's prompt")
	}
}
