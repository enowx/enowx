package agentapi

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// Each agent wired from its docs: what its session gets, the key in the
// environment alone, never in a file or on its command line.
func TestSessionRoutes(t *testing.T) {
	const url = "https://gw.example.com/v1"
	one := func(proto string) API {
		return API{BaseURL: url, APIKey: routeKey, Protocol: proto, Model: "m",
			Models: []Model{{ID: "m", Context: 64000, Output: 8000, Vision: true}}}
	}
	for name, tc := range map[string]struct {
		id   string
		api  API
		env  []string // wanted exactly
		args []string // wanted exactly, the session's folder as {dir}
		file string   // a file of the session's that must name the key's variable
		ref  string
	}{
		"kilo": {id: "kilo", api: one(""), env: []string{kiloKeyVar + "=" + routeKey}, file: ".config/kilo/kilo.json", ref: "{env:" + kiloKeyVar + "}"},
		"qwen openai": {id: "qwen-code", api: one(OpenAI), env: []string{"OPENAI_API_KEY=" + routeKey, "OPENAI_BASE_URL=" + url, "OPENAI_MODEL=m"},
			args: []string{"--auth-type", "openai", "--model", "m"}},
		"qwen anthropic": {id: "qwen-code", api: one(Anthropic), env: []string{"ANTHROPIC_API_KEY=" + routeKey, "ANTHROPIC_BASE_URL=" + url, "ANTHROPIC_MODEL=m"},
			args: []string{"--auth-type", "anthropic", "--model", "m"}},
		"kimi openai": {id: "kimi", api: one(OpenAI), env: []string{"KIMI_MODEL_NAME=m", "KIMI_MODEL_API_KEY=" + routeKey, "KIMI_MODEL_BASE_URL=" + url,
			"KIMI_MODEL_PROVIDER_TYPE=openai", "KIMI_MODEL_MAX_CONTEXT_SIZE=64000", "KIMI_MODEL_CAPABILITIES=image_in"}},
		"kimi anthropic": {id: "kimi", api: API{BaseURL: url, APIKey: routeKey, Protocol: Anthropic, Model: "m"},
			env: []string{"KIMI_MODEL_NAME=m", "KIMI_MODEL_API_KEY=" + routeKey, "KIMI_MODEL_BASE_URL=" + url, "KIMI_MODEL_PROVIDER_TYPE=anthropic"}},
		"continue": {id: "continue", api: one(""), env: []string{continueKeyVar + "=" + routeKey}, args: []string{"--config", "{dir}/" + continueFile},
			file: continueFile, ref: "${{ secrets." + continueKeyVar + " }}"},
		"goose openai": {id: "goose", api: one(OpenAI), env: []string{"GOOSE_PROVIDER=openai", "GOOSE_MODEL=m", "GOOSE_PROVIDER__TYPE=openai", "GOOSE_PROVIDER__HOST=" + url,
			"GOOSE_PROVIDER__API_KEY=" + routeKey, "OPENAI_BASE_URL=" + url, "OPENAI_API_KEY=" + routeKey, "GOOSE_CONTEXT_LIMIT=64000", "GOOSE_MAX_TOKENS=8000"}},
		"goose anthropic": {id: "goose", api: API{BaseURL: url, APIKey: routeKey, Protocol: Anthropic, Model: "m"}, env: []string{"GOOSE_PROVIDER=anthropic", "GOOSE_MODEL=m", "GOOSE_PROVIDER__TYPE=anthropic",
			"GOOSE_PROVIDER__HOST=" + url, "GOOSE_PROVIDER__API_KEY=" + routeKey, "ANTHROPIC_HOST=" + url, "ANTHROPIC_API_KEY=" + routeKey}},
		"crush": {id: "crush", api: one(""), env: []string{"CRUSH_GLOBAL_CONFIG={dir}/crush", "CRUSH_GLOBAL_DATA={dir}/crush-data", crushKeyVar + "=" + routeKey},
			file: "crush/crush.json", ref: "$" + crushKeyVar},
	} {
		t.Run(name, func(t *testing.T) {
			if !PerSession(tc.id) {
				t.Fatalf("%s isn't per session", tc.id)
			}
			dir := t.TempDir()
			l, err := SessionLaunch(tc.id, tc.api, dir)
			if err != nil {
				t.Fatal(err)
			}
			env := l.Env
			if tc.id == "kilo" {
				// Its config is a variable, naming the key's.
				var content string
				env = nil
				for _, e := range l.Env {
					if v, ok := strings.CutPrefix(e, "KILO_CONFIG_CONTENT="); ok {
						content = v
					} else {
						env = append(env, e)
					}
				}
				if strings.Contains(content, routeKey) || !strings.Contains(content, tc.ref) || !strings.Contains(content, `"enowx/m"`) {
					t.Fatalf("KILO_CONFIG_CONTENT: %s", content)
				}
			}
			var want []string
			for _, e := range tc.env {
				want = append(want, strings.ReplaceAll(e, "{dir}", dir))
			}
			if !slices.Equal(env, want) {
				t.Errorf("env\n%q\nwant\n%q", env, want)
			}
			var args []string
			for _, a := range tc.args {
				args = append(args, strings.ReplaceAll(a, "{dir}", dir))
			}
			if !slices.Equal(l.Args, args) {
				t.Errorf("args %q, want %q", l.Args, args)
			}
			if found := keyInFiles(t, dir); len(found) > 0 {
				t.Errorf("the key is in %q", found)
			}
			if strings.Contains(strings.Join(l.Args, " "), routeKey) {
				t.Errorf("the key is on its command line: %q", l.Args)
			}
			if tc.file != "" {
				data, err := os.ReadFile(filepath.Join(dir, tc.file))
				if err != nil || !strings.Contains(string(data), tc.ref) {
					t.Errorf("%s doesn't name the key: %s, %v", tc.file, data, err)
				}
			}
		})
	}
}

// What a session can't start on says so as it starts, not in its agent.
func TestSessionRoutesRefuse(t *testing.T) {
	const url = "https://gw.example.com/v1"
	for name, tc := range map[string]struct {
		id  string
		api API
	}{
		"qwen without a model":  {"qwen-code", API{BaseURL: url, APIKey: routeKey}},
		"qwen's Responses API":  {"qwen-code", API{BaseURL: url, APIKey: routeKey, Model: "m", Protocol: OpenAIResponses}},
		"kimi without a model":  {"kimi", API{BaseURL: url, APIKey: routeKey}},
		"kimi without a key":    {"kimi", API{BaseURL: url, Model: "m"}},
		"kimi's Gemini API":     {"kimi", API{BaseURL: url, APIKey: routeKey, Model: "m", Protocol: Gemini}},
		"crush without a model": {"crush", API{BaseURL: url, APIKey: routeKey}},
		"goose without a model": {"goose", API{BaseURL: url, APIKey: routeKey}},
		"kilo without a model":  {"kilo", API{BaseURL: url, APIKey: routeKey}},
		"continue, no model":    {"continue", API{BaseURL: url, APIKey: routeKey}},
	} {
		if _, err := SessionLaunch(tc.id, tc.api, t.TempDir()); err == nil {
			t.Errorf("%s: started", name)
		}
	}
}

// A local server with no key: nothing names a variable that isn't set.
func TestSessionConfigWithoutAKey(t *testing.T) {
	dir := t.TempDir()
	l, err := SessionLaunch("crush", API{BaseURL: "http://localhost:8080/v1", Model: "m"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "crush", "crush.json"))
	if strings.Contains(string(data), crushKeyVar) || slices.ContainsFunc(l.Env, func(e string) bool { return strings.HasPrefix(e, crushKeyVar) }) {
		t.Fatalf("env %q, config %s", l.Env, data)
	}
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

// agy --help shows --model: a config's model reaches it, and it stays optional.
func TestAntigravitySessionTakesAModel(t *testing.T) {
	l, err := SessionLaunch("antigravity", API{BaseURL: "https://gw.example.com", APIKey: routeKey, Model: "m"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(l.Args, " ") != "--model m" || !contains(l.Env, "GEMINI_API_KEY="+routeKey) {
		t.Fatalf("args %q, env %q", l.Args, l.Env)
	}
	l, err = SessionLaunch("antigravity", API{BaseURL: "https://gw.example.com", APIKey: routeKey}, t.TempDir())
	if err != nil || len(l.Args) != 0 {
		t.Fatalf("no model: args %q, %v", l.Args, err)
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
