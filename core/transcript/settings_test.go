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
	if m, e, _ := LastSettings("claude", path); m != "claude-old" || e != "low" {
		t.Fatalf("far from the end: %q %q", m, e)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(line("claude-new", "high"))
	f.Close()
	if m, e, _ := LastSettings("claude", path); m != "claude-new" || e != "high" {
		t.Fatalf("after the change: %q %q", m, e)
	}
	// /model says it at once, before any message on it.
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"user","message":{"role":"user","content":"<local-command-stdout>Set model to ` + "`" + `Haiku 5.5` + "`" + ` and saved as your default</local-command-stdout>"}}` + "\n")
	f.Close()
	if m, _, _ := LastSettings("claude", path); m != "Haiku 5.5" {
		t.Fatalf("after /model: %q", m)
	}
	if m, _, _ := LastSettings("claude", path+".gone"); m != "" {
		t.Fatal("a guess for no file")
	}
}
