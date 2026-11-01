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
