package daemon

import (
	"strings"
	"testing"

	"github.com/enowx/enowx/core/agent"
)

// A Claude Code session that never had a conversation starts anew when
// continued, rather than "--continue" finding none and exiting; one with
// a conversation resumes it; an agent not resumed by ID continues its
// folder's latest, as before.
func TestContinueWithoutAConversation(t *testing.T) {
	claude, _ := agent.Find("claude")
	codex, _ := agent.Find("codex")
	argv := func(p agent.Preset, convo string) string {
		l := agentLaunch(p, false, "", hasConversation(p, convo), chatFor{})
		l.Resume = convo
		return strings.Join(p.Argv(l), " ")
	}
	if a := argv(claude, ""); strings.Contains(a, "--continue") || strings.Contains(a, "--resume") {
		t.Fatalf("fresh claude: %s", a)
	}
	if a := argv(claude, "abc-123"); !strings.Contains(a, "abc-123") {
		t.Fatalf("claude with its conversation: %s", a)
	}
	if len(codex.Resume) == 0 && !hasConversation(codex, "") {
		t.Fatal("codex, resumed by no ID, should continue its folder's latest")
	}
}
