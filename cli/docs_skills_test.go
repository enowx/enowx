package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

func TestDocsSkillPrintsThem(t *testing.T) {
	var out bytes.Buffer
	r := runner{out: &out}
	if err := r.docs([]string{"skill"}); err != nil {
		t.Fatal(err)
	}
	for _, s := range projectdocs.Skills() {
		if !strings.Contains(out.String(), s.Name+": Use when") {
			t.Errorf("the list has no %s: %s", s.Name, out.String())
		}
	}
	out.Reset()
	if err := r.docs([]string{"skill", "write-spec"}); err != nil || !strings.HasPrefix(out.String(), "---\nname: write-spec\n") {
		t.Fatalf("%v %q", err, out.String())
	}
	if err := r.docs([]string{"skill", "nope"}); err == nil || !strings.Contains(err.Error(), "write-spec") {
		t.Fatalf("a wrong name: %v", err)
	}
}

func TestDocsSkillsInstallKeepsWhatTheUserChanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var out bytes.Buffer
	r := runner{out: &out}
	path := filepath.Join(home, ".claude", "skills", "write-prd", "SKILL.md")

	// Nothing is written until asked.
	if err := r.docs([]string{"skill", "write-prd"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); err == nil {
		t.Fatal("reading a skill wrote to the agent's folder")
	}

	if err := r.docs([]string{"skills", "install"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "wrote: "+path) {
		t.Fatalf("doesn't say what it wrote: %s", out.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(data), "---\nname: write-prd\n") {
		t.Fatalf("%v %q", err, data)
	}
	// What the user put beside it stays through everything below.
	extra := filepath.Join(filepath.Dir(path), "mine.md")
	os.WriteFile(extra, []byte("mine"), 0o644)

	// A later install of an earlier version of ours updates it.
	s := projectdocs.Skill{Name: "write-prd", Text: "---\nname: write-prd\n---\nAn earlier version\n"}
	os.WriteFile(path, []byte(stampedSkill(s)), 0o644)
	out.Reset()
	if err := r.docs([]string{"skills", "install"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "wrote: "+path) || !strings.Contains(out.String(), "already there:") {
		t.Fatalf("after an earlier version: %s", out.String())
	}

	// The user's edit is kept, and said to be.
	changed := strings.Replace(string(data), "# Write the PRD", "# Our PRD", 1)
	os.WriteFile(path, []byte(changed), 0o644)
	out.Reset()
	if err := r.docs([]string{"skills", "install"}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("no refusal: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != changed || !strings.Contains(out.String(), "kept, changed by hand: "+path) {
		t.Fatalf("overwritten: %s", out.String())
	}
	if err := r.docs([]string{"skills", "install", "--force"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(data) {
		t.Fatal("--force didn't replace it")
	}
	if _, err := os.Stat(extra); err != nil {
		t.Fatalf("the user's file beside it: %v", err)
	}
}

func TestDocsSkillsInstallUsesEachAgentsFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	r := runner{out: &bytes.Buffer{}}
	for agent, folder := range map[string]string{
		"codex":    ".agents/skills",
		"shared":   ".agents/skills",
		"opencode": ".config/opencode/skills",
	} {
		if err := r.docs([]string{"skills", "install", "--agent", agent}); err != nil {
			t.Fatalf("%s: %v", agent, err)
		}
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(folder), "plan-work", "SKILL.md")); err != nil {
			t.Errorf("%s: %v", agent, err)
		}
	}
	if err := r.docs([]string{"skills", "install", "--agent", "gemini"}); err == nil {
		t.Error("installed for an agent with no skills folder")
	}
}

// mcpToolNames lists what the MCP server serves.
func mcpToolNames(t *testing.T, r runner) map[string]bool {
	t.Helper()
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	var out bytes.Buffer
	if err := r.mcp(in, &out); err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range reply.Result.Tools {
		names[tool.Name] = true
	}
	return names
}

func TestNotesPageAgentGetsTheSkillTool(t *testing.T) {
	t.Setenv(proto.EnvNotesPropose, "1")
	names := mcpToolNames(t, runner{out: &bytes.Buffer{}})
	if !names["docs_skill"] || !names["notes_read"] {
		t.Fatalf("served: %v", names)
	}
	if names["notes_delete"] || names["goa_board"] {
		t.Fatalf("too much served: %v", names)
	}
}

func TestNotesDocsSkillGroupsKeepOnlyWhatReads(t *testing.T) {
	groups := notesDocsGroups([]mcpGroup{
		{[]mcpTool{{name: "docs_new"}, {name: "docs_summary"}, {name: "docs_link"}}, runner.docs},
		{[]mcpTool{{name: "docs_gaps"}, {name: "docs_status"}, {name: "docs_export"}}, runner.docs},
		{[]mcpTool{{name: "docs_attach"}}, runner.docs},
		{docsSkillTools, runner.docs},
	})
	var kept []string
	for _, g := range groups {
		for _, tool := range g.tools {
			kept = append(kept, tool.name)
		}
	}
	if got := strings.Join(kept, ","); got != "docs_summary,docs_gaps,docs_skill" {
		t.Fatalf("kept %s", got)
	}
}
