package daemon

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/hooks"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A plain approval must not send Codex an unsupported PreToolUse allow;
// Claude Code and callers without an agent flag keep their approval output.
func TestJudgeHookApprovalByAgent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	if err := srv.judge.Settings().Set(proto.DecideSettings{Policy: proto.DecideLocalOnly,
		Own:  []proto.Judge{{Kind: proto.JudgeLocal, URL: judgeServer(t), Model: "judge"}},
		Uses: map[string]proto.DecideUse{proto.DecideApprove: {On: true}, proto.DecideTrim: {}}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "claude", "codex"} {
		t.Run("agent="+id, func(t *testing.T) {
			args := []string{"--event", proto.HookToolBefore}
			if id != "" {
				args = append(args, "--agent="+id)
			}
			code, out, reason := hookOf(`{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}`, args...)
			if code != 0 || reason != "" {
				t.Fatalf("allowed: exit %d, stdout %q, stderr %q", code, out, reason)
			}
			if id == "codex" {
				if out != "" {
					t.Errorf("Codex plain approval must have no output, got %q", out)
				}
			} else if !strings.Contains(out, `"permissionDecision":"allow"`) {
				t.Errorf("approval lost: %q", out)
			}
			code, out, reason = hookOf(`{"tool_name":"Bash","tool_input":{"command":"rm -rf ~"}}`, args...)
			if code != hooks.ExitBlock || out != "" || !strings.Contains(reason, "wipes the home folder") {
				t.Fatalf("refused: exit %d, stdout %q, stderr %q", code, out, reason)
			}
		})
	}
}

// Codex still receives supported allow+updatedInput rewrites when the
// session skips permissions; otherwise the original tool call is refused.
func TestCodexHookRewriteOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	if err := srv.judge.Settings().Set(proto.DecideSettings{Uses: map[string]proto.DecideUse{
		proto.DecideApprove: {}, proto.DecideTrim: {},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.saveHostHooks([]proto.Hook{{On: true, Event: proto.HookToolBefore,
		Action: proto.HookActionRewrite, Find: `^npm install`, Replace: "pnpm install"}}); err != nil {
		t.Fatal(err)
	}
	for _, skip := range []bool{false, true} {
		sess, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"},
			Meta: session.Meta{Agent: "codex", SkipPermissions: skip}})
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv(session.EnvSession, sess.ID())
		code, out, reason := hookOf(`{"tool_name":"Bash","tool_input":{"command":"npm install lodash","description":"deps"}}`,
			"--event", proto.HookToolBefore, "--agent=codex")
		if !skip {
			if code != hooks.ExitBlock || out != "" || !strings.Contains(reason, "Run this instead: pnpm install lodash") {
				t.Fatalf("unapproved rewrite: exit %d, stdout %q, stderr %q", code, out, reason)
			}
			continue
		}
		var response struct {
			Hook struct {
				Event    string            `json:"hookEventName"`
				Decision string            `json:"permissionDecision"`
				Input    map[string]string `json:"updatedInput"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.Unmarshal([]byte(out), &response); err != nil {
			t.Fatal(err)
		}
		if code != 0 || reason != "" || response.Hook.Event != "PreToolUse" || response.Hook.Decision != "allow" ||
			response.Hook.Input["command"] != "pnpm install lodash" || response.Hook.Input["description"] != "deps" {
			t.Fatalf("approved rewrite: exit %d, stdout %q, stderr %q", code, out, reason)
		}
	}
}

// Exercise the output protocol on every platform, including Windows.
func TestHookDecisionOutputByAgent(t *testing.T) {
	for _, agent := range []string{"", "claude", "codex"} {
		t.Run(agent, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := answerHook(proto.HookToolBefore, agent, nil, proto.HookDecision{Approve: true}, &out, &errOut)
			if code != 0 || errOut.Len() != 0 || (out.Len() == 0) != (agent == "codex") {
				t.Fatalf("approval: code %d stdout %q stderr %q", code, out.String(), errOut.String())
			}
			if agent != "codex" && !strings.Contains(out.String(), `"permissionDecision":"allow"`) {
				t.Fatalf("approval: %q", out.String())
			}
			out.Reset()
			errOut.Reset()
			code = answerHook(proto.HookToolBefore, agent, nil, proto.HookDecision{Block: true, Approve: true, Reason: "blocked"}, &out, &errOut)
			if code != hooks.ExitBlock || out.Len() != 0 || !strings.Contains(errOut.String(), "blocked") {
				t.Fatalf("block: code %d stdout %q stderr %q", code, out.String(), errOut.String())
			}
			out.Reset()
			errOut.Reset()
			code = answerHook(proto.HookToolBefore, agent, []byte(`{"tool_input":{"command":"old"}}`), proto.HookDecision{Approve: true, Command: "new"}, &out, &errOut)
			if code != 0 || !strings.Contains(out.String(), `"updatedInput":{"command":"new"}`) || !strings.Contains(out.String(), `"permissionDecision":"allow"`) {
				t.Fatalf("rewrite: code %d stdout %q stderr %q", code, out.String(), errOut.String())
			}
		})
	}
}

// The decision log says what happens: the judge's allow runs the command unasked
// on Claude Code, and on Codex is left to its prompt, for a worker as for a hook;
// Approve stays set on both, as a rewrite goes by it.
func TestJudgeAllowIsLoggedAsWhatHappens(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	if err := srv.judge.Settings().Set(proto.DecideSettings{Policy: proto.DecideLocalOnly,
		Own:  []proto.Judge{{Kind: proto.JudgeLocal, URL: judgeServer(t), Model: "judge"}},
		Uses: map[string]proto.DecideUse{proto.DecideApprove: {On: true}}}); err != nil {
		t.Fatal(err)
	}
	last := func() string {
		t.Helper()
		log := srv.judge.Log(proto.DecideLogParams{Use: proto.DecideApprove})
		if len(log) == 0 {
			t.Fatal("nothing logged")
		}
		newest := log[0]
		for _, d := range log {
			if d.Time.After(newest.Time) {
				newest = d
			}
		}
		return newest.Acted
	}
	ev := proto.HookEvent{Event: proto.HookToolBefore, Tool: "Bash", Command: "go test ./..."}
	for _, c := range []struct{ agent, want string }{{"claude", proto.ActedAllowed}, {"codex", proto.ActedLeft}} {
		info := proto.SessionInfo{ID: "s-" + c.agent, Agent: c.agent}
		if d, _ := srv.judgeHook(info, ev); !d.Approve {
			t.Fatalf("%s: hook %+v", c.agent, d)
		}
		if got := last(); got != c.want {
			t.Errorf("%s: the hook logged %q", c.agent, got)
		}
		if d := srv.goaWorkerTool(info, ev); !d.Approve {
			t.Fatalf("%s: worker %+v", c.agent, d)
		}
		if got := last(); got != c.want {
			t.Errorf("%s: the worker logged %q", c.agent, got)
		}
	}
}
