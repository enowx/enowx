package agentapi

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Each agent's way of taking an API, from its documentation and source
// (docs/agent-api.md says where): adapters_env.go for those that read the
// environment, adapters_json.go and adapters_config.go for the files.
// What enowX adds it names "enowx", so it finds it again and leaves the
// user's own providers alone.

const ourName = "enowx"

func init() {
	adapters["claude"] = claudeCode
	adapters["codex"] = codex
	adapters["opencode"] = openCodeLike("opencode", ".config/opencode/opencode.json")
	adapters["opencode2"] = adapters["opencode"]
	adapters["kilo"] = openCodeLike("kilo", ".config/kilo/kilo.json")
	adapters["gemini"] = geminiCLI
	adapters["antigravity"] = antigravity
	adapters["qwen-code"] = qwenCode
	adapters["droid"] = droid
	adapters["goose"] = goose
	adapters["aider"] = aider
	adapters["kimi"] = kimi
	adapters["mistral-vibe"] = vibe
	adapters["copilot"] = copilot
	adapters["openclaude"] = openClaude
	adapters["continue"] = continueCLI
	adapters["pi"] = pi
	adapters["crush"] = crush
}

// Caps most agents with a list of models have.
var listCaps = Caps{Multi: true, Vision: true, Context: "model", Output: "model"}

func withEfforts(c Caps, scope string, efforts ...string) Caps {
	c.Effort, c.Efforts = scope, efforts
	return c
}

// slug is id as a name of a table or a profile.
var slugRe = regexp.MustCompile(`[^a-z0-9_-]+`)

func slug(id string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(id), "-"), "-")
}

func removeFile(home, rel string) error {
	err := os.Remove(filepath.Join(home, rel))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func yamlQuote(s string) string { return strconv.Quote(s) }

func num(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// orDefault is n, else def: for keys an agent takes only together.
func orDefault(n, def int) int {
	if n > 0 {
		return n
	}
	return def
}

// itoa is a token count for a variable or a TOML value, "" for none.
func itoa(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}
