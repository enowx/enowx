package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A link leading out of GOA's folder keeps it from starting on OpenCode, from version 2
// nothing else stopping an edit going through it. One inside does not.
func TestLinkOutOfGOAFolder(t *testing.T) {
	root := t.TempDir()
	dir, project := filepath.Join(root, "goa"), filepath.Join(root, "project")
	for _, d := range []string{filepath.Join(dir, "sub"), project} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "sub"), filepath.Join(dir, "in")); err != nil {
		t.Skip(err)
	}
	if got := linkOutOf(dir); got != "" {
		t.Fatalf("one inside counted: %s", got)
	}
	if err := os.Symlink(project, filepath.Join(dir, "sub", "out")); err != nil {
		t.Fatal(err)
	}
	if got := linkOutOf(dir); !strings.HasSuffix(got, "out") {
		t.Fatalf("got %q", got)
	}
	if err := os.Remove(filepath.Join(dir, "sub", "out")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(project, "not-yet"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	if got := linkOutOf(dir); !strings.HasSuffix(got, "dangling") {
		t.Fatalf("dangling: %q", got)
	}
}

// GOA starts only on an agent proven to hold it, saying so otherwise.
func TestGOARefusedOnAnAgentNotProven(t *testing.T) {
	srv := startServer(t)
	_, err := srv.createSession(proto.SessionCreateParams{GOA: true, Agent: "qwen-code"})
	if err == nil || !strings.Contains(err.Error(), "not tested") {
		t.Fatalf("got %v", err)
	}
	for _, a := range goaAgents() {
		if a.Agent == "qwen-code" && a.NotYet == "" {
			t.Error("offered in the list")
		}
	}
}

// Its edits stay in its folder, through the hook the agents with hooks answer to.
func TestGOAEditOutsideItsFolderRefused(t *testing.T) {
	srv := startServer(t)
	info := proto.SessionInfo{ID: "g", GOA: true}
	ev := proto.HookEvent{Event: proto.HookToolBefore, Tool: "Write", Paths: []string{"/etc/hosts"}}
	if d, stop := srv.goaHook(info, ev); !stop || !d.Block {
		t.Errorf("outside: %+v %v", d, stop)
	}
	ev.Paths = []string{"plan.md"}
	if d, _ := srv.goaHook(info, ev); d.Block {
		t.Errorf("inside: %+v", d)
	}
}

// GOA on OpenCode comes back from a restart or a wake with what holds it.
func TestGOAOnOpenCodeRestoredWithItsConfig(t *testing.T) {
	srv := startServer(t)
	env := strings.Join(srv.restoredToolEnv(session.Meta{GOA: true, Agent: "opencode2"}), " ")
	if !strings.Contains(env, "OPENCODE_CONFIG_CONTENT=") || !strings.Contains(env, `"policies"`) {
		t.Fatalf("got %q", env)
	}
	if got := srv.restoredToolEnv(session.Meta{GOA: true, Agent: "claude"}); got != nil {
		t.Fatalf("claude: %q", got)
	}
}
