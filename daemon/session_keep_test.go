package daemon

import (
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A session starts again on what it had: its restart command says so itself, and an
// agent's global settings have no say.
func TestStartCommandsKeepASessionsSettings(t *testing.T) {
	has := func(argv []string, want ...string) bool {
		return strings.Contains(strings.Join(argv, " "), strings.Join(want, " "))
	}
	args := keptArgs(session.Meta{Agent: "claude", Model: "claude-opus-5-5", Effort: "high", Mode: "plan"})
	if !has(args, "--model", "claude-opus-5-5") || !has(args, "--effort", "high") || !has(args, "--permission-mode", "plan") {
		t.Errorf("claude: %v", args)
	}
	args = keptArgs(session.Meta{Agent: "codex", Model: "gpt-6-luna", Effort: "low"})
	if !has(args, `model="gpt-6-luna"`) || !has(args, `model_reasoning_effort="low"`) {
		t.Errorf("codex: %v", args)
	}
	if args := keptArgs(session.Meta{Agent: "claude", Model: "x", Config: &proto.SessionConfig{}}); len(args) != 0 {
		t.Errorf("a config decides its own: %v", args)
	}
	if args := keptArgs(session.Meta{Agent: "claude", Model: "<synthetic>", SkipPermissions: true, Mode: "auto"}); len(args) != 0 {
		t.Errorf("nothing its command line would take: %v", args)
	}
	l, err := launchFor(proto.SessionCreateParams{Agent: "claude", Model: "claude-fable-5-1", Effort: "max", Mode: "acceptEdits"}, chatFor{})
	if err != nil {
		t.Fatal(err)
	}
	if !has(l.restart, "--model", "claude-fable-5-1") || !has(l.restart, "--effort", "max") || slices.Contains(l.restart, "--permission-mode") {
		t.Errorf("restart: %v", l.restart)
	}
}

// What the user chose for an agent starts its new sessions, whatever a session wrote to the
// agent's own settings; what a caller asks for goes first.
func TestNewSessionTakesTheChosenStart(t *testing.T) {
	srv := startServer(t)
	st := srv.sched.get()
	st.AgentStart = map[string]proto.AgentStart{"claude": {Model: "claude-opus-5-5", Effort: "high", Mode: "auto"}}
	if _, err := srv.sched.set(st); err != nil {
		t.Fatal(err)
	}
	p := srv.withAgentStart(proto.SessionCreateParams{Agent: "claude", Effort: "low"})
	if p.Model != "claude-opus-5-5" || p.Effort != "low" || p.Mode != "auto" {
		t.Fatalf("%+v", p)
	}
}
