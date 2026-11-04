package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The latest model and effort said win, past a transcript too long to read
// whole, and a model changed partway shows as the new one.
func TestLastSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	line := func(model, effort string) string {
		return fmt.Sprintf(`{"type":"assistant","effort":%q,"message":{"role":"assistant","model":%q,"content":[{"type":"text","text":"hi"}]}}`, effort, model) + "\n"
	}
	body := line("claude-old", "low") + strings.Repeat(`{"type":"user","message":{"role":"user","content":"`+strings.Repeat("x", 1000)+`"}}`+"\n", 400)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LastSettings("claude", path); got.Model != "claude-old" || got.Effort != "low" {
		t.Fatalf("far from the end: %+v", got)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(line("claude-new", "high"))
	f.Close()
	if got := LastSettings("claude", path); got.Model != "claude-new" || got.Effort != "high" || got.ModelChosen != "" {
		t.Fatalf("after the change: %+v", got)
	}
	// /model says it at once, before any message on it.
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"user","message":{"role":"user","content":"<local-command-stdout>Set model to ` + "`" + `Haiku 5.5` + "`" + ` and saved as your default</local-command-stdout>"}}` + "\n")
	f.Close()
	if got := LastSettings("claude", path); got.Model != "Haiku 5.5" || got.ModelChosen != "Haiku 5.5" {
		t.Fatalf("after /model: %+v", got)
	}
	if got := LastSettings("claude", path+".gone"); got.Model != "" {
		t.Fatal("a guess for no file")
	}
}

// A fallback leaves what a session started on, /model replaces it, and an alias becomes what
// the agent first says it means.
func TestModelStart(t *testing.T) {
	for _, tc := range []struct {
		name, started string
		said          Settings
		want          string
	}{
		{"nothing asked: the first it says", "", Settings{Model: "claude-opus-5-5"}, "claude-opus-5-5"},
		{"an alias", "opus", Settings{Model: "claude-opus-5-5"}, "claude-opus-5-5"},
		{"the full name of what was asked", "claude-opus-5-5", Settings{Model: "claude-opus-5-5-20261101"}, "claude-opus-5-5-20261101"},
		{"a fallback", "claude-opus-5-5", Settings{Model: "claude-sonnet-5-5"}, "claude-opus-5-5"},
		{"/model before the next answer", "claude-opus-5-5", Settings{Model: "Haiku 5.5", ModelChosen: "Haiku 5.5"}, "Haiku 5.5"},
		{"/model's answer read as the name", "Haiku 5.5", Settings{Model: "claude-haiku-5-5", ModelChosen: "Haiku 5.5"}, "claude-haiku-5-5"},
		{"a fallback after /model", "claude-haiku-5-5", Settings{Model: "claude-sonnet-5-5", ModelChosen: "Haiku 5.5"}, "claude-haiku-5-5"},
	} {
		if got := ModelStart(tc.started, tc.said); got != tc.want {
			t.Errorf("%s: %q", tc.name, got)
		}
	}
	if SameModel("claude-opus-5-5", "claude-sonnet-5-5") || !SameModel("anthropic/claude-opus-5-5[1m]", "Opus 5.5") {
		t.Error("SameModel")
	}
}
