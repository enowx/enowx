package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/enowx/enowx/internal/quietcmd"
	"github.com/enowx/enowx/proto"
)

// Commands lists the slash commands the agent offers in dir: its own,
// the user's and the project's commands and skills, and its plugins'.
// It returns nil for agents enowX can't ask.
func Commands(ctx context.Context, id, dir string) ([]proto.AgentCommand, error) {
	id = Base(id)
	switch id {
	case "claude":
		return claudeCommands(ctx, dir)
	case "codex":
		return codexCommands(), nil
	case PlatformID:
		return platformCommands(), nil
	}
	return nil, nil
}

// claudeCommands asks Claude Code for its commands the way its SDK learns
// them: the init record of a non-interactive run. The run is a local
// command (/cost), so it reaches no model; it runs no hooks, starts no MCP
// server (whose prompts are left out) and saves no session.
func claudeCommands(ctx context.Context, dir string) ([]proto.AgentCommand, error) {
	cmd := quietcmd.CommandContext(ctx, "claude", "-p", "--strict-mcp-config", "--no-session-persistence",
		"--settings", `{"disableAllHooks":true}`, "--output-format", "stream-json", "--verbose", "/cost")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	var init struct {
		Type     string   `json:"type"`
		Subtype  string   `json:"subtype"`
		Commands []string `json:"slash_commands"`
		Skills   []string `json:"skills"`
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		if json.Unmarshal(sc.Bytes(), &init) == nil && init.Type == "system" && init.Subtype == "init" {
			break
		}
		init.Commands = nil
	}
	if init.Commands == nil {
		return nil, nil
	}
	notes := claudeNotes(dir)
	list := make([]proto.AgentCommand, 0, len(init.Commands))
	for _, name := range init.Commands {
		if strings.HasPrefix(name, "__") {
			continue // internal
		}
		c := proto.AgentCommand{Name: name, Description: notes[name], Kind: "command"}
		switch {
		case slices.Contains(init.Skills, name):
			c.Kind = "skill"
		case notes[name] == "" && !strings.Contains(name, ":"):
			c.Kind = "built-in"
		}
		list = append(list, c)
	}
	return list, nil
}

// claudeNotes are the descriptions of the commands and skills on disk:
// the user's, the project's and the plugins', by the name Claude Code
// lists them under ("plugin:name" for a plugin's).
func claudeNotes(dir string) map[string]string {
	notes := map[string]string{}
	home, _ := os.UserHomeDir()
	for _, root := range []string{filepath.Join(home, ".claude"), filepath.Join(dir, ".claude")} {
		readNotes(filepath.Join(root, "skills"), "", notes)
		readNotes(filepath.Join(root, "commands"), "", notes)
	}
	// Plugins keep skills and commands under their own folder, at depths
	// that vary by marketplace.
	plugins := filepath.Join(home, ".claude", "plugins")
	_ = filepath.WalkDir(plugins, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.Count(strings.TrimPrefix(p, plugins), string(filepath.Separator)) > 6 {
			return filepath.SkipDir
		}
		if d.IsDir() && (d.Name() == "skills" || d.Name() == "commands") {
			plugin := filepath.Base(filepath.Dir(p))
			if v := filepath.Base(filepath.Dir(p)); looksLikeVersion(v) {
				plugin = filepath.Base(filepath.Dir(filepath.Dir(p)))
			}
			readNotes(p, plugin+":", notes)
			return filepath.SkipDir
		}
		return nil
	})
	return notes
}

func looksLikeVersion(s string) bool {
	return s != "" && (s[0] >= '0' && s[0] <= '9' || s[0] == 'v' && len(s) > 1 && s[1] >= '0' && s[1] <= '9')
}

// readNotes reads the description of each skill (folder/SKILL.md) and
// command (name.md) in folder into notes, under prefix+name, unless one
// is there already.
func readNotes(folder, prefix string, notes map[string]string) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return
	}
	for _, e := range entries {
		name, path := e.Name(), filepath.Join(folder, e.Name())
		if e.IsDir() {
			path = filepath.Join(path, "SKILL.md")
		} else if strings.HasSuffix(name, ".md") {
			name = strings.TrimSuffix(name, ".md")
		} else {
			continue
		}
		if _, ok := notes[prefix+name]; ok {
			continue
		}
		if d := frontDescription(path); d != "" {
			notes[prefix+name] = d
		}
	}
}

// frontDescription is the description in a Markdown file's front matter,
// folded onto one line and cut short.
func frontDescription(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return ""
	}
	var desc []string
	in := false
	for i := 0; sc.Scan() && i < 60; i++ {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		if v, ok := strings.CutPrefix(line, "description:"); ok {
			in = true
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			if v != ">" && v != "|" && v != ">-" && v != "|-" && v != "" {
				desc = append(desc, v)
			}
			continue
		}
		if in && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			desc = append(desc, strings.TrimSpace(line))
			continue
		}
		in = false
	}
	s := strings.Join(desc, " ")
	if r := []rune(s); len(r) > 160 {
		s = string(r[:157]) + "…"
	}
	return s
}
