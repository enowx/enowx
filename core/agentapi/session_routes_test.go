package agentapi

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const routeKey = "fake-route-key-9876"

// keyInFiles is every file under dir holding the key.
func keyInFiles(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), routeKey) {
			found = append(found, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// A providers.json of the user's would win over Copilot's variables: its
// sessions read an empty one instead, the key staying in the environment.
func TestCopilotSessionDoesNotReadTheUsersProviders(t *testing.T) {
	dir := t.TempDir()
	l, err := SessionLaunch("copilot", API{BaseURL: "https://gw.example.com/v1", APIKey: routeKey, Model: "m"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "copilot-providers.json")
	if !contains(l.Env, "COPILOT_PROVIDERS_CONFIG="+path) || !contains(l.Env, "COPILOT_PROVIDER_API_KEY="+routeKey) || !contains(l.Env, "COPILOT_PROVIDER_BASE_URL=https://gw.example.com/v1") {
		t.Fatalf("env %q", l.Env)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(data)) != `{"providers":[],"models":[]}` {
		t.Fatalf("providers %s, %v", data, err)
	}
	if found := keyInFiles(t, dir); len(found) > 0 || strings.Contains(strings.Join(l.Args, " "), routeKey) {
		t.Fatalf("the key in %q, args %q", found, l.Args)
	}
}

// Gemini CLI refuses plain http anywhere but on this machine: a config says so
// when it's made, not when its session fails to start.
func TestGeminiAddressIsHTTPSUnlessLocal(t *testing.T) {
	for url, ok := range map[string]bool{
		"https://gw.example.com":      true,
		"http://localhost:8080/genai": true,
		"http://127.0.0.1:8080":       true,
		"http://[::1]:8080":           true,
		"http://gw.example.com":       false,
		"http://localhost.evil.com":   false,
		"http://192.168.1.5:8080":     false,
	} {
		_, err := Prepare("gemini", API{BaseURL: url})
		if ok && err != nil {
			t.Errorf("%s refused: %v", url, err)
		}
		if !ok && (err == nil || !strings.Contains(err.Error(), "localhost")) {
			t.Errorf("%s: %v", url, err)
		}
	}
	if _, err := Prepare("codex", API{BaseURL: "http://gw.example.com"}); err != nil {
		t.Errorf("the rule reached another agent: %v", err)
	}
}
