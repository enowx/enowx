package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// What an agent is told and given runs to tens of thousands of characters, which a
// Windows command line can't hold: a program installed by npm is a .cmd file there,
// and cmd.exe stops at 8,191. So they go in files of the session's own folder, for
// the agents whose documentation gives a file form (docs/agents.md says which).

// fileForm is how an agent takes in a file what flag takes inline.
type fileForm struct {
	flag string
	to   string // the flag of the file's path
	name string // the file, in the session's folder
	// joined: one argument, --flag=value, its path after at.
	joined bool
	at     string
	// json: a value that isn't JSON is a path already.
	json bool
}

var claudeFileForms = []fileForm{
	{flag: "--append-system-prompt", to: "--append-system-prompt-file", name: "system-prompt.md"},
	{flag: "--mcp-config", to: "--mcp-config", name: "mcp.json", json: true},
	{flag: "--settings", to: "--settings", name: "settings.json", json: true},
}

// fileForms are the agents' file forms, by agent.
var fileForms = map[string][]fileForm{
	"claude":             claudeFileForms,
	PlatformID:           platformFileForms,
	"claude-agent-teams": claudeFileForms,
	"openclaude":         claudeFileForms,
	"droid":              {{flag: "--append-system-prompt", to: "--append-system-prompt-file", name: "system-prompt.md"}},
	"autohand":           {{flag: "--append-sys-prompt", to: "--append-sys-prompt", name: "system-prompt.md"}},
	"continue":           {{flag: "--rule", to: "--rule", name: "system-prompt.md"}},
	"kimi":               {{flag: "--mcp-config", to: "--mcp-config-file", name: "mcp.json", json: true}},
	"copilot":            {{flag: "--additional-mcp-config", to: "--additional-mcp-config", name: "mcp.json", joined: true, at: "@", json: true}},
}

// MoveToFiles is args of agent id with what its file forms take written to files of
// dir, readable only by the user, and named instead. The agent gets the same text and
// tools; args without a file form stay as they are.
func MoveToFiles(id, dir string, args []string) ([]string, error) {
	forms := fileForms[id]
	if dir == "" || len(forms) == 0 {
		return args, nil
	}
	out := make([]string, 0, len(args))
	used := map[string]int{}
	place := func(f fileForm, value string) (string, error) {
		used[f.name]++
		name := f.name
		if n := used[f.name]; n > 1 {
			ext := filepath.Ext(name)
			name = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), n, ext)
		}
		return writeSessionFile(dir, name, value)
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		form, value, found := fileForm{}, "", false
		for _, f := range forms {
			switch {
			case f.joined && strings.HasPrefix(arg, f.flag+"="):
				form, value, found = f, strings.TrimPrefix(arg, f.flag+"="), true
			case !f.joined && arg == f.flag && i+1 < len(args):
				form, value, found = f, args[i+1], true
			}
			if found {
				break
			}
		}
		if !found || form.json && !strings.HasPrefix(strings.TrimSpace(value), "{") {
			out = append(out, arg)
			continue
		}
		path, err := place(form, value)
		if err != nil {
			return args, err
		}
		if form.joined {
			out = append(out, form.to+"="+form.at+path)
			continue
		}
		out = append(out, form.to, path)
		i++
	}
	return out, nil
}

// inlineGuides are the flags of the agents whose guide has no file form.
var inlineGuides = map[string]string{
	"qoder": "--append-system-prompt", "qoder-cn": "--append-system-prompt", "omp": "--append-system-prompt",
	"prime-agent": "--append-system-prompt", "qwen-code": "--append-system-prompt", "codebuddy": "--append-system-prompt",
	"grok": "--rules",
}

// codexGuide starts the argument of Codex's guide, in a -c.
const codexGuide = "developer_instructions="

// guidePointer starts the short text that stands for a guide in a file.
const guidePointer = "You are in an enowX session, which has instructions for you. "

// ShortenGuide is args of agent id with its inline guide written to a file of dir and
// a line naming the file instead, for a command line that can't hold it. The agent then
// has the guide only by reading the file, so it is for what wouldn't start otherwise.
// It says whether it did.
func ShortenGuide(id, dir string, args []string) ([]string, bool, error) {
	if dir == "" {
		return args, false, nil
	}
	out := append([]string(nil), args...)
	line := func(path string) string {
		return guidePointer + "They are in the file " + path + ": read all of it now, before anything else, and follow it."
	}
	if id == "codex" {
		for i, a := range out {
			if !strings.HasPrefix(a, codexGuide) || i == 0 || out[i-1] != "-c" {
				continue
			}
			var text string
			if err := json.Unmarshal([]byte(strings.TrimPrefix(a, codexGuide)), &text); err != nil {
				return args, false, err
			}
			path, err := writeSessionFile(dir, "system-prompt.md", text)
			if err != nil {
				return args, false, err
			}
			out[i] = codexGuide + marshal(line(path))
			return out, true, nil
		}
		return args, false, nil
	}
	flag := inlineGuides[id]
	for i := 0; flag != "" && i+1 < len(out); i++ {
		if out[i] != flag {
			continue
		}
		path, err := writeSessionFile(dir, "system-prompt.md", out[i+1])
		if err != nil {
			return args, false, err
		}
		out[i+1] = line(path)
		return out, true, nil
	}
	return args, false, nil
}

// writeSessionFile writes a fresh copy of a session's file, whole or not at all: an
// agent starting never reads half of one.
func writeSessionFile(dir, name, text string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("agent: session folder: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("agent: session folder: %w", err)
	}
	tmp, err := os.CreateTemp(dir, name+".*") // readable only by the user
	if err != nil {
		return "", fmt.Errorf("agent: session file: %w", err)
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.WriteString(text)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	path := filepath.Join(dir, name)
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		return "", fmt.Errorf("agent: session file %s: %w", name, err)
	}
	return path, nil
}

// GuideShortened says args have ShortenGuide's line in place of a guide.
func GuideShortened(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, guidePointer) || strings.HasPrefix(a, codexGuide+`"`+guidePointer) {
			return true
		}
	}
	return false
}
