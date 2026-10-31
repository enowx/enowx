package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/hooks"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// readsSoon waits for file to hold want.
func readsSoon(t *testing.T, file, want string) {
	t.Helper()
	waitFor(t, want, func() bool { data, _ := os.ReadFile(file); return strings.Contains(string(data), want) })
}

// A hook changes the agent's command (the agent asks the user about the
// new one); Git about to run, from the agent or the app, goes by the
// hooks before Git; a permission's answer is told by what follows.
func TestHooksChangeAndGuardGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, sess := hookServer(t)
	// The built-in trim would wrap the rewritten command: this is about the user's hooks.
	if err := srv.judge.Settings().Set(proto.DecideSettings{Uses: map[string]proto.DecideUse{proto.DecideTrim: {}}}); err != nil {
		t.Fatal(err)
	}
	got := filepath.Join(t.TempDir(), "got")
	if _, err := srv.saveHostHooks([]proto.Hook{
		{On: true, Event: proto.HookToolBefore, Action: proto.HookActionRewrite, Find: `^npm install`, Replace: "pnpm install"},
		{On: true, Event: proto.HookGitBefore, Text: `\bmain\b`, Action: proto.HookActionBlock, Reason: "Not on main."},
		{On: true, Event: proto.HookGitBefore, Action: proto.HookActionRun, Command: `grep -q '"op":"commit"' && { echo "Run the tests first." >&2; exit 2; }; exit 0`},
		{On: true, Event: proto.HookAnswered, Action: proto.HookActionRun, Command: "cat >> " + got},
	}); err != nil {
		t.Fatal(err)
	}
	code, out, _ := hookOf(`{"tool_name":"Bash","tool_input":{"command":"npm install lodash","description":"deps"}}`, "--event", proto.HookToolBefore)
	if code != 0 || !strings.Contains(out, `"command":"pnpm install lodash"`) || !strings.Contains(out, `"permissionDecision":"ask"`) || !strings.Contains(out, `"description":"deps"`) {
		t.Fatalf("rewrite: %d %s", code, out)
	}
	code, _, reason := hookOf(`{"tool_name":"Bash","tool_input":{"command":"git push origin main"}}`, "--event", proto.HookToolBefore)
	if code != hooks.ExitBlock || strings.TrimSpace(reason) != "Not on main." {
		t.Fatalf("git push main: %d %q", code, reason)
	}
	if err := srv.gitBefore(sess.ID(), "commit"); err == nil || err.Error() != "Run the tests first." {
		t.Fatalf("commit from the app: %v", err)
	}
	if err := srv.gitBefore(sess.ID(), "push"); err != nil {
		t.Fatalf("push from the app: %v", err)
	}
	hookOf(`{"tool_name":"Bash","tool_input":{"command":"make"}}`, "--event", proto.HookPermission)
	hookOf(`{"tool_name":"Bash","tool_input":{"command":"make"}}`, "--event", proto.HookToolAfter)
	readsSoon(t, got, `"allowed":true`)
}

// Hooks kept in a project (allowed as the user saved them), in a session
// alone, and proposed by an agent, run once allowed.
func TestHookScopesAndProposals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, sess := hookServer(t)
	dir := t.TempDir()
	block := proto.Hook{On: true, Event: proto.HookToolBefore, Tool: "deploy", Action: proto.HookActionBlock, Reason: "project"}
	if err := srv.saveProjectHooks(proto.ProjectHooks{Dir: dir, Hooks: []proto.Hook{block}}); err != nil {
		t.Fatal(err)
	}
	info := sess.Info()
	info.Dir = dir
	ev := proto.HookEvent{Event: proto.HookToolBefore, Command: "make deploy", Via: proto.HookInAgent}
	if d := srv.fireHooks(info, ev); d.Reason != "project" {
		t.Fatalf("the project's own: %+v", d)
	}
	block.Reason = "session"
	if err := srv.saveSessionHooks(proto.SessionHooks{Session: sess.ID(), Hooks: []proto.Hook{block}}); err != nil {
		t.Fatal(err)
	}
	if d := srv.fireHooks(sess.Info(), ev); d.Reason != "session" {
		t.Fatalf("the session's own: %+v", d)
	}
	other := sess.Info()
	other.ID = "another"
	if d := srv.fireHooks(other, ev); d.Block {
		t.Fatal("a session's hook ran in another")
	}

	block.Reason = "proposed"
	p, err := srv.propose(proto.HookProposal{Hook: block, By: sess.ID(), Why: "keep deploys manual"})
	if err != nil {
		t.Fatal(err)
	}
	if len(srv.hooks.store.List()) != 0 || len(srv.hooksInfo("").Proposals) != 1 {
		t.Fatal("a proposal ran before it was allowed")
	}
	if err := srv.decide(proto.HookDecideParams{ID: p.ID, Allow: true}); err != nil {
		t.Fatal(err)
	}
	host := srv.hooks.store.List()
	if len(host) != 1 || host[0].Reason != "proposed" || len(srv.hooksInfo("").Proposals) != 0 {
		t.Fatalf("after allowing: %+v", host)
	}
	// Removing it is a proposal too.
	p, _ = srv.propose(proto.HookProposal{Hook: host[0], Remove: true, By: sess.ID()})
	srv.decide(proto.HookDecideParams{ID: p.ID, Allow: false})
	if len(srv.hooks.store.List()) != 1 {
		t.Fatal("a rejected removal removed")
	}
}

// For an agent without hooks: a prompt typed, a limit of once, a
// schedule's time, an error ending its program.
func TestHooksOutsideMore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)
	got := filepath.Join(t.TempDir(), "got")
	if _, err := srv.saveHostHooks([]proto.Hook{
		{On: true, Event: proto.HookPromptStart, Once: true, Action: proto.HookActionRun, Command: "cat >> " + got},
		{On: true, Event: proto.HookError, Action: proto.HookActionRun, Command: "cat >> " + got},
		{On: true, Event: proto.HookSchedule, Schedule: "hourly", Action: proto.HookActionRun, Command: "echo scheduled >> " + got},
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "read a; read b; exit 3"}, Cols: 80, Rows: 24, Meta: session.Meta{Agent: "aider"}})
	if err != nil {
		t.Fatal(err)
	}
	srv.sessions.Input(sess, []byte("fix the \x1b[Dlogin\r"))
	readsSoon(t, got, `"prompt":"fix the login"`)
	srv.sessions.Input(sess, []byte("\x1b[200~two\nlines\x1b[201~\r"))
	readsSoon(t, got, `"event":"error"`)
	if data, _ := os.ReadFile(got); strings.Contains(string(data), "two") {
		t.Fatal("a hook set to run once ran twice")
	}
	if data, _ := os.ReadFile(got); !strings.Contains(string(data), `"exit":3`) {
		t.Fatalf("error: %s", data)
	}
	now := time.Now()
	srv.scheduleDue(now.Add(-time.Hour), now)
	readsSoon(t, got, "scheduled")
}

// A commit in a session's folder is seen, and a merge as one too.
func TestHooksOnGitChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, sess := hookServer(t)
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if _, err := git.Run(dir, append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...); err != nil {
			t.Fatal(args, err)
		}
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "first")
	info := sess.Info()
	info.Dir = dir
	was, _ := gitStateOf(dir)
	run("checkout", "-q", "-b", "side")
	run("commit", "-q", "--allow-empty", "-m", "on side")
	run("checkout", "-q", "main")
	run("commit", "-q", "--allow-empty", "-m", "on main")
	run("merge", "-q", "--no-edit", "side")
	got := filepath.Join(t.TempDir(), "got")
	srv.saveHostHooks([]proto.Hook{
		{On: true, Event: proto.HookCommit, Action: proto.HookActionRun, Command: "cat >> " + got},
		{On: true, Event: proto.HookMerge, Action: proto.HookActionRun, Command: "cat >> " + got},
	})
	now, _ := gitStateOf(dir)
	srv.gitChanged(info, was, now)
	readsSoon(t, got, `"event":"git.merge"`)
	readsSoon(t, got, `"event":"git.commit"`)
}
