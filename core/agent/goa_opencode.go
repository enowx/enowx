package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/internal/quietcmd"
	"github.com/enowx/enowx/proto"
)

// GOD of Agents on OpenCode: no hooks, so what holds it is what its agent is allowed,
// in a config that differs by version. Both were run with a model forced to try each
// tool (docs/goa.md): from version 2 policies, which hold for any agent the user
// switches to, and with --standalone, or its background server ignores this config;
// before it an agent's permissions, which another agent escapes, so a plugin too.

// IsOpenCode says whether id is one of OpenCode's presets.
func IsOpenCode(id string) bool { return id == "opencode" || id == "opencode2" }

var openCodeVersion = regexp.MustCompile(`(\d+)\.\d+\.\d+`)

// OpenCodeMajor is the major version of command, an OpenCode; 0 when it doesn't say.
func OpenCodeMajor(command string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := quietcmd.CommandContext(ctx, command, "--version")
	cmd.WaitDelay = time.Second // what it left running mustn't hold the session's start
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	m := openCodeVersion.FindSubmatch(out)
	if m == nil {
		return 0
	}
	major, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return 0
	}
	return major
}

// goaOpenCodeOwn are what GOA may not write in its own folder: OpenCode loads them
// from there at its next start, plugins running as programs.
var goaOpenCodeOwn = []string{".opencode/*", ".opencode", "opencode.json", "opencode.jsonc"}

// goaOpenCodeSecrets are the files it doesn't read: allowing read lifts OpenCode's own
// care of them.
var goaOpenCodeSecrets = []string{"*.env", "*.env.*", "*.pem", "*.p12", "*id_rsa*", "*id_ed25519*", "*credentials*"}

// goaOpenCodeServer is enowX's MCP server as GOA's.
func goaOpenCodeServer(exe string) map[string]any {
	return map[string]any{"type": "local", "command": []string{exe, "mcp"}, "environment": map[string]string{proto.EnvGOA: "1"}}
}

// goaOpenCode2Config is GOA's config from OpenCode 2 on.
func goaOpenCode2Config(exe, guide string) map[string]any {
	type rule = map[string]string
	perms := []rule{{"action": "*", "resource": "*", "effect": "deny"}}
	policies := []rule{{"action": "tool.use", "resource": "*", "effect": "deny"}}
	add := func(action, resource, effect string) {
		perms = append(perms, rule{"action": action, "resource": resource, "effect": effect})
		policies = append(policies, rule{"action": "tool.use", "resource": action + ":" + resource, "effect": effect})
	}
	add("enowx_*", "*", "allow")
	for _, a := range []string{"read", "glob", "grep", "external_directory", "edit"} {
		add(a, "*", "allow")
	}
	for _, r := range goaOpenCodeSecrets {
		add("read", r, "deny")
	}
	for _, r := range append([]string{"/*", "../*"}, goaOpenCodeOwn...) {
		add("edit", r, "deny")
	}
	server := goaOpenCodeServer(exe)
	server["codemode"] = false // or its tools hide behind one that was denied
	return map[string]any{
		"default_agent": "goa",
		"agents": map[string]any{"goa": map[string]any{
			"mode": "primary", "description": "GOD of Agents: delegates only",
			"system": "{file:" + guide + "}", "permissions": perms,
		}},
		"mcp":      map[string]any{"enowx": server},
		"policies": policies,
	}
}

// goaOpenCode1Config is GOA's config before OpenCode 2. Patterns are relative to the
// root of its worktree: / outside a git repository, as GOA's folder is.
func goaOpenCode1Config(exe, goaDir, guide string) map[string]any {
	rel := strings.TrimPrefix(filepath.ToSlash(goaDir), "/")
	edit := map[string]string{"*": "deny", rel + "/**": "allow"}
	for _, f := range goaOpenCodeOwn {
		edit[rel+"/"+f] = "deny"
	}
	read := map[string]string{"*": "allow"}
	for _, r := range goaOpenCodeSecrets {
		read[r] = "deny"
	}
	off := map[string]any{"disable": true}
	server := goaOpenCodeServer(exe)
	server["enabled"] = true
	return map[string]any{
		"default_agent": "goa",
		"agent": map[string]any{
			"goa": map[string]any{
				"mode": "primary", "description": "GOD of Agents: delegates only",
				"prompt": "{file:" + guide + "}",
				"permission": map[string]any{"*": "deny", "enowx_*": "allow", "read": read, "grep": "allow", "glob": "allow",
					"external_directory": "allow", "edit": edit},
			},
			"build": off, "plan": off, "general": off, "explore": off,
		},
		"mcp":      map[string]any{"enowx": server},
		"keybinds": map[string]string{"agent_cycle": "none", "agent_cycle_reverse": "none", "agent_list": "none"},
	}
}

// goaOpenCodePlugin holds GOA before OpenCode 2 under any agent the user picks, and
// through a symlink in its folder. Its list is one of what is allowed: a tool of a later
// version is refused.
const goaOpenCodePlugin = `import { realpathSync } from "node:fs"
import { dirname, resolve, sep, basename, relative } from "node:path"
const GOA = realpathSync(process.env.ENOWX_GOA_DIR)
const SECRET = /(^|[\/\\])\.env(\.|$)|id_rsa|id_ed25519|\.pem$|\.p12$|credentials/
const OWN = /^(\.opencode([\/\\]|$)|opencode\.jsonc?$)/
const real = (p) => {
  try { return realpathSync(p) } catch { return resolve(real(dirname(p)), basename(p)) }
}
export const Goa = async () => ({ "tool.execute.before": async (input, output) => {
  const t = input.tool
  if (t.startsWith("enowx_") || t === "grep" || t === "glob" || t === "list") return
  const f = output.args?.filePath
  if (t === "read") {
    if (!f || !SECRET.test(f)) return
    throw new Error("GOD of Agents doesn't read secrets: refused by enowX")
  }
  if ((t === "write" || t === "edit") && f) {
    const inside = relative(GOA, real(resolve(process.cwd(), f)))
    if (inside && !inside.startsWith("..") && !OWN.test(inside)) return
    throw new Error("GOD of Agents edits only in its own folder: hand this to a session (goa_delegate)")
  }
  throw new Error("GOD of Agents only delegates: " + t + " refused by enowX. Hand this to a session (goa_delegate)")
}})
`

// GOAOpenCodeEnv is what GOA's OpenCode of major version gets in its environment: its
// config, and before version 2 the plugin, kept in dataDir out of GOA's own folder.
func GOAOpenCodeEnv(major int, exe, dataDir string) ([]string, error) {
	goaDir := filepath.Join(dataDir, "goa")
	guide := filepath.Join(goaDir, GOAGuideName)
	var cfg map[string]any
	var env []string
	if major >= 2 {
		cfg = goaOpenCode2Config(exe, guide)
	} else {
		cfg = goaOpenCode1Config(exe, goaDir, guide)
		dir := filepath.Join(dataDir, "goa-opencode")
		if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0o700); err != nil {
			return nil, fmt.Errorf("goa: opencode plugin: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "plugins", "goa.js"), []byte(goaOpenCodePlugin), 0o600); err != nil {
			return nil, fmt.Errorf("goa: opencode plugin: %w", err)
		}
		env = append(env, "OPENCODE_CONFIG_DIR="+dir, "ENOWX_GOA_DIR="+goaDir)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("goa: opencode config: %w", err)
	}
	return append(env, "OPENCODE_CONFIG_CONTENT="+string(b)), nil
}

// GOAAgentNote is what GOA's guide adds on agent id.
func GOAAgentNote(id string) string {
	if !IsOpenCode(id) {
		return ""
	}
	return `On this agent you have no shell and no subagents. Where this guide shows "$ENOWX_BIN" goa delegate, goa board or the like, call enowX's MCP tool of that name (goa_delegate, goa_board); for any other command, its enowx tool, with the command's words as its arguments ("sessions", or "read", ID). If enowX's tools aren't among yours yet they are still starting, so never say they don't exist: read a file in your folder, and they are there at your next step.

`
}
