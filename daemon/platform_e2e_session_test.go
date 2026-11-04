//go:build !windows

package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/enowx/enowx/internal/standinapi"
	"github.com/enowx/enowx/proto"
)

// agentProcess is the PID of session id's agent program itself: the wrappers' shells
// hold the same words on their command lines, but aren't the program.
func (e *e2e) agentProcess(id string) int {
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		exe, _ := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		if exe != e2eBin {
			continue
		}
		data, _ := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		line := strings.ReplaceAll(string(data), "\x00", " ")
		if strings.Contains(line, "claude-platform") && strings.Contains(line, id) {
			return pid
		}
	}
	return 0
}

// agentCommandLine is what `ps` shows of it.
func (e *e2e) agentCommandLine(id string) string {
	pid := e.agentProcess(id)
	if pid == 0 {
		return ""
	}
	data, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	return strings.ReplaceAll(string(data), "\x00", " ")
}

// A hibernated session wakes on its conversation, cost and rules; so does one whose
// agent was killed in the middle of a tool, its history repaired.
func e2eResume(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{})

	e.api.Enqueue(standinapi.Calls(standinapi.ToolUse("toolu_first", "Bash", `{"command":"echo kept-rule"}`)))
	e.send(id, "remember the word pineapple")
	e.answer(id, proto.AnswerAlways)
	e.wait(id, "the command's result", func() bool {
		b, ok := lastTool(e.entries(id), "Bash")
		return ok && b.Done
	})
	e.waitState(id, proto.StateDone)
	costBefore := e.transcript(id).CostUSD
	if costBefore <= 0 {
		t.Fatalf("no cost: %v", costBefore)
	}
	before := e.agentProcess(id)

	if out, err := e.run("hibernate", id); err != nil {
		t.Fatalf("hibernate: %v: %s", err, out)
	}
	e.wait(id, "hibernated", func() bool { return e.info(id).Hibernated })
	e.wait(id, "its program gone", func() bool { return e.agentProcess(id) == 0 })
	if out, err := e.run("wake", id); err != nil {
		t.Fatalf("wake: %v: %s", err, out)
	}
	e.wait(id, "its program back", func() bool { p := e.agentProcess(id); return p != 0 && p != before })
	line := e.agentCommandLine(id)
	if !strings.Contains(line, "--continue") && !strings.Contains(line, "--resume") {
		t.Errorf("woken on a new conversation: %s", line)
	}
	e.wait(id, "the transcript back", func() bool { return e.transcript(id).CostUSD >= costBefore })

	// The rule kept still holds: the like of that command doesn't ask.
	e.api.Enqueue(standinapi.Calls(standinapi.ToolUse("toolu_second", "Bash", `{"command":"echo rule-held"}`)))
	e.say(id, "again", 2)
	r := lastRequest(t, e.api)
	if p := historyProblem(r); p != "" {
		t.Error(p)
	}
	if !strings.Contains(string(r.Body), "pineapple") {
		t.Errorf("the conversation didn't come back: %s", r.Body)
	}
	if b, _ := lastTool(e.entries(id), "Bash"); !strings.Contains(b.Result, "rule-held") {
		t.Errorf("after waking: %+v", b)
	}
	if got := e.transcript(id).CostUSD; got <= costBefore {
		t.Errorf("cost went from %v to %v", costBefore, got)
	}

	// Killed mid-tool: the next start repairs the call that never finished.
	costBefore = e.transcript(id).CostUSD
	e.api.Enqueue(standinapi.Calls(standinapi.ToolUse("toolu_killed", "Bash", `{"command":"echo x; sleep 27183"}`)))
	// That compound command has no rule: allow it, for it to run.
	e.send(id, "sleep then")
	e.answer(id, proto.AnswerAllow)
	e.wait(id, "sleep to run", func() bool { return processWith("sleep 27183") != 0 })
	killed := e.agentProcess(id)
	if killed == 0 {
		t.Fatal("no agent process")
	}
	if err := syscall.Kill(killed, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if pid := processWith("sleep 27183"); pid != 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	e.wait(id, "the agent gone", func() bool { return e.agentProcess(id) == 0 })
	// Its session's shell takes over when an agent dies, as for any agent; stopping that
	// is what lets it start again.
	if out, err := e.run("stop", id); err != nil {
		t.Fatalf("stop: %v: %s", err, out)
	}
	e.wait(id, "stopped", func() bool { i := e.info(id); return i.Exited || i.Hibernated })
	if out, err := e.run("continue", id); err != nil {
		t.Fatalf("continue: %v: %s", err, out)
	}
	e.wait(id, "its program back", func() bool { return e.agentProcess(id) != 0 })
	e.wait(id, "ready for a message", func() bool { return strings.Contains(e.screen(id), "❯") })
	if !strings.Contains(e.agentCommandLine(id), "--continue") && !strings.Contains(e.agentCommandLine(id), "--resume") {
		t.Errorf("continued on a new conversation: %s", e.agentCommandLine(id))
	}
	e.say(id, "what happened", 1)
	r = lastRequest(t, e.api)
	if p := historyProblem(r); p != "" {
		t.Errorf("%s: %s", p, r.Body)
	}
	if !strings.Contains(string(r.Body), "toolu_killed") {
		t.Errorf("the killed call is gone from the history: %s", r.Body)
	}
	if got := e.transcript(id).CostUSD; got < costBefore {
		t.Errorf("the cost went back from %v to %v", costBefore, got)
	}
}

// Modes by session.mode, model and effort typed: they read back, the next request carries
// them, and the cached prefix's bytes don't move.
func e2eSettings(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{})
	e.say(id, "first", 1)
	first := lastRequest(t, e.api)

	// The mode as its status line, the screen's last, says: what the daemon goes by.
	shown := map[string]string{"plan": "plan mode on", "acceptEdits": "accept edits on", "default": "ask first mode on", "bypassPermissions": "bypass permissions on"}
	switchTo := func(mode string) {
		t.Helper()
		if err := e.c.SwitchMode(id, mode); err != nil {
			t.Fatal(err)
		}
		e.wait(id, "mode "+mode+" on its status line", func() bool {
			lines := strings.Split(strings.TrimSpace(e.screen(id)), "\n")
			return len(lines) >= 2 && strings.HasPrefix(lines[len(lines)-2], shown[mode]) && e.transcript(id).Mode == mode
		})
	}
	for _, mode := range []string{"plan", "acceptEdits", "default", "plan"} {
		switchTo(mode)
	}
	e.api.Enqueue(standinapi.Calls(standinapi.ToolUse("toolu_plan", "Write", `{"file_path":"plan.txt","content":"no"}`)))
	e.say(id, "write in plan mode", 2)
	if w, _ := lastTool(e.entries(id), "Write"); !w.Failed || !strings.Contains(w.Result, "plan mode") {
		t.Errorf("Write in plan mode: %+v", w)
	}
	if _, err := os.Stat(filepath.Join(e.work, "plan.txt")); err == nil {
		t.Error("plan mode wrote a file")
	}

	type typed struct{ text, want string }
	for _, c := range []typed{{"/model claude-sonnet-5-5", "claude-sonnet-5-5"}, {"/effort low", "low"}} {
		if err := e.c.Input(id, c.text, false, true); err != nil {
			t.Fatal(err)
		}
		e.wait(id, c.text+" on screen", func() bool { return strings.Contains(e.screen(id), c.want) })
	}
	e.wait(id, "the settings read back", func() bool {
		tr := e.transcript(id)
		return tr.Model == "claude-sonnet-5-5" && tr.Effort == "low"
	})
	if err := e.c.Input(id, "next", true, true); err != nil {
		t.Fatal(err)
	}
	n := len(e.api.Requests())
	e.wait(id, "the next request", func() bool { return len(e.api.Requests()) > n || lastModel(e.api) == "claude-sonnet-5-5" })
	e.wait(id, "done", func() bool { return e.info(id).State == proto.StateDone })
	r := lastRequest(t, e.api)
	if got := lastModel(e.api); got != "claude-sonnet-5-5" {
		t.Errorf("the next request's model = %q", got)
	}
	if !strings.Contains(string(r.Raw("output_config")), `"low"`) {
		t.Errorf("the next request's effort: %s", r.Raw("output_config"))
	}
	if string(r.Raw("tools")) != string(first.Raw("tools")) {
		t.Error("the tools' bytes changed across turns")
	}
	if string(r.Raw("system")) != string(first.Raw("system")) {
		t.Error("the system prompt's bytes changed across turns")
	}
	// `switch`, the command line's way, reaches the same.
	if out, err := e.run("switch", id, "--model", "claude-haiku-5-5", "--effort", "medium"); err != nil {
		t.Fatalf("switch: %v: %s", err, out)
	}
	e.wait(id, "switched", func() bool {
		tr := e.transcript(id)
		return tr.Model == "claude-haiku-5-5" && tr.Effort == "medium"
	})
}

func lastModel(api *standinapi.Server) string {
	list := api.Requests()
	if len(list) == 0 {
		return ""
	}
	var model string
	_ = json.Unmarshal(list[len(list)-1].Raw("model"), &model)
	return model
}

// enowX's MCP server comes by --mcp-config from the daemon, its tools in the request as
// mcp__enowx__*, and a call of one runs.
func e2eMCP(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{SkipPermissions: true})
	if line := e.waitCommandLine(id); !strings.Contains(line, "--mcp-config") || !strings.Contains(line, filepath.Join("agent-files", id, "mcp.json")) {
		t.Errorf("command line: %s", line)
	}
	e.say(id, "list your tools", 1)
	names := toolNames(lastRequest(t, e.api))
	if !slices.ContainsFunc(names, func(n string) bool { return strings.HasPrefix(n, "mcp__enowx__") }) {
		t.Fatalf("no enowX tools in the request: %v", names)
	}
	if !slices.Contains(names, "mcp__enowx__sessions_list") {
		t.Fatalf("no sessions_list: %v", names)
	}
	e.api.Enqueue(standinapi.Calls(standinapi.ToolUse("toolu_mcp", "mcp__enowx__sessions_list", `{}`)))
	e.say(id, "list the sessions", 2)
	call, ok := lastTool(e.entries(id), "mcp__enowx__sessions_list")
	if !ok || call.Failed || !strings.Contains(call.Result, id) {
		t.Errorf("the MCP call: %+v", call)
	}
}

// A user's hook that rewrites a command has the agent run what it says instead.
func e2eHookRewrite(t *testing.T) {
	e := startE2E(t, e2eOptions{})
	hook := proto.Hook{On: true, Event: proto.HookToolBefore, Action: proto.HookActionRewrite, Find: `^echo before`, Replace: "echo after"}
	if err := e.c.Call(proto.MethodHooksSave, []proto.Hook{hook}, nil); err != nil {
		t.Fatal(err)
	}
	id := e.spawn(proto.SessionCreateParams{SkipPermissions: true})
	e.api.Enqueue(standinapi.Calls(standinapi.ToolUse("toolu_hook", "Bash", `{"command":"echo before"}`)))
	e.say(id, "run it", 2)
	if b, _ := lastTool(e.entries(id), "Bash"); !strings.Contains(b.Result, "after") {
		t.Errorf("the rewrite didn't run: %+v", b)
	}
}

func (e *e2e) waitCommandLine(id string) string {
	e.t.Helper()
	e.wait(id, "the agent's program", func() bool { return e.agentProcess(id) != 0 })
	return e.agentCommandLine(id)
}

// systemText is a request's system prompt, its blocks' text joined.
func systemText(r standinapi.Request) string {
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(r.Raw("system"), &blocks) != nil {
		return string(r.Raw("system"))
	}
	var all strings.Builder
	for _, b := range blocks {
		all.WriteString(b.Text + "\n")
	}
	return all.String()
}

// The guide reaches the request by its file; a prompt of the user's replaces the built-in
// one whole, the guide still after it; the app's RPC writes the same file.
func e2eSystemPrompt(t *testing.T) {
	const builtin = "You are Claude Platform, a coding agent"
	e := startE2E(t, e2eOptions{})
	id := e.spawn(proto.SessionCreateParams{})
	line := e.waitCommandLine(id)
	file := filepath.Join(e.data, "agent-files", id, "system-prompt.md")
	if !strings.Contains(line, "--append-system-prompt-file "+file) {
		t.Errorf("command line: %s", line)
	}
	guide := strings.TrimSpace(readT(file))
	if guide == "" {
		t.Fatal("no guide in " + file)
	}
	e.say(id, "hello", 1)
	system := systemText(lastRequest(t, e.api))
	if !strings.Contains(system, builtin) {
		t.Errorf("no built-in prompt: %.200s", system)
	}
	if !strings.Contains(system, guide) {
		t.Errorf("the guide isn't in the system prompt: %.400s", system)
	}

	// The app's editor: the next session starts on it.
	var saved proto.PlatformPrompt
	if err := e.c.Call(proto.MethodPlatformPromptSave, proto.PlatformPrompt{Text: "Saved through the app: be terse."}, &saved); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(e.data, "claude-platform", "system-prompt.md"); saved.Path != want || !saved.Custom {
		t.Errorf("saved %+v, want it at %s", saved, want)
	}
	second := e.spawn(proto.SessionCreateParams{})
	e.say(second, "hello", 1)
	system = systemText(lastRequest(t, e.api))
	if !strings.Contains(system, "Saved through the app") || strings.Contains(system, builtin) {
		t.Errorf("the saved prompt: %.300s", system)
	}
	if !strings.Contains(system, "ENOWX_BIN") {
		t.Errorf("the guide isn't after the saved prompt: %.300s", system)
	}

	// A file put there by hand, before the daemon starts.
	own := startE2E(t, e2eOptions{prepare: func(data string) {
		dir := filepath.Join(data, "claude-platform")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "system-prompt.md"), []byte("My own prompt, whole."), 0o600); err != nil {
			t.Fatal(err)
		}
	}})
	third := own.spawn(proto.SessionCreateParams{})
	own.say(third, "hello", 1)
	system = systemText(lastRequest(t, own.api))
	if !strings.Contains(system, "My own prompt, whole.") || strings.Contains(system, builtin) || !strings.Contains(system, "ENOWX_BIN") {
		t.Errorf("own prompt: %.300s", system)
	}
}
