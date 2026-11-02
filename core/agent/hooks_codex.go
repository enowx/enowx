package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Codex runs a hook only once the user trusted its definition, kept as
// the hash of it in config.toml (hooks.state."<source>:<event>:<group>:
// <handler>".trusted_hash). The hooks enowX passes on the command line
// (source "/<session-flags>/config.toml") are its own, the same in every
// session, so it trusts them itself, as Claude Code needs no trust for
// hooks in --settings; the user's own hooks still ask.

// codexFlagsSource is the source Codex names hooks given with -c by.
const codexFlagsSource = "/<session-flags>/config.toml"

// tomlInline is groups as a TOML inline value: what -c takes.
func tomlInline(groups []agentMatcher) string {
	var b strings.Builder
	b.WriteString("[")
	for i, g := range groups {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("{")
		if g.Matcher != "" {
			b.WriteString("matcher=" + strconv.Quote(g.Matcher) + ",")
		}
		b.WriteString("hooks=[")
		for j, h := range g.Hooks {
			if j > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "{type=%s,command=%s,timeout=%d}", strconv.Quote(h.Type), strconv.Quote(h.Command), h.Timeout)
		}
		b.WriteString("]}")
	}
	b.WriteString("]")
	return b.String()
}

// codexHookHash is the hash Codex keeps as trusted for handler h of a
// group without a matcher, of event label: its definition as JSON with
// sorted keys, without what it leaves unset.
func codexHookHash(label string, h agentHook) string {
	identity := map[string]any{
		"event_name": label,
		"hooks": []map[string]any{{
			"type": h.Type, "command": h.Command, "timeout": h.Timeout, "async": false,
		}},
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(identity)
	sum := sha256.Sum256(bytes.TrimSpace(b.Bytes()))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// TrustCodexHooks records enowX's own Codex hooks (codexHookArgs) as
// trusted in the config.toml of Codex home home ("" for the user's),
// adding to it and changing nothing else; a hook the user turned off
// stays off.
func TrustCodexHooks(home string) error {
	if home == "" {
		home = codexHome()
	}
	if home == "" || !hooksSupported() {
		return nil
	}
	path := filepath.Join(home, "config.toml")
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real // a home sharing the user's config: the user's
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("codex hooks: %w", err)
	}
	text := string(data)
	changed := false
	for _, e := range codexEvents {
		g := codexOn(e.event, e.state, e.timeout)
		key := fmt.Sprintf("%s:%s:0:0", codexFlagsSource, e.label)
		var ok bool
		text, ok = setTrustedHash(text, key, codexHookHash(e.label, g.Hooks[0]))
		changed = changed || ok
	}
	if !changed {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("codex hooks: %w", err)
	}
	tmp := path + ".enowx.tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o600); err != nil {
		return fmt.Errorf("codex hooks: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("codex hooks: %w", err)
	}
	return nil
}

// setTrustedHash sets trusted_hash in the table hooks.state."key" of
// config text, adding the table at the end when it has none; it reports
// whether text changed.
func setTrustedHash(text, key, hash string) (string, bool) {
	header := `[hooks.state.` + strconv.Quote(key) + `]`
	line := `trusted_hash = ` + strconv.Quote(hash)
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != header {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if strings.HasPrefix(t, "[") {
				lines = append(lines[:j], append([]string{line}, lines[j:]...)...)
				return strings.Join(lines, "\n"), true
			}
			if strings.HasPrefix(t, "trusted_hash") {
				if t == line {
					return text, false
				}
				lines[j] = line
				return strings.Join(lines, "\n"), true
			}
		}
		return strings.TrimRight(text, "\n") + "\n" + line + "\n", true
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + "\n" + header + "\n" + line + "\n", true
}
