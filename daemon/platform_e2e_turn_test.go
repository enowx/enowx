//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/internal/standinapi"
	"github.com/enowx/enowx/proto"
)

// A first prompt: the session starts, its answer streams into one entry that grows, the
// final answer replaces it, the state goes working to done, and the settings read back.
func e2eFirstPrompt(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	hold := make(chan struct{})
	full := "The first half of the answer, and then the second half of the answer."
	e.api.Enqueue(standinapi.Reply{
		Blocks:    []standinapi.Block{standinapi.Thinking("Thinking it over.", "sig"), standinapi.TextBlock(full)},
		Hold:      hold,
		HoldAfter: 1,
		Usage:     standinapi.Usage{Input: 1000, CacheWrite: 500},
	})
	id := e.spawn(proto.SessionCreateParams{Prompt: "Say hello"})

	var partial proto.TranscriptEntry
	e.wait(id, "the answer streaming", func() bool {
		for _, entry := range e.entries(id) {
			if entry.Kind == proto.TranscriptAssistant && entry.Text != "" {
				partial = entry
				return true
			}
		}
		return false
	})
	if partial.Text == full || !strings.HasPrefix(full, partial.Text) {
		t.Fatalf("streamed %q of %q", partial.Text, full)
	}
	if st := e.info(id).State; st != proto.StateWorking {
		t.Errorf("state while it streams = %s", st)
	}
	close(hold)
	e.waitTurns(id, 1)
	e.wait(id, "the final answer", func() bool {
		return slices.ContainsFunc(e.entries(id), func(x proto.TranscriptEntry) bool {
			return x.Kind == proto.TranscriptAssistant && x.Text == full
		})
	})

	tr := e.transcript(id)
	if n := kindCount(tr.Entries, proto.TranscriptAssistant); n != 1 {
		t.Errorf("%d assistant entries, want one: %s", n, e.dumpEntries(id))
	}
	if n := kindCount(tr.Entries, proto.TranscriptThinking); n != 1 {
		t.Errorf("%d thoughts, want one: %s", n, e.dumpEntries(id))
	}
	for _, entry := range tr.Entries {
		if entry.Kind == proto.TranscriptAssistant && entry.Index != partial.Index {
			t.Errorf("its index moved from %d to %d", partial.Index, entry.Index)
		}
	}
	if tr.Model != "claude-opus-5-5" || tr.Effort != "high" || tr.Mode != "default" {
		t.Errorf("settings: model %q effort %q mode %q", tr.Model, tr.Effort, tr.Mode)
	}
	if tr.ContextTokens == 0 || tr.ContextWindow != 1_000_000 {
		t.Errorf("context: %d of %d", tr.ContextTokens, tr.ContextWindow)
	}
	if tr.CostUSD <= 0 || tr.BudgetUSD != 10 {
		t.Errorf("cost %v of budget %v", tr.CostUSD, tr.BudgetUSD)
	}
	if r := lastRequest(t, e.api); r.Header.Get("X-Api-Key") != e2eKey {
		t.Errorf("the request's key header is %q", r.Header.Get("X-Api-Key"))
	}
}

// Each tool: those that change something wait for the chat's answer, in its three forms;
// reads run unasked; an edit shows its diff; the key never reaches what a command sees.
func e2eTools(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{})
	call := func(name, input string) standinapi.Reply {
		return standinapi.Calls(standinapi.ToolUse("toolu_"+strings.ToLower(name), name, input))
	}

	// Reads don't ask.
	e.api.Enqueue(call("Read", `{"file_path":"README.md"}`))
	e.say(id, "read it", 2)
	if read, ok := lastTool(e.entries(id), "Read"); !ok || !read.Done || !strings.Contains(read.Result, "hello readme") {
		t.Errorf("Read: %+v", read)
	}
	e.api.Enqueue(call("Glob", `{"pattern":"*.md"}`))
	e.say(id, "glob it", 2)
	if glob, ok := lastTool(e.entries(id), "Glob"); !ok || !strings.Contains(glob.Result, "README.md") {
		t.Errorf("Glob: %+v", glob)
	}
	e.api.Enqueue(call("Grep", `{"pattern":"second","output_mode":"content"}`))
	e.say(id, "grep it", 2)
	if grep, ok := lastTool(e.entries(id), "Grep"); !ok || !strings.Contains(grep.Result, "second line") {
		t.Errorf("Grep: %+v", grep)
	}

	// Write, allowed once with the chat's "1"; the next asks again.
	e.api.Enqueue(call("Write", `{"file_path":"new.txt","content":"one\ntwo\n"}`))
	e.send(id, "write it")
	e.answer(id, proto.AnswerAllow)
	e.wait(id, "the file", func() bool { return readT(filepath.Join(e.work, "new.txt")) == "one\ntwo\n" })
	e.waitState(id, proto.StateDone)

	// Edit, its diff in the transcript; denied with "3", the file stays.
	e.api.Enqueue(call("Edit", `{"file_path":"new.txt","old_string":"two","new_string":"three"}`))
	e.send(id, "edit it")
	e.answer(id, proto.AnswerDeny)
	e.wait(id, "the denial", func() bool {
		edit, ok := lastTool(e.entries(id), "Edit")
		return ok && edit.Done && edit.Failed && strings.Contains(edit.Result, "denied")
	})
	e.waitState(id, proto.StateDone)
	if got := readT(filepath.Join(e.work, "new.txt")); got != "one\ntwo\n" {
		t.Errorf("a denied edit changed the file: %q", got)
	}
	e.api.Enqueue(call("Edit", `{"file_path":"new.txt","old_string":"two","new_string":"three"}`))
	e.send(id, "edit it again")
	e.answer(id, proto.AnswerAllow)
	e.wait(id, "the edit", func() bool { return readT(filepath.Join(e.work, "new.txt")) == "one\nthree\n" })
	e.waitState(id, proto.StateDone)
	edit, _ := lastTool(e.entries(id), "Edit")
	if edit.Old != "two" || edit.New != "three" || edit.Added != 1 || edit.Removed != 1 {
		t.Errorf("the edit's diff: %+v", edit)
	}

	// Bash with "2": kept as a rule, so the like of it doesn't ask again.
	e.api.Enqueue(call("Bash", `{"command":"echo always-one"}`))
	e.send(id, "run it")
	e.answer(id, proto.AnswerAlways)
	e.wait(id, "the command's result", func() bool {
		bash, ok := lastTool(e.entries(id), "Bash")
		return ok && bash.Done && strings.Contains(bash.Result, "always-one")
	})
	e.waitState(id, proto.StateDone)
	e.api.Enqueue(call("Bash", `{"command":"echo always-two"}`))
	e.say(id, "run it again", 2)
	if bash, _ := lastTool(e.entries(id), "Bash"); !strings.Contains(bash.Result, "always-two") {
		t.Errorf("the rule kept didn't cover it: %+v", bash)
	}

	// The key isn't in what a command sees, though the agent has it.
	e.api.Enqueue(call("Bash", `{"command":"env"}`))
	e.send(id, "show the environment")
	e.answer(id, proto.AnswerAllow)
	e.wait(id, "env's result", func() bool {
		bash, ok := lastTool(e.entries(id), "Bash")
		return ok && bash.Done && strings.Contains(bash.Result, "ENOWX_SESSION")
	})
	e.waitState(id, proto.StateDone)
	bash, _ := lastTool(e.entries(id), "Bash")
	if strings.Contains(bash.Result, e2eKey) || strings.Contains(bash.Result, "ANTHROPIC_API_KEY") {
		t.Errorf("env shows the key: %s", bash.Result)
	}
	if strings.Contains(string(lastRequest(t, e.api).Body), e2eKey) {
		t.Error("the key went to the model in a request's body")
	}
	if p := historyProblem(lastRequest(t, e.api)); p != "" {
		t.Error(p)
	}
}

// Skipping permissions asks nothing, for a command or a file.
func e2eBypass(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{SkipPermissions: true})
	e.api.Enqueue(standinapi.Calls(
		standinapi.ToolUse("toolu_w", "Write", `{"file_path":"by.txt","content":"x"}`),
		standinapi.ToolUse("toolu_b", "Bash", `{"command":"echo bypassed"}`),
	))
	e.say(id, "do it all", 2)
	if bash, _ := lastTool(e.entries(id), "Bash"); !strings.Contains(bash.Result, "bypassed") {
		t.Errorf("Bash: %+v", bash)
	}
	if readT(filepath.Join(e.work, "by.txt")) != "x" {
		t.Error("Write didn't run")
	}
	if tr := e.transcript(id); tr.Mode != "bypassPermissions" {
		t.Errorf("mode = %q", tr.Mode)
	}
}

// AskUserQuestion: an option's digit, and the chat's own-answer form, reach the agent.
func e2eQuestions(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{})
	ask := standinapi.Calls(standinapi.ToolUse("toolu_q", "AskUserQuestion", `{"questions":[{"header":"Pick","question":"Which one?","options":[{"label":"Alpha","description":"the first"},{"label":"Beta","description":"the second"}],"multiSelect":false}]}`))

	e.api.Enqueue(ask)
	e.send(id, "ask me")
	e.answer(id, "2")
	e.wait(id, "the answer's result", func() bool {
		q, ok := lastTool(e.entries(id), "AskUserQuestion")
		return ok && q.Done
	})
	e.waitTurns(id, 2)
	if body := string(lastRequest(t, e.api).Body); !strings.Contains(body, "Beta") {
		t.Errorf("the option picked didn't reach the agent: %s", body)
	}

	// What the chat's form sends: the option after the last, the words, Enter, in one go.
	ask.Blocks[0].ID = "toolu_q2"
	e.api.Enqueue(ask)
	n := len(e.api.Requests())
	e.send(id, "ask me again")
	e.waitState(id, proto.StateWaiting)
	e.wait(id, "the question on the screen", func() bool { return strings.Contains(e.screen(id), "Your own words") })
	if err := e.c.Input(id, "3my very own words\r", false, false); err != nil {
		t.Fatal(err)
	}
	e.waitTurns(id, n+2)
	if body := string(lastRequest(t, e.api).Body); !strings.Contains(body, "my very own words") {
		t.Errorf("the own answer didn't reach the agent: %s", body)
	}
	if p := historyProblem(lastRequest(t, e.api)); p != "" {
		t.Error(p)
	}
}

// processWith is a running process whose whole command line is exactly command, 0 for
// none: a program's arguments merely naming it don't count.
func processWith(command string) int {
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		var pid int
		if _, err := fmt.Sscanf(entry.Name(), "%d", &pid); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if strings.TrimRight(strings.ReplaceAll(string(data), "\x00", " "), " ") == command {
			return pid
		}
	}
	return 0
}

// Interrupted while a tool runs: the command dies, the next prompt works, and the
// history sent has a result for the call.
func e2eInterruptMidTool(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{SkipPermissions: true})
	e.api.Enqueue(standinapi.Calls(standinapi.ToolUse("toolu_sleep", "Bash", `{"command":"sleep 31337"}`)))
	e.send(id, "sleep a while")
	e.wait(id, "sleep to run", func() bool { return processWith("sleep 31337") != 0 })
	// A quiet command's screen still moves, or the session would read as stopped.
	e.wait(id, "the seconds counting", func() bool { return strings.Contains(e.screen(id), "esc to interrupt · 1s") })
	e.wait(id, "the seconds counting", func() bool { return strings.Contains(e.screen(id), "esc to interrupt · 2s") })
	if err := e.c.InterruptSession(id); err != nil {
		t.Fatal(err)
	}
	e.wait(id, "sleep to die", func() bool { return processWith("sleep 31337") == 0 })
	e.wait(id, "the call to end interrupted", func() bool {
		bash, ok := lastTool(e.entries(id), "Bash")
		return ok && bash.Done && bash.Failed
	})
	e.wait(id, "the turn to end", func() bool { return e.info(id).State == proto.StateDone })
	n := len(e.api.Requests())
	e.say(id, "go on", 1)
	if got := len(e.api.Requests()); got != n+1 {
		t.Errorf("requests went from %d to %d", n, got)
	}
	if p := historyProblem(lastRequest(t, e.api)); p != "" {
		t.Error(p)
	}
	if !strings.Contains(string(lastRequest(t, e.api).Body), "toolu_sleep") {
		t.Error("the interrupted call is gone from the history")
	}
}

// Interrupted while the answer streams: nothing half-made goes back to the API.
func e2eInterruptMidStream(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{})
	hold := make(chan struct{})
	defer close(hold)
	e.api.Enqueue(standinapi.Reply{
		Blocks:    []standinapi.Block{standinapi.TextBlock("a long answer that never ends, cut in its middle")},
		Hold:      hold,
		HoldAfter: 0,
	})
	e.send(id, "talk")
	e.wait(id, "the answer streaming", func() bool { return kindCount(e.entries(id), proto.TranscriptAssistant) == 1 })
	if err := e.c.InterruptSession(id); err != nil {
		t.Fatal(err)
	}
	e.wait(id, "the turn to end", func() bool { return e.info(id).State == proto.StateDone })
	e.say(id, "go on", 1)
	r := lastRequest(t, e.api)
	if p := historyProblem(r); p != "" {
		t.Error(p)
	}
	if !strings.Contains(string(r.Body), "go on") {
		t.Errorf("the next prompt isn't in the request: %s", r.Body)
	}
	if n := kindCount(e.entries(id), proto.TranscriptUser); n != 2 {
		t.Errorf("%d user entries: %s", n, e.dumpEntries(id))
	}
}
