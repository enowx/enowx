package goa

import (
	"slices"
	"strings"
)

// goaConfigFlags are the flags of a config GOA takes: what model and effort it runs on.
// A config's other flags come last on its command line, so they would win over what holds
// GOA (its tools, its sandbox, its MCP server, its permissions).
var goaConfigFlags = []string{"--model", "-m", "--effort"}

// goaConfigVars are the variables a config can't set for GOA: enowX's own (its hooks'
// program, GOA's mark) and those GOA's agent is held by.
var goaConfigVars = []string{"ENABLE_TOOL_SEARCH", "OPENCODE_CONFIG", "OPENCODE_CONFIG_CONTENT", "OPENCODE_CONFIG_DIR", "OPENCODE_PERMISSION"}

// ConfigArgs are the flags of a config's args GOA takes.
func ConfigArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		name, _, inline := strings.Cut(args[i], "=")
		if !slices.Contains(goaConfigFlags, name) {
			continue
		}
		out = append(out, args[i])
		if !inline && i+1 < len(args) {
			i++
			out = append(out, args[i])
		}
	}
	return out
}

// ConfigEnv are the variables of a config's env GOA takes.
func ConfigEnv(env []string) []string {
	var out []string
	for _, v := range env {
		name, _, _ := strings.Cut(v, "=")
		if strings.HasPrefix(name, "ENOWX_") || slices.Contains(goaConfigVars, name) {
			continue
		}
		out = append(out, v)
	}
	return out
}
