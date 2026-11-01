package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A worker stopped on a prompt no hook tells of, in a program with no hooks,
// goes to waiting: GOA is told what's asked and the time it gives, session_ask
// says the same, and GOA hears when the timed prompt goes.
func TestGOATold(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	c := connect(t, srv, nil)
	g, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: t.TempDir(), Cols: 100, Rows: 24, Meta: session.Meta{GOA: true, Agent: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.sessions.SetState(g, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	script := `read task; printf 'rm -rf x\nDo you want to proceed?\n > 1. Yes\n   2. No\nIt will block this automatically in 1:51\n'; read answer; printf '\033[2J\033[Hbusy again\n'; sleep 60`
	var w proto.SessionInfo
	if err := c.Call(proto.MethodSessionCreate, proto.SessionCreateParams{Command: []string{"sh", "-c", script}, Dir: t.TempDir(), Cols: 100, Rows: 24}, &w); err != nil {
		t.Fatal(err)
	}
	var task proto.GOATask
	if err := c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Session: w.ID, Prompt: "go", NoRoute: true}, &task); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the task waiting", func() bool {
		for _, x := range goaBoardOf(t, c).Tasks {
			if x.ID == task.ID && x.Status == proto.GOATaskWaiting {
				return true
			}
		}
		return false
	})
	waitScreen(t, c, g.ID(), "Do you want to proceed?")
	waitScreen(t, c, g.ID(), "1:51")
	var ask proto.SessionAsk
	if err := c.Call(proto.MethodSessionAsk, proto.SessionRef{ID: w.ID}, &ask); err != nil {
		t.Fatal(err)
	}
	if !ask.Waiting || !strings.Contains(ask.Prompt, "proceed") || ask.Deadline != "1:51" {
		t.Fatalf("ask: %+v", ask)
	}
	if err := c.Input(w.ID, "1", false, true); err != nil {
		t.Fatal(err)
	}
	waitScreen(t, c, g.ID(), "no longer shows")
}

// What a session and its task show is what its agent's transcript says, each
// change kept on the task; stopped, the session shows it as the last.
func TestSessionModelFollowsItsTranscript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	bin := t.TempDir()
	script := `#!/bin/sh
dir="$HOME/.claude/projects/$(pwd | sed 's/[^A-Za-z0-9]/-/g')"
mkdir -p "$dir"
say() { echo "{\"type\":\"assistant\",\"effort\":\"$2\",\"message\":{\"role\":\"assistant\",\"model\":\"$1\",\"content\":[{\"type\":\"text\",\"text\":\"ok\"}]}}" >> "$dir/s.jsonl"; }
while read -r line; do [ -n "$line" ] && say $line; done
`
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	project := gitRepo(t)
	if _, err := c.AddProject(project); err != nil {
		t.Fatal(err)
	}
	var task proto.GOATask
	if err := c.Call(proto.MethodGOADelegate, proto.GOADelegateParams{Agent: "claude", Project: project, Branch: "", Prompt: "begin", NoRoute: true}, &task); err != nil {
		t.Fatal(err)
	}
	sess, err := srv.sessions.Get(task.Session)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.sessions.SetState(sess, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	info := func() proto.SessionInfo { return sess.Info() }
	ran := func() []string {
		for _, x := range goaBoardOf(t, c).Tasks {
			if x.ID == task.ID {
				return x.Ran
			}
		}
		return nil
	}
	waitForLong := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(30 * time.Second); !ok(); time.Sleep(100 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s: session %+v, task ran %v", what, info().Model, ran())
			}
		}
	}
	if err := c.Input(task.Session, "model-a low", false, true); err != nil {
		t.Fatal(err)
	}
	waitForLong("the first model", func() bool { return info().Model == "model-a" && info().Effort == "low" })
	// As /model, a switch, a fallback or a config make it answer on another.
	if err := c.Input(task.Session, "model-b high", false, true); err != nil {
		t.Fatal(err)
	}
	waitForLong("the change", func() bool { return info().Model == "model-b" && info().Effort == "high" })
	waitForLong("the task keeping both", func() bool {
		return slices.Equal(ran(), []string{"model-a · low", "model-b · high"})
	})
	if err := srv.sessions.SetState(sess, proto.StateDone); err != nil {
		t.Fatal(err)
	}
	res, err := c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{task.Session}})
	if err != nil || len(res.Hibernated) != 1 {
		t.Fatalf("hibernating: %+v %v", res, err)
	}
	stopped, err := srv.sessions.Get(task.Session)
	if err != nil {
		t.Fatal(err)
	}
	if got := stopped.Info(); got.Model != "model-b" || !got.ModelLast {
		t.Fatalf("stopped: %q last %v", got.Model, got.ModelLast)
	}
}

// An agent with another account but no sign-in command starts itself to sign in,
// as it asks on its first run; one with no account route still says it can't.
func TestSignInWithNoCommand(t *testing.T) {
	l, err := launchFor(proto.SessionCreateParams{Agent: "kimi", SignIn: true}, chatFor{})
	if err != nil || !slices.Contains(l.command, "kimi") || l.name != "Sign in to Kimi" {
		t.Fatalf("%+v %v", l, err)
	}
	if _, err := launchFor(proto.SessionCreateParams{Agent: "grok", SignIn: true}, chatFor{}); err == nil {
		t.Fatal("an agent with no sign-in and no accounts was started to sign in")
	}
}

// GOD of Agents reaches any project's RAG, to set it up; another session only
// its own.
func TestGOAReachesAnyProjectsRAG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses cat")
	}
	srv := startServer(t)
	g, err := srv.sessions.Create(session.Spec{Command: []string{"cat"}, Dir: t.TempDir(), Meta: session.Meta{GOA: true, Agent: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	w, err := srv.sessions.Create(session.Spec{Command: []string{"cat"}, Dir: t.TempDir(), Meta: session.Meta{Agent: "claude", Project: "/code/a"}})
	if err != nil {
		t.Fatal(err)
	}
	if dir, err := srv.ragDir(proto.RAGRef{Session: g.ID(), Project: "/code/b"}); err != nil || dir != "/code/b" {
		t.Fatalf("GOA: %q %v", dir, err)
	}
	if _, err := srv.ragDir(proto.RAGRef{Session: w.ID(), Project: "/code/b"}); err == nil {
		t.Fatal("a worker reached another project")
	}
}
