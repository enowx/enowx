package hooks

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/proto"
)

// ToolEvent is event of a tool call: its command or the files it touches, read from its
// input in Claude Code's names. What holds a session to its rules (GOD of Agents', the
// user's hooks) reads these, so every agent's event is made here. cwd is the folder paths
// are told from.
func ToolEvent(event, tool string, input []byte, cwd string) proto.HookEvent {
	ev := proto.HookEvent{Event: event, Tool: tool}
	var in map[string]any
	if json.Unmarshal(input, &in) != nil {
		return ev
	}
	for _, k := range []string{"command", "cmd"} {
		if v, ok := in[k].(string); ok {
			ev.Command = v
			break
		}
	}
	for _, k := range []string{"file_path", "path", "notebook_path"} {
		if v, ok := in[k].(string); ok && v != "" {
			ev.Paths = append(ev.Paths, relTo(cwd, v))
		}
	}
	if ev.Command == "" && len(ev.Paths) == 0 {
		ev.Command = strings.TrimSpace(string(input))
	}
	return ev
}

// relTo is path p from folder dir, with slashes, when it is inside it.
func relTo(dir, p string) string {
	if dir != "" && filepath.IsAbs(p) {
		if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = rel
		}
	}
	return filepath.ToSlash(p)
}
