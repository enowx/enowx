package agent

import (
	"strings"
	"testing"
)

// What stops an agent is read from its screen; what merely runs is not.
func TestPromptShown(t *testing.T) {
	danger := "  Bash command\n  rm -rf \"$X\"\n  Dangerous rm operation on statically-unresolvable target\n  Do you want to proceed?\n  ❯ 1. Yes\n    2. No\n  Claude Code will block this request automatically in 1:51\n"
	p, ok := PromptShown(danger)
	if !ok || p.Text != "Do you want to proceed?" || p.Deadline != "1:51" {
		t.Fatalf("%+v %v", p, ok)
	}
	for _, screen := range []string{
		"△ Permission required\n Allow once  Allow always  Reject",
		"Is this a project you created or one you trust?\nDo you trust the files in this folder?\n",
		"New MCP server found in .mcp.json: gh\n",
		"You've reached your usage limit. It resets at 5pm\n",
		"Please sign in: open this url in your browser to sign in\n",
		"Update available! Press Enter to update\n",
		// What Claude Code showed in a folder it doesn't know.
		"Is this a project you created or one you trust?\n ❯ No, exit\n   Yes, I trust this folder\n Enter to confirm · Esc to cancel\n",
	} {
		if _, ok := PromptShown(screen); !ok {
			t.Errorf("not seen: %q", screen)
		}
	}
	if p, _ := PromptShown("  ┃  $ touch asked\n  ┃   Allow once   Allow always   Reject\n"); !strings.HasPrefix(p.Text, "$ touch asked: Allow once") {
		t.Fatalf("what it's about: %q", p.Text)
	}
	for _, screen := range []string{"$ ls\na.txt\n$ ", "Compiling…\n  esc to interrupt", ""} {
		if p, ok := PromptShown(screen); ok {
			t.Errorf("%q read as asking %q", screen, p.Text)
		}
	}
}

// What an interrupt leaves on the screen isn't an ask, with the composer and
// its status line under it or not; a real menu under it still is.
func TestInterruptLeftoverIsNoPrompt(t *testing.T) {
	for _, screen := range []string{
		"  ⎿  Interrupted · What should Claude do instead?\n",
		"● Fixing the parser\n  ⎿  Interrupted · What should Claude do instead?\n\n╭────╮\n❯ Try \"fix the tests\"\n╰────╯\n  ⏵⏵ bypass permissions on\n",
	} {
		if p, ok := StalledPrompt(screen); ok {
			t.Errorf("read as an ask: %q from %q", p.Text, screen)
		}
		if p, ok := PromptShown(screen); ok {
			t.Errorf("read as a known prompt: %q from %q", p.Text, screen)
		}
	}
	menu := "  ⎿  Interrupted · What should Claude do instead?\nWhich database should the new table go in?\n ❯ 1. Main\n   2. Replica\n"
	if p, ok := StalledPrompt(menu); !ok || !strings.Contains(p.Text, "database") {
		t.Errorf("a real menu missed: %+v %v", p, ok)
	}
}

// A prompt nobody listed is still seen, and work isn't taken for one.
func TestStalledPrompt(t *testing.T) {
	for _, screen := range []string{
		"Which database should the new table go in?\n ❯ 1. Main\n   2. Replica\n",
		"Overwrite the file? (y/n)",
		"Pick a theme\n ❯ Dark\n   Light\n   System\n",
	} {
		if _, ok := StalledPrompt(screen); !ok {
			t.Errorf("unseen prompt missed: %q", screen)
		}
	}
	for _, screen := range []string{
		"Running the tests\n⠋ thinking\n",
		"Why did it fail? Looking into it\n  esc to interrupt",
		"npm install\nadded 200 packages\n",
		"$ make build\ngo build ./...\n",
		"",
	} {
		if p, ok := StalledPrompt(screen); ok {
			t.Errorf("%q read as asking %q", screen, p.Text)
		}
	}
}
