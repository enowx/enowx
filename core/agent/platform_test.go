package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/proto"
)

func TestPlatformPreset(t *testing.T) {
	if PlatformID != record.AgentID {
		t.Fatalf("%s isn't the record's %s", PlatformID, record.AgentID)
	}
	if Presets[1].ID != PlatformID {
		t.Errorf("listed second? first two: %s, %s", Presets[0].ID, Presets[1].ID)
	}
	p, ok := Find(PlatformID)
	if !ok {
		t.Fatal("no preset")
	}
	t.Setenv(EnvBin, "/data/enowx-launcher")
	t.Setenv("PATH", t.TempDir()) // nothing of it on any PATH
	if !p.Installed() || p.Program() != "/data/enowx-launcher" || p.NamesakeHere() != "" || p.Install != "" {
		t.Errorf("installed %v, program %s", p.Installed(), p.Program())
	}
	argv := p.Argv(Launch{Prompt: "go", SessionDir: "/s", AllowSkip: true})
	if got := strings.Join(argv, " "); got != "/data/enowx-launcher claude-platform --session-dir /s --allow-dangerously-skip-permissions go" {
		t.Errorf("argv: %s", got)
	}
	argv = p.Argv(Launch{Resume: "c1", Continue: true, SkipPermissions: true})
	if got := strings.Join(argv, " "); got != "/data/enowx-launcher claude-platform --resume c1 --dangerously-skip-permissions" {
		t.Errorf("resume: %s", got)
	}
	t.Setenv(EnvBin, "")
	if p.Program() == "" || !strings.Contains(p.Program(), "agent") { // the test's own binary
		t.Errorf("without the variable: %s", p.Program())
	}
	for _, a := range List() {
		if a.ID == PlatformID && (!a.Installed || a.Command == "" || !a.CanSkipPermissions) {
			t.Errorf("listed as %+v", a)
		}
	}
}

func TestPlatformGuideAndTools(t *testing.T) {
	if !GuideLoaded(PlatformID) || NeedsProjectBlock(PlatformID) || GuideNote(PlatformID) != "" {
		t.Errorf("guide: loaded %v, block %v", GuideLoaded(PlatformID), NeedsProjectBlock(PlatformID))
	}
	dir := t.TempDir()
	args := append(ToolArgs(PlatformID, "/bin/enowx"), GuideArgsText(PlatformID, "the guide")...)
	moved, err := MoveToFiles(PlatformID, dir, args)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(moved, " ")
	if strings.Contains(got, "the guide") || !strings.Contains(got, "--append-system-prompt-file") || !strings.Contains(got, "mcp.json") {
		t.Errorf("not in files: %s", got)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	goa := GOAArgs(PlatformID, "/bin/enowx", "guide")
	for i, a := range goa {
		if a == "--mcp-config" {
			if err := json.Unmarshal([]byte(goa[i+1]), &cfg); err != nil {
				t.Fatal(err)
			}
		}
		if a == "--tools" || a == "--allowedTools" || a == "--strict-mcp-config" || a == "--settings" {
			t.Errorf("a flag it doesn't take: %s", a)
		}
	}
	if s := cfg.MCPServers["enowx"]; s.Command != "/bin/enowx" || s.Env[proto.EnvGOA] != "1" {
		t.Errorf("GOA's server: %+v", cfg)
	}
	if text := (Inject{Agent: PlatformID, GOA: true}).Text(); !strings.Contains(text, "no exception") {
		t.Errorf("GOA's duty: %s", text)
	}
	if GOALimitsTools(PlatformID) {
		t.Error("it has no flags to limit its tools")
	}
	if strings.Contains(strings.Join(NotesArgs(PlatformID, "/bin/enowx", ""), " "), "--allowedTools") {
		t.Error("notes: a flag it doesn't take")
	}
}

func TestPlatformHooks(t *testing.T) {
	if len(HookArgs(PlatformID)) != 0 || !ReportsItself(PlatformID) || ReportsItself("aider") {
		t.Errorf("reports itself: %v", ReportsItself(PlatformID))
	}
	s := HookSupport(PlatformID)
	for _, e := range []string{proto.HookToolBefore, proto.HookToolAfter, proto.HookPromptStart, proto.HookTurnDone, proto.HookSessionStart, proto.HookSessionEnd} {
		if s[e] != proto.HookInAgent {
			t.Errorf("%s: %q", e, s[e])
		}
	}
	if s[proto.HookWorking] != proto.HookOutside {
		t.Errorf("working: %q", s[proto.HookWorking])
	}
	if !JudgeApprovesByHook(PlatformID) || CanRewind(PlatformID) {
		t.Errorf("judge %v, rewind %v", JudgeApprovesByHook(PlatformID), CanRewind(PlatformID))
	}
	if err := Rewind(context.Background(), PlatformID, 1, "x", Screen{}); err == nil {
		t.Error("rewind says yes")
	}
	if p, _ := Find(PlatformID); len(p.Fork) != 0 || p.Approve.Allow != "1" || p.Approve.Always != "2" || p.Approve.Deny != "3" {
		t.Errorf("%+v", p)
	}
}

func TestPlatformSettings(t *testing.T) {
	s := SettingsOf(PlatformID)
	if s.Models[0].ID != record.DefaultModel {
		t.Errorf("first model: %s", s.Models[0].ID)
	}
	if s.ContextWindow("claude-opus-5-5") != 1_000_000 || s.ContextWindow("claude-haiku-4-5") != 200_000 {
		t.Errorf("windows")
	}
	if got := s.ModeShown("x\n  plan mode on (shift+tab to cycle)\n"); got != record.ModePlan {
		t.Errorf("mode shown: %q", got)
	}
	got := strings.Join(StartArgs(PlatformID, "claude-sonnet-5-5", "low", "plan", false), " ")
	if got != "--model claude-sonnet-5-5 --effort low --permission-mode plan" {
		t.Errorf("start args: %s", got)
	}
	cmds, err := Commands(context.Background(), PlatformID, "")
	if err != nil || len(cmds) == 0 {
		t.Errorf("commands: %v %v", cmds, err)
	}
}

func TestPlatformInATab(t *testing.T) {
	t.Setenv(EnvBin, "/data/launcher")
	got, ok := TabAgentCommand([]string{"/data/enowx-daemon", "claude-platform", "--model", "m", "--session-dir", "/s", "a prompt"})
	if !ok || strings.Join(got, " ") != "/data/launcher claude-platform --continue --model m --session-dir /s" {
		t.Errorf("%v %v", got, ok)
	}
	if _, ok := TabAgentCommand([]string{"/usr/bin/vim", "claude-platform"}); ok {
		t.Error("another program")
	}
}
