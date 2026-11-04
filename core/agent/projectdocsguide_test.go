package agent

import (
	"slices"
	"strings"
	"testing"
)

func TestProjectDocsIsAFeatureServingItsTools(t *testing.T) {
	f, ok := FindFeature("projectdocs")
	if !ok || !f.Default || f.Group != GroupNotes || !slices.Contains(f.Requires, "notes") {
		t.Fatalf("feature: %+v", f)
	}
	for _, tool := range []string{"docs_new", "docs_summary", "docs_skill"} {
		if got := FeatureOfTool(tool); got != "projectdocs" {
			t.Errorf("%s belongs to %q", tool, got)
		}
	}
	if !strings.Contains(GuideFor(DefaultFeatures(), true), "docs gate") {
		t.Error("the guide doesn't tell of the gate")
	}
	if strings.Contains(GuideFor(map[string]bool{}, true), "docs gate") {
		t.Error("turned off, it still costs the guide")
	}
	if n := strings.Count(ProjectDocsGuide, "\n"); n > 15 {
		t.Errorf("the guide part is %d lines", n)
	}
}

func TestNotesPageAgentMayOnlyReadProjectDocs(t *testing.T) {
	args := strings.Join(NotesArgs("claude", "/bin/enowx", ""), " ")
	for _, tool := range NotesDocsTools {
		if !strings.Contains(args, "mcp__enowx__"+tool) {
			t.Errorf("%s isn't allowed", tool)
		}
	}
	for _, tool := range []string{"docs_new", "docs_status", "docs_link", "docs_add", "docs_skip", "docs_attach", "docs_export", "docs_gate"} {
		if strings.Contains(args, tool) || slices.Contains(NotesDocsTools, tool) {
			t.Errorf("%s is allowed", tool)
		}
	}
	if !strings.Contains(NotesGuide, "project-brainstorm") {
		t.Error("the Notes page's agent isn't told how to brainstorm")
	}
}
