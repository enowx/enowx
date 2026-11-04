package agent

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Guide tells an agent what enowX gives it beyond a terminal, as a skill
// built in: the user sees its dev servers and desktop apps from enowX,
// wherever the session runs. It is every feature's, in full; a session
// gets GuideFor the features the user has on.
var Guide = GuideFor(AllFeatures(), false)

// DefaultGuide is the guide of the features on by default.
var DefaultGuide = GuideFor(DefaultFeatures(), true)

// GuideArgs returns the arguments that give agent id the Guide, or nil
// when its command line takes none (PlaceGuide then puts it where the
// agent reads it). They add to the user's own instructions, never
// replace them; docs/agents.md says where each comes from.
func GuideArgs(id string) []string { return guideArgsWith(id, Guide, GuideFile) }

// GuideArgsText is GuideArgs with guide text: the guide of the features
// the user has on.
func GuideArgsText(id, text string) []string { return guideArgsWith(id, text, GuideFile) }

// GuideArgsIn is GuideArgsText for a session whose agent reads the file at path.
func GuideArgsIn(id, text, path string) []string {
	return guideArgsWith(id, text, func() string { return path })
}

// guideArgsWith gives agent id text as GuideArgs gives the Guide; file is
// where the agents that take a file read it.
func guideArgsWith(id, text string, file func() string) []string {
	switch id {
	case "claude", PlatformID, "claude-agent-teams", "openclaude", "qoder", "qoder-cn", "omp", "prime-agent", "droid", "qwen-code", "codebuddy":
		return []string{"--append-system-prompt", text}
	case "codex":
		if own := codexInstructions(); own != "" {
			text = own + "\n\n" + text // -c replaces the user's value
		}
		return []string{"-c", "developer_instructions=" + marshal(text)}
	case "autohand":
		return []string{"--append-sys-prompt", text}
	case "grok":
		return []string{"--rules", text}
	case "continue":
		return []string{"--rule", text}
	case "aider":
		return []string{"--read", file()}
	case "aug":
		return []string{"--rules", file()}
	}
	return nil
}

// GuideFile is the Guide as a file, for agents that take a file of
// instructions; PlaceGuide writes it.
func GuideFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".enowx-app", "agent-guide.md")
}

// codexInstructions is developer_instructions of the user's Codex
// configuration.
func codexInstructions() string {
	var cfg struct {
		Instructions string `toml:"developer_instructions"`
	}
	if _, err := toml.DecodeFile(filepath.Join(codexHome(), "config.toml"), &cfg); err != nil {
		return ""
	}
	return cfg.Instructions
}

// codexHome is where Codex keeps its configuration.
func codexHome() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".codex")
}
