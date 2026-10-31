package agentapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// A real OpenCode, started as a session config starts it (OPENCODE_REAL=1):
// it reaches the config's API with the key from its environment, the
// model named, the user's own config untouched.
func TestOpenCodeSessionReal(t *testing.T) {
	if os.Getenv("OPENCODE_REAL") == "" {
		t.Skip("OPENCODE_REAL=1 runs the opencode installed here")
	}
	var mu sync.Mutex
	var auth, model string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			auth, model = r.Header.Get("Authorization"), in.Model
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":1,\"total_tokens\":6}}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	l, err := SessionLaunch("opencode", API{BaseURL: srv.URL + "/v1", APIKey: "k-test-123", Model: "tiny-1", Models: []Model{{ID: "tiny-1"}}}, dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "opencode", "run", "-m", "enowx/tiny-1", "say hi")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), l.Env...)
	out, _ := cmd.CombinedOutput()
	mu.Lock()
	defer mu.Unlock()
	if auth != "Bearer k-test-123" || model != "tiny-1" {
		t.Fatalf("the API got auth %q model %q; opencode said:\n%s", auth, model, out)
	}
	if data, _ := os.ReadFile(l.Env[0][len("OPENCODE_CONFIG="):]); strings.Contains(string(data), "k-test-123") {
		t.Fatal("the key was written to the session's config file")
	}
}

// Gemini CLI signs in with the config's key by GEMINI_CLI_AUTH_OVERRIDE,
// the one variable its source reads for this (settings.ts): its base URL
// alone would pick "gateway" auth, and an interactive start would ask.
func TestGeminiSessionSignsInWithTheKey(t *testing.T) {
	l, err := SessionLaunch("gemini", API{BaseURL: "https://llm.example.com", APIKey: "k", Model: "gemini-flash"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env := strings.Join(l.Env, " ")
	for _, want := range []string{"GEMINI_CLI_AUTH_OVERRIDE=gemini-api-key", "GEMINI_API_KEY=k", "GOOGLE_GEMINI_BASE_URL=https://llm.example.com", "GEMINI_MODEL=gemini-flash"} {
		if !strings.Contains(env, want) {
			t.Fatalf("no %s in %q", want, l.Env)
		}
	}
}
