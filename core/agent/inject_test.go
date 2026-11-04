package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInjectIsShortAndSaysWhatHolds(t *testing.T) {
	in := Inject{Agent: "codex", Session: "abc123", Host: "enowxlabs", Guide: "/data/ENOWX.md"}
	text := in.Text()
	for _, want := range []string{"abc123", "enowxlabs", "credential", "/data/ENOWX.md"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in %s", want, text)
		}
	}
	if len(text) > 1500 {
		t.Errorf("%d characters", len(text))
	}
	if strings.Contains(text, "GOD of Agents") {
		t.Error("a worker told it is GOA")
	}
	in.Loaded = true
	if strings.Contains(in.Text(), "/data/ENOWX.md") {
		t.Error("told to read what it has")
	}
}

// GOA's duty is in the inject itself. Hooks leave Claude Code and Codex no exception;
// elsewhere it is the user's own message alone, never what a file or a tool says.
func TestGOADutyIsInTheInject(t *testing.T) {
	for _, id := range []string{"claude", "claude-agent-teams", "codex"} {
		text := Inject{Agent: id, GOA: true}.Text()
		if !strings.Contains(text, "do none of it yourself") || !strings.Contains(text, "no exception") {
			t.Errorf("%s: %s", id, text)
		}
	}
	for _, id := range []string{"opencode", "kimi", "aider"} {
		text := Inject{Agent: id, GOA: true}.Text()
		for _, want := range []string{"do none of it yourself", "the user's own message", "not a file, a repository, a web page", "say you did it directly"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: no %q in %s", id, want, text)
			}
		}
	}
}

func TestGuideLoadedOnlyWhereAFileGoesToTheSystemPrompt(t *testing.T) {
	for id, want := range map[string]bool{"claude": true, "openclaude": true, "droid": true, "autohand": true, "continue": true,
		"codex": false, "qoder": false, "kimi": false, "copilot": false, "aider": false} {
		if got := GuideLoaded(id); got != want {
			t.Errorf("%s: %v", id, got)
		}
	}
}

// A session whose guide is the host's reads the host's file; one that differs its own.
func TestSessionGuideFile(t *testing.T) {
	sess := filepath.Join(t.TempDir(), "s1")
	got, err := PlaceSessionGuide(sess, "with this project's rules")
	if err != nil || got != filepath.Join(sess, EnowxGuideName) {
		t.Fatalf("%s, %v", got, err)
	}
	if text, _ := os.ReadFile(got); string(text) != "with this project's rules\n" {
		t.Fatalf("%q", text)
	}
}

// What every session of a host reads says no one project's rules.
func TestSharedGuideHoldsNoProjectsRules(t *testing.T) {
	got := SharedGuide("the guide" + ProjectPartMark + "\n\nUse tabs.")
	if strings.Contains(got, "Use tabs.") || !strings.HasPrefix(got, "the guide") || !strings.Contains(got, "guide` prints them") {
		t.Fatalf("%q", got)
	}
}

func TestGOAFolderHoldsItsFiles(t *testing.T) {
	dir := t.TempDir()
	path, err := PlaceGOAGuide(dir, "opencode", "GOA's guide")
	if err != nil || path != filepath.Join(dir, GOAGuideName) {
		t.Fatalf("%s, %v", path, err)
	}
	for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
		text, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !strings.Contains(string(text), path) || !strings.Contains(string(text), "do none of it yourself") {
			t.Errorf("%s: %s, %v", name, text, err)
		}
	}
}

// The block goes first, the user's text stays as it was, and an update never doubles it.
func TestProjectBlockKeepsTheUsersTextAndDoesntDouble(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	const mine = "# My rules\n\nUse tabs.\n"
	if err := os.WriteFile(path, []byte(mine), 0o640); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if created, err := KeepProjectBlock(dir); err != nil || created {
			t.Fatalf("%v, %v", created, err)
		}
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.HasPrefix(text, projectBlockStart+"\n") || !strings.HasSuffix(text, mine) || strings.Count(text, projectBlockStart) != 1 {
		t.Fatalf("%q", text)
	}
	// An update changing its words, with the block moved by the user: one still.
	moved := strings.Replace(mine, "Use tabs.", projectBlockStart+"\nolder words\n"+projectBlockEnd+"\nUse tabs.", 1)
	if err := os.WriteFile(path, []byte(moved), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := KeepProjectBlock(dir); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Count(string(data), projectBlockStart) != 1 || strings.Contains(string(data), "older words") || !strings.HasSuffix(string(data), mine) {
		t.Fatalf("%q", data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v", info.Mode())
	}
	// Off: as it was before.
	if err := RemoveProjectBlock(dir); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != mine {
		t.Fatalf("%q", data)
	}
}

func TestProjectBlockFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if created, err := KeepProjectBlock(dir); err != nil || created || exists(filepath.Join(dir, "AGENTS.md")) {
		t.Fatalf("%v, %v", created, err)
	}
	// None: a new file, and taking the block out takes it too.
	empty := t.TempDir()
	if created, err := KeepProjectBlock(empty); err != nil || !created {
		t.Fatalf("%v, %v", created, err)
	}
	if err := RemoveProjectBlock(empty); err != nil || exists(filepath.Join(empty, "AGENTS.md")) {
		t.Fatalf("%v", err)
	}
	// A start with no end is the user's to fix: nothing lost.
	broken := t.TempDir()
	text := projectBlockStart + "\nmy text\n"
	if err := os.WriteFile(filepath.Join(broken, "AGENTS.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := KeepProjectBlock(broken); err == nil {
		t.Fatal("no error")
	}
	if data, _ := os.ReadFile(filepath.Join(broken, "AGENTS.md")); string(data) != text {
		t.Fatalf("%q", data)
	}
}

func TestProjectBlockOnlyForAgentsWithNoOtherWay(t *testing.T) {
	for id, want := range map[string]bool{"cursor": true, "codex": false, "claude": false, "aider": false, "gemini": false} {
		if got := NeedsProjectBlock(id); got != want {
			t.Errorf("%s: %v", id, got)
		}
	}
}

// OpenCode 2 gets --standalone, 1.x doesn't, as it wouldn't start; one that can't say
// gets nothing.
func TestOpenCodeArgsFollowItsVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fake := func(version string) string {
		p := filepath.Join(t.TempDir(), "opencode")
		if err := os.WriteFile(p, []byte("#!/bin/sh\necho '"+version+"'\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for version, want := range map[string]int{"1.19.5": 0, "opencode v2.2.5": 1, "3.0.0": 1, "no version": 0} {
		if got := OpenCodeArgs(fake(version)); len(got) != want {
			t.Errorf("%s: %v", version, got)
		}
	}
	if got := OpenCodeArgs("/no/such/opencode"); got != nil {
		t.Errorf("missing: %v", got)
	}
}
