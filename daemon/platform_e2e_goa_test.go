//go:build !windows

package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/internal/standinapi"
	"github.com/enowx/enowx/proto"
)

// goaBoard reads GOA's board.
func (e *e2e) goaBoard() proto.GOABoard {
	e.t.Helper()
	var b proto.GOABoard
	if err := e.c.Call(proto.MethodGOABoard, nil, &b); err != nil {
		e.t.Fatal(err)
	}
	return b
}

// GOD of Agents on it: refused what it may not do, in bypass as much as asking.
func e2eGOARefuses(t *testing.T) {
	for _, bypass := range []bool{true, false} {
		e := startE2E(t, e2eOptions{})
		outside := filepath.Join(e.work, "goa-wrote.txt")
		id := e.spawn(proto.SessionCreateParams{GOA: true, SkipPermissions: bypass, Project: ""})
		e.api.Enqueue(standinapi.Calls(
			standinapi.ToolUse("toolu_bash", "Bash", `{"command":"touch `+outside+`"}`),
			standinapi.ToolUse("toolu_write", "Write", fmt.Sprintf(`{"file_path":%q,"content":"nope"}`, outside)),
		))
		e.say(id, "build it yourself", 2)
		entries := e.entries(id)
		if bash, _ := lastTool(entries, "Bash"); !bash.Failed || bash.Result == "" {
			t.Errorf("bypass %v: Bash wasn't refused: %+v", bypass, bash)
		}
		if write, _ := lastTool(entries, "Write"); !write.Failed {
			t.Errorf("bypass %v: Write wasn't refused: %+v", bypass, write)
		}
		if _, err := os.Stat(outside); err == nil {
			t.Errorf("bypass %v: GOA changed a file outside its folder", bypass)
		}
		e.stopDaemon()
	}
}

// GOA delegating, in the mode that asks: it runs unasked and starts a worker of the same
// agent, whose turn's end reaches the board and GOA itself.
func e2eGOADelegates(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{GOA: true, Project: ""})
	input, _ := json.Marshal(map[string]any{"prompt": "Say the worker's word", "title": "The worker's task", "agent": "claude-platform", "project": e.work, "no_route": true})
	e.api.Enqueue(standinapi.Calls(standinapi.ToolUse("toolu_goa", "mcp__enowx__goa_delegate", string(input))))
	e.send(id, "delegate it")
	e.wait(id, "the delegate's result, with nothing asked", func() bool {
		if e.info(id).State == proto.StateWaiting {
			t.Fatalf("it asks:\n%s", e.screen(id))
		}
		call, ok := lastTool(e.entries(id), "mcp__enowx__goa_delegate")
		return ok && call.Done
	})
	if call, _ := lastTool(e.entries(id), "mcp__enowx__goa_delegate"); call.Failed {
		t.Fatalf("goa_delegate failed: %+v", call)
	}
	e.wait(id, "a task on the board", func() bool { return len(e.goaBoard().Tasks) == 1 })
	task := e.goaBoard().Tasks[0]
	if task.Session == "" || task.Agent != "claude-platform" {
		t.Fatalf("task: %+v", task)
	}
	if worker := e.info(task.Session); worker.Agent != "claude-platform" {
		t.Errorf("worker: %+v", worker)
	}
	e.wait(task.Session, "the worker's end on the board", func() bool { return e.goaBoard().Tasks[0].Status == proto.GOATaskTurnEnded })
	e.wait(id, "GOA told of it", func() bool {
		return slices.ContainsFunc(e.entries(id), func(x proto.TranscriptEntry) bool {
			return x.Kind == proto.TranscriptUser && strings.Contains(x.Text, "[enowX GOA] Task "+task.ID)
		})
	})

	// `goa delegate` on the command line, as GOA's guide shows it.
	out, err := e.run("goa", "delegate", "--agent", "claude-platform", "--project", e.work, "--title", "Second", "--no-route", "Another word")
	if err != nil {
		t.Fatalf("goa delegate: %v: %s", err, out)
	}
	e.wait(id, "a second task", func() bool { return len(e.goaBoard().Tasks) == 2 })
}

// Retries, a refusal, no key and a bad key, all as what the user reads in the chat.
func e2eFailures(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{})
	e.api.Enqueue(standinapi.RateLimited("1"), standinapi.Err(529), standinapi.Text("Made it through."))
	e.say(id, "try hard", 3)
	entries := e.entries(id)
	if !hasNotice(entries, "rate limited", "retrying") || !hasNotice(entries, "overloaded", "retrying") {
		t.Errorf("no retry notices: %s", e.dumpEntries(id))
	}
	if !slices.ContainsFunc(entries, func(x proto.TranscriptEntry) bool {
		return x.Kind == proto.TranscriptAssistant && x.Text == "Made it through."
	}) {
		t.Errorf("no answer after retrying: %s", e.dumpEntries(id))
	}

	e.api.Enqueue(standinapi.Refusal())
	e.say(id, "something it won't do", 1)
	e.wait(id, "the refusal's notice", func() bool { return hasNotice(e.entries(id), "declined") })

	e.api.Enqueue(standinapi.Reply{Status: 401, Message: "invalid x-api-key"})
	e.say(id, "bad key now", 1)
	e.wait(id, "the 401's notice", func() bool { return hasNotice(e.entries(id), "401") })
	if !strings.Contains(e.screen(id), "401") {
		t.Errorf("the screen doesn't say 401:\n%s", e.screen(id))
	}

	none := startE2E(t, e2eOptions{noKey: true})
	bare := none.spawn(proto.SessionCreateParams{Prompt: "hello with no key"})
	none.wait(bare, "the no-key notice", func() bool { return hasNotice(none.entries(bare), "no API key") })
	if strings.Contains(strings.ToLower(none.screen(bare)), "panic") {
		t.Errorf("screen:\n%s", none.screen(bare))
	}
	if n := len(none.api.Requests()); n != 0 {
		t.Errorf("%d requests with no key", n)
	}
	if info := none.info(bare); info.Exited {
		t.Errorf("the session ended: %+v", info)
	}
}

// Token use comes through its adapter; the budget, when tiny, asks y or n.
func e2eUsageAndBudget(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	if err := e.c.Call(proto.MethodTokensSetSettings, proto.TokensSettings{On: true}, nil); err != nil {
		t.Fatal(err)
	}
	id := e.spawn(proto.SessionCreateParams{})
	reply := standinapi.Text("Counted.")
	reply.Usage = standinapi.Usage{Input: 12345, Output: 678}
	e.api.Enqueue(reply)
	e.say(id, "count me", 1)
	var out string
	e.wait(id, "usage to count it", func() bool {
		out, _ = e.run("usage", "session", id, "--json")
		return strings.Contains(out, "12345")
	})

	// /budget typed: past it, the next request asks first.
	if err := e.c.Input(id, "/budget 0.000001", false, true); err != nil {
		t.Fatal(err)
	}
	e.wait(id, "the budget set", func() bool { return e.transcript(id).BudgetUSD > 0 && e.transcript(id).BudgetUSD < 0.001 })
	n := len(e.api.Requests())
	e.send(id, "spend more")
	e.waitState(id, proto.StateWaiting)
	if !strings.Contains(e.screen(id), "y") {
		t.Errorf("screen:\n%s", e.screen(id))
	}
	if got := len(e.api.Requests()); got != n {
		t.Errorf("it spent over budget without asking: %d requests, was %d", got, n)
	}
	if err := e.c.Input(id, "y", false, false); err != nil {
		t.Fatal(err)
	}
	e.waitTurns(id, n+1)

	// --max-budget on the command line, in a session of the program by hand.
	flagged, err := e.c.CreateSession(proto.SessionCreateParams{Command: []string{e2eBin, "claude-platform", "--max-budget", "0.000001"}, Project: e.work, Cols: 120, Rows: 40})
	if err != nil {
		t.Fatal(err)
	}
	e.wait(flagged.ID, "ready", func() bool { return strings.Contains(e.screen(flagged.ID), "❯") })
	reply = standinapi.Text("Over it now.")
	reply.Usage = standinapi.Usage{Input: 5000}
	e.api.Enqueue(reply)
	n = len(e.api.Requests())
	e.send(flagged.ID, "first")
	e.wait(flagged.ID, "the first answer", func() bool {
		return len(e.api.Requests()) > n && strings.Contains(e.screen(flagged.ID), "Over it now.")
	})
	e.wait(flagged.ID, "ready again", func() bool { return strings.Contains(e.screen(flagged.ID), "❯") })
	n = len(e.api.Requests())
	e.send(flagged.ID, "second")
	e.wait(flagged.ID, "the budget's question", func() bool {
		return strings.Contains(e.screen(flagged.ID), "budget") && strings.Contains(e.screen(flagged.ID), "y/n")
	})
	if got := len(e.api.Requests()); got != n {
		t.Errorf("--max-budget: spent without asking, %d requests, was %d", got, n)
	}
	if err := e.c.Input(flagged.ID, "n", false, false); err != nil {
		t.Fatal(err)
	}
	e.wait(flagged.ID, "the turn ended on the no", func() bool { return strings.Contains(e.screen(flagged.ID), "The budget is spent") })
	if got := len(e.api.Requests()); got != n {
		t.Errorf("--max-budget: spent after a no: %d requests, was %d", got, n)
	}
}
