package projectdocs

import (
	"regexp"
	"strings"
	"testing"
)

// What each skill must tell the agent to do, in the words of its commands.
var skillNeeds = map[string][]string{
	"project-brainstorm": {"docs gaps", "docs catalog", "docs add", "docs skip", "notes write", "--section", "rag search", "project context", "terserah kamu", "{{Assumption}}", "source"},
	"write-prd":          {"docs link", "docs status", "Acceptance", "--section"},
	"write-spec":         {"docs link", "Based on", "rag search", "again"},
	"plan-work":          {"docs plan", "goa delegate --doc", "docs gate", "- [ ] T-1:", "depends on T-1"},
	"build-from-spec":    {"docs gate", "docs summary", "notes check", "same turn", "Don't approve"},
	"review-against-prd": {"docs verify", "notes check", "evidence", "file:line", "Unverified"},
	"draw-diagram":       {"canvas guide", "canvas new", "--kind diagram", "canvas shape", "docs diagram", "--from", "embed", "schema"},
}

func TestSkillsSayWhatTheyMust(t *testing.T) {
	skills := Skills()
	if len(skills) != len(skillNeeds) {
		t.Fatalf("%d skills, want %d", len(skills), len(skillNeeds))
	}
	for _, s := range skills {
		needs, ok := skillNeeds[s.Name]
		if !ok {
			t.Errorf("unexpected skill %q", s.Name)
		}
		if !strings.HasPrefix(s.Description, "Use when") {
			t.Errorf("%s: a description says when to use it: %q", s.Name, s.Description)
		}
		// Agents read the front matter as YAML, where ": " would end the value.
		if strings.Contains(s.Description, ": ") {
			t.Errorf("%s: description breaks YAML: %q", s.Name, s.Description)
		}
		if !strings.HasPrefix(s.Text, "---\nname: "+s.Name+"\n") {
			t.Errorf("%s: front matter: %q", s.Name, s.Text[:40])
		}
		for _, want := range append(needs, "{{Unverified}}", "Change history", "Architecture Decision Records", "notes outline", "Only the owner approves") {
			if !strings.Contains(s.Text, want) {
				t.Errorf("%s doesn't say %q", s.Name, want)
			}
		}
		for _, banned := range []string{"canvas open", "canvas goto", "canvas compare", "—"} {
			if strings.Contains(s.Text, banned) {
				t.Errorf("%s says %q", s.Name, banned)
			}
		}
	}
	if s, ok := FindSkill("WRITE-PRD"); !ok || s.Name != "write-prd" {
		t.Errorf("FindSkill: %+v %v", s, ok)
	}
	if _, ok := FindSkill("nope"); ok {
		t.Error("found a skill that isn't")
	}
}

// A section a skill names exists in that document's template, when the template
// is there: an agent told to write to a heading that isn't there fails.
func TestSkillsNameRealSections(t *testing.T) {
	re := regexp.MustCompile(`notes (?:show|write) "([^"]+)" --section "([^"]+)"`)
	for _, s := range Skills() {
		for _, m := range re.FindAllStringSubmatch(s.Text, -1) {
			e := Find(m[1])
			if e == nil {
				t.Errorf("%s: no document %q", s.Name, m[1])
				continue
			}
			tmpl, err := Template(e.Key)
			if err != nil {
				continue // written elsewhere for now
			}
			if !strings.Contains(tmpl, "## "+m[2]) {
				t.Errorf("%s: %s has no section %q", s.Name, m[1], m[2])
			}
		}
	}
}

func TestIndexTextLeavesOutWhatRecords(t *testing.T) {
	body := "Project Docs: PRD · {{Approved}} · v2 · @9f8e7d6c\n\n## Goals\n\nBased on: [[Brief]] @1a2b3c4d\n\nOrders reach the warehouse within a day.\n\n## Users\n\n_To fill: each main kind of user._\n"
	got := IndexText(body)
	for _, gone := range []string{"Project Docs:", "Based on", "To fill", "@9f8e7d6c"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q is left in %q", gone, got)
		}
	}
	if !strings.Contains(got, "Orders reach the warehouse") || !strings.Contains(got, "## Goals") {
		t.Errorf("content lost: %q", got)
	}
	// Status changes don't change what is embedded.
	other, _ := SetStatus(body, StatusOutdated)
	if IndexText(other) != got {
		t.Error("a status change changes what is embedded")
	}
	// An ordinary note may say these words, and stays as it is.
	plain := "Based on: what we saw\n_To fill: later_\n"
	if IndexText(plain) != plain {
		t.Errorf("an ordinary note changed: %q", IndexText(plain))
	}
}
