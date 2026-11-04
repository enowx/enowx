package tools

import (
	"os"
	"strings"
)

// secretEnv are the credentials the agent itself runs on, which a program a tool starts
// has no business reading.
var secretEnv = map[string]bool{
	"ANTHROPIC_API_KEY":        true,
	"ANTHROPIC_AUTH_TOKEN":     true,
	"ANTHROPIC_CUSTOM_HEADERS": true, // can carry an authorization header
	"CLAUDE_API_KEY":           true,
	"CLAUDE_CODE_OAUTH_TOKEN":  true,
}

// CleanEnviron is env without what must never reach a program a tool starts: the API
// key and the agent's other credentials. All else stays, ENOWX_BIN included.
func CleanEnviron(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !secretName(envName(kv)) {
			out = append(out, kv)
		}
	}
	return out
}

// ForgetCredentials takes what CleanEnviron removes out of this process's own environment.
// /proc's copy of what it started with stays: that is what HardenProcess is for.
func ForgetCredentials() {
	for _, kv := range os.Environ() {
		if secretName(envName(kv)) {
			name, _, _ := strings.Cut(kv, "=")
			_ = os.Unsetenv(name)
		}
	}
}

// envName is the name of a KEY=value entry, folded where names ignore case. Windows has
// entries like "=C:=C:\dir", their name starting with the '='.
func envName(kv string) string {
	name := kv
	if len(kv) > 1 {
		if i := strings.IndexByte(kv[1:], '='); i >= 0 {
			name = kv[:i+1]
		}
	}
	return foldEnvName(name)
}

// secretName says name, as envName folds it, is a credential: one listed, or by its
// name one of the agent's own that the list hasn't caught up with.
func secretName(name string) bool {
	if secretEnv[name] {
		return true
	}
	if !strings.HasPrefix(name, "ANTHROPIC_") && !strings.HasPrefix(name, "CLAUDE_CODE_") {
		return false
	}
	for _, end := range []string{"_KEY", "_TOKEN", "_SECRET", "_PASSWORD"} {
		if strings.HasSuffix(name, end) {
			return true
		}
	}
	return false
}
