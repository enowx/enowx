package agent

import (
	"maps"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/core/procstat"
)

// WindowsGOAProcesses recognizes only direct helpers of a tagged Codex and
// their console hosts. Shells and arbitrary helper descendants stay watched.
func WindowsGOAProcesses(agentID, sessionID string, list []procstat.Proc, args func(int) []string,
	image func(int) string, tagged func(int) bool, localAppData, systemRoot string) map[int]bool {
	out := map[int]bool{}
	byPID := map[int]procstat.Proc{}
	for _, p := range list {
		byPID[p.PID] = p
	}
	if agentID != "codex" || sessionID == "" {
		return out
	}
	parentOf := func(p procstat.Proc) (procstat.Proc, bool) {
		parent, ok := byPID[p.PPID]
		// Windows start times are decimal FILETIMEs. Compare numeric length
		// first so a wrapped/reused PID cannot supply a younger parent.
		older := len(parent.Start) < len(p.Start) || len(parent.Start) == len(p.Start) && parent.Start <= p.Start
		return parent, ok && p.Start != "" && parent.Start != "" && older && parent.PID != p.PID
	}
	for _, p := range list {
		if !strings.EqualFold(p.Name, "codex.exe") || !tagged(p.PID) {
			continue
		}
		parent, ok := parentOf(p)
		if !ok || !strings.EqualFold(filepath.Base(image(parent.PID)), "node.exe") || !tagged(parent.PID) {
			continue
		}
		argv := args(parent.PID)
		if len(argv) < 2 {
			continue
		}
		script := filepath.Clean(filepath.FromSlash(argv[1]))
		if !strings.EqualFold(filepath.Base(script), "codex.js") || !strings.EqualFold(filepath.Base(filepath.Dir(script)), "bin") {
			continue
		}
		pkg := filepath.Dir(filepath.Dir(script))
		if !strings.EqualFold(filepath.Base(pkg), "codex") || !strings.EqualFold(filepath.Base(filepath.Dir(pkg)), "@openai") {
			continue
		}
		// Resolve an npm junction before comparing the native binary's path.
		if real, err := filepath.EvalSymlinks(pkg); err == nil {
			pkg = real
		}
		rel, err := filepath.Rel(pkg, image(p.PID))
		out[parent.PID] = err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
	}
	for _, p := range list {
		name := strings.ToLower(p.Name)
		if name != "codex-code-mode-host.exe" && name != "node.exe" && name != "node_repl.exe" {
			continue
		}
		if !tagged(p.PID) {
			continue
		}
		parent, ok := parentOf(p)
		if !ok || !strings.EqualFold(filepath.Base(image(parent.PID)), "codex.exe") || !tagged(parent.PID) {
			continue
		}
		path, argv := image(p.PID), args(p.PID)
		if len(argv) == 0 {
			continue
		}
		name = strings.ToLower(filepath.Base(path))
		switch name {
		case "codex-code-mode-host.exe":
			out[p.PID] = len(argv) == 1 && strings.EqualFold(filepath.Dir(path), filepath.Dir(image(parent.PID)))
		case "node.exe", "node_repl.exe":
			root := filepath.Join(localAppData, "OpenAI", "Codex", "runtimes", "cua_node")
			rel, err := filepath.Rel(root, path)
			parts := strings.Split(rel, string(filepath.Separator))
			if localAppData == "" || err != nil || len(parts) != 3 || parts[0] == ".." || !strings.EqualFold(parts[1], "bin") {
				continue
			}
			if name == "node_repl.exe" {
				out[p.PID] = len(argv) == 1
			} else {
				// The CUA entry point is fixed; node -e and arbitrary scripts
				// from the same runtime are still activity worth flagging.
				entry := filepath.Join(filepath.Dir(path), "node_modules", "@oai", "cua-repl", "bin", "cua-repl.mjs")
				out[p.PID] = (len(argv) == 2 || len(argv) == 4 && argv[1] == "--import" && cuaAccessHook(argv[2], localAppData)) &&
					strings.EqualFold(argv[len(argv)-1], entry)
			}
		}
	}
	helperParents := maps.Clone(out)
	for _, p := range list {
		if !strings.EqualFold(p.Name, "conhost.exe") {
			continue
		}
		parent, ok := parentOf(p)
		if !ok || !helperParents[parent.PID] || !tagged(p.PID) || systemRoot == "" ||
			!strings.EqualFold(image(p.PID), filepath.Join(systemRoot, "System32", "conhost.exe")) {
			continue
		}
		argv := args(p.PID)
		out[p.PID] = len(argv) == 2 && strings.EqualFold(argv[1], "0x4")
	}
	return out
}

func cuaAccessHook(rawURL, localAppData string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "file" || u.Host != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	path := filepath.FromSlash(strings.TrimPrefix(u.Path, "/"))
	root := filepath.Join(filepath.Dir(filepath.Dir(localAppData)), ".codex", "plugins", "cache", "openai-bundled", "unified-computer-use")
	rel, err := filepath.Rel(root, path)
	parts := strings.Split(rel, string(filepath.Separator))
	return err == nil && len(parts) == 2 && parts[0] != ".." && parts[0] != "." &&
		strings.EqualFold(parts[1], "require-browser-access.mjs")
}
