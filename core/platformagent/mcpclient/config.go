package mcpclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/core/envsync"
)

// Kinds of server: what its entry's type says, or its fields when it has none.
const (
	kindStdio = "stdio"
	kindHTTP  = "http"
	kindSSE   = "sse"
)

// serverConfig is one server's entry, its variables expanded.
type serverConfig struct {
	name string
	kind string
	envsync.MCPServer
	// environ is what its program runs with: the caller's and its own env.
	environ []string
	// missing are variables it names that have no value.
	missing []string
	// secrets are the values no message may repeat.
	secrets []string
}

// readSpec reads an mcp.json from a path or its text into its servers' entries.
func readSpec(spec string) (map[string]json.RawMessage, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	text := []byte(spec)
	if !strings.HasPrefix(spec, "{") {
		var err error
		if text, err = os.ReadFile(spec); err != nil {
			return nil, fmt.Errorf("mcpclient: read config: %w", err)
		}
	}
	var cfg struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(text, &cfg); err != nil {
		// The decoder's message names a position and a type, never a value.
		return nil, fmt.Errorf("mcpclient: read config: %w", err)
	}
	return cfg.MCPServers, nil
}

// parseServer reads one entry and expands its variables from environ.
func parseServer(name string, entry json.RawMessage, environ []string) (serverConfig, error) {
	cfg := serverConfig{name: name}
	if sanitize(name) == "" {
		return cfg, errors.New("it has no name")
	}
	var raw struct {
		envsync.MCPServer
		Type string `json:"type"`
	}
	if err := json.Unmarshal(entry, &raw); err != nil {
		return cfg, errors.New("its entry isn't a server: it needs command, args and env, or type, url and headers")
	}
	switch {
	case raw.Type == kindHTTP || raw.Type == kindSSE:
		cfg.kind = raw.Type
		if raw.URL == "" {
			return cfg, fmt.Errorf("type %s needs a url", raw.Type)
		}
	case raw.Type != "" && raw.Type != kindStdio:
		return cfg, errors.New("its type isn't one of stdio, http, sse")
	case raw.Command != "":
		cfg.kind = kindStdio
	case raw.URL != "" && raw.Type == "":
		cfg.kind = kindHTTP
	default:
		return cfg, errors.New("it runs nothing and serves nowhere")
	}

	vars := environValues(environ)
	expand := func(s string) string { return expandVars(s, vars, &cfg.missing) }
	cfg.Command = expand(raw.Command)
	for _, a := range raw.Args {
		cfg.Args = append(cfg.Args, expand(a))
	}
	cfg.URL = expand(raw.URL)
	cfg.Env = map[string]string{}
	for _, k := range sortedKeys(raw.Env) {
		cfg.Env[k] = expand(raw.Env[k])
	}
	cfg.Headers = map[string]string{}
	for _, k := range sortedKeys(raw.Headers) {
		cfg.Headers[k] = expand(raw.Headers[k])
	}
	slices.Sort(cfg.missing)
	cfg.missing = slices.Compact(cfg.missing)

	// Not nil even when empty: a nil Env would give the program this process's, the
	// API key included.
	cfg.environ = append([]string{}, environ...)
	for _, k := range sortedKeys(cfg.Env) {
		cfg.environ = append(cfg.environ, k+"="+cfg.Env[k])
	}
	cfg.secrets = secretsOf(cfg)
	if cfg.kind != kindStdio {
		u, err := url.Parse(cfg.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return cfg, errors.New("its url isn't an http or https address")
		}
	}
	return cfg, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

var varRef = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

// expandVars replaces ${VAR} and ${VAR:-default}; one with neither stays as written
// and goes in missing.
func expandVars(s string, vars map[string]string, missing *[]string) string {
	return varRef.ReplaceAllStringFunc(s, func(ref string) string {
		m := varRef.FindStringSubmatch(ref)
		if v, ok := vars[envKey(m[1])]; ok {
			return v
		}
		if strings.Contains(ref, ":-") {
			return m[2]
		}
		*missing = append(*missing, m[1])
		return ref
	})
}

// environValues are environ's variables; the last of a name counts, as for a program.
func environValues(environ []string) map[string]string {
	vars := make(map[string]string, len(environ))
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			vars[envKey(k)] = v
		}
	}
	return vars
}

// secretsOf are what a server's entry holds that no message should repeat: its env
// and header values, a header's parts ("Bearer x"), its URL's query and password.
func secretsOf(cfg serverConfig) []string {
	var out []string
	add := func(v string) {
		out = append(out, v)
		out = append(out, strings.Fields(v)...)
	}
	for _, k := range sortedKeys(cfg.Env) {
		add(cfg.Env[k])
	}
	for _, k := range sortedKeys(cfg.Headers) {
		add(cfg.Headers[k])
	}
	if cfg.URL != "" {
		out = append(out, cfg.URL)
		if u, err := url.Parse(cfg.URL); err == nil {
			out = append(out, u.RawQuery)
			if p, ok := u.User.Password(); ok {
				out = append(out, p)
			}
			for _, vs := range u.Query() {
				out = append(out, vs...)
			}
		}
	}
	return out
}

// shortestSecret: shorter values are ordinary words, and hiding them garbles a message.
const shortestSecret = 4

// hide is text with the server's values and what looks like a secret taken out.
func (c serverConfig) hide(text string) string {
	secrets := slices.Clone(c.secrets)
	// The longest first, so one holding another goes whole.
	slices.SortStableFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	for _, v := range secrets {
		if len(v) >= shortestSecret {
			text = strings.ReplaceAll(text, v, "[hidden]")
		}
	}
	return diag.Scrub(text)
}

// hideOutput is hide for what a program printed itself, which can repeat anything in its
// environment and not only its own config: every value in there goes too.
func (c serverConfig) hideOutput(text string) string {
	inherited := c
	inherited.secrets = slices.Clone(c.secrets)
	for _, kv := range c.environ {
		if _, v, ok := strings.Cut(kv, "="); ok {
			inherited.secrets = append(inherited.secrets, v)
		}
	}
	return inherited.hide(text)
}
