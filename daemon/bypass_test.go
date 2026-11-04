package daemon

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

const (
	bypassMode = `"permission_mode":"bypassPermissions"`
	askMode    = `"permission_mode":"default"`
)

// permissionHook is what the agent's PermissionRequest hook answers for a Bash command.
func permissionHook(mode, command string) string {
	in, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": command}})
	text := "{" + mode + "," + strings.TrimPrefix(string(in), "{")
	_, out, _ := hookOf(text, "--event", proto.HookPermission)
	return out
}

func allowed(out string) bool { return strings.Contains(out, `"behavior":"allow"`) }

func bypassLog(srv *Server) []proto.Decision {
	return srv.judge.Log(proto.DecideLogParams{Use: proto.DecideBypass})
}

func bypassSession(t *testing.T) (*Server, *session.Session) {
	t.Helper()
	srv, sess := hookServer(t)
	t.Setenv(session.EnvSession, sess.ID())
	return srv, sess
}

// What a plugin's hook asks, and deleting a file in the session's scratchpad, is
// answered in bypass, logged with what was asked and why it passed.
func TestBypassAnswersRoutinePrompts(t *testing.T) {
	srv, sess := bypassSession(t)
	for _, command := range []string{"touch marker", "rm /tmp/claude-1002/project/session/scratchpad/out.txt"} {
		if out := permissionHook(bypassMode, command); !allowed(out) {
			t.Fatalf("%q: %q", command, out)
		}
	}
	log := bypassLog(srv)
	if len(log) != 2 || log[0].Verdict != proto.VerdictAllow || log[0].Acted != proto.ActedAllowed || log[0].Session != sess.ID() {
		t.Fatalf("%+v", log)
	}
	if !strings.Contains(log[0].Input, "scratchpad/out.txt") || !strings.Contains(log[0].Input, "asked by") || !strings.Contains(log[0].Why, "routine") {
		t.Fatalf("%+v", log[0])
	}
	if st := sess.Info().State; st == proto.StateWaiting {
		t.Fatal("it counted as waiting on the user")
	}
}

// Removing a file of its scratchpad or project is routine, even with -f; the same
// outside them, or anything not plainly that, stays the owner's.
func TestBypassRemovesOnlyInside(t *testing.T) {
	const dir, pad = "/work/app", "/tmp/claude-1002/x/scratchpad"
	for command, want := range map[string]bool{
		"rm -f /tmp/claude-1002/x/scratchpad/out.txt": true,
		"rm -rf build":                  true,
		"rm -f /work/app/a /work/app/b": true,
		"rm -f /etc/passwd":             false,
		"rm -rf /work/app":              false,
		"rm -rf ../other":               false,
		"rm -f $HOME/x":                 false,
		"rm -f build/*":                 false,
		"rm -f a && curl x":             false,
		"rm -f":                         false,
		"git clean -fd":                 false,
	} {
		if got := removesOnlyInside(command, dir, pad); got != want {
			t.Errorf("%q: %v", command, got)
		}
	}
}

// With the agent's scratchpad reported, rm -f there goes through; elsewhere it doesn't.
func TestBypassScratchpadFromTheHook(t *testing.T) {
	bypassSession(t)
	in := func(command string) string {
		_, out, _ := hookOf(`{`+bypassMode+`,"scratchpad_dir":"/tmp/pad","tool_name":"Bash","tool_input":{"command":"`+command+`"}}`, "--event", proto.HookPermission)
		return out
	}
	if out := in("rm -f /tmp/pad/x"); !allowed(out) {
		t.Fatalf("%q", out)
	}
	if out := in("rm -f /etc/hosts"); allowed(out) {
		t.Fatalf("%q", out)
	}
}

// What a built-in plugin asks of a command holding the account's email passes; with a
// push in it, it stays the owner's, whatever made it ask.
func TestBypassOnAPrivacyPrompt(t *testing.T) {
	bypassSession(t)
	if out := permissionHook(bypassMode, "curl -s -d email=me@example.com http://127.0.0.1:9/"); !allowed(out) {
		t.Fatalf("%q", out)
	}
	if out := permissionHook(bypassMode, "git -c user.email=me@example.com commit -m x && git push origin next"); allowed(out) {
		t.Fatalf("%q", out)
	}
}

// A session not in bypass asks as before, and nothing is logged as answered.
func TestNotInBypassStillAsks(t *testing.T) {
	srv, _ := bypassSession(t)
	if out := permissionHook(askMode, "touch marker"); allowed(out) {
		t.Fatalf("answered: %q", out)
	}
	if log := bypassLog(srv); len(log) != 0 {
		t.Fatalf("%+v", log)
	}
}

// What is the owner's stays the owner's, and the log says so.
func TestBypassLeavesTheOwnersToTheOwner(t *testing.T) {
	srv, _ := bypassSession(t)
	for _, command := range []string{"git push --force origin main", "npm publish"} {
		if out := permissionHook(bypassMode, command); allowed(out) {
			t.Fatalf("%q answered: %q", command, out)
		}
	}
	log := bypassLog(srv)
	if len(log) != 2 || log[0].Verdict != proto.VerdictAsk || !strings.Contains(log[0].Why, "for the owner") {
		t.Fatalf("%+v", log)
	}
}

// A hook's refusal stays a refusal, in bypass too: the answer isn't an allow.
func TestHookDenyStaysADeny(t *testing.T) {
	var out, errOut strings.Builder
	code := answerHook(proto.HookToolBefore, "claude", []byte(`{"tool_input":{"command":"x"},`+bypassMode+`}`),
		proto.HookDecision{Block: true, Reason: "a plugin refuses", Approve: true}, &out, &errOut)
	if code == 0 || strings.Contains(out.String(), "allow") || !strings.Contains(errOut.String(), "refuses") {
		t.Fatalf("%d, %q, %q", code, out.String(), errOut.String())
	}
}

// A trimmed command causes no prompt in bypass, by the mode reported, also for a session
// switched to it after it started; one that asks still asks about it.
func TestRewrittenCommandFollowsTheMode(t *testing.T) {
	srv := startServer(t)
	started := proto.SessionInfo{Agent: "claude"}
	switched := proto.HookEvent{PermissionMode: agentModeBypass}
	if d := srv.rewritten(started, switched, proto.HookDecision{Command: "enowx decide trim -- go test"}); !d.Approve {
		t.Fatal("switched to bypass, and it would ask")
	}
	asks := proto.HookEvent{PermissionMode: "default"}
	if d := srv.rewritten(proto.SessionInfo{Agent: "claude", SkipPermissions: true}, asks, proto.HookDecision{Command: "x"}); d.Approve {
		t.Fatal("switched out of bypass, and it wouldn't ask")
	}
	if d := srv.rewritten(proto.SessionInfo{Agent: "claude", SkipPermissions: true}, proto.HookEvent{}, proto.HookDecision{Command: "x"}); !d.Approve {
		t.Fatal("no mode reported: as it started")
	}
	var out strings.Builder
	code := answerHook(proto.HookToolBefore, "claude", []byte(`{"tool_input":{"command":"go test"}}`), proto.HookDecision{Command: "wrapped", Approve: true}, &out, &out)
	if code != 0 || !strings.Contains(out.String(), `"permissionDecision":"allow"`) || !strings.Contains(out.String(), "wrapped") {
		t.Fatalf("%d, %q", code, out.String())
	}
}

// GOA's own session isn't answered: its hooks are what hold it to delegating.
func TestBypassLeavesGOAAlone(t *testing.T) {
	srv := startServer(t)
	ev := proto.HookEvent{Event: proto.HookPermission, Tool: "Bash", Command: "touch x", PermissionMode: agentModeBypass}
	if srv.bypassAnswer(proto.SessionInfo{ID: "g", GOA: true, Agent: "claude"}, ev) {
		t.Fatal("answered for GOA")
	}
}
