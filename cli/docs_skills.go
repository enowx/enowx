package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

const docsSkillsUsage = "usage: enowx-app docs skills [install [--agent AGENT] [--force]]"

func init() {
	docsCommands["skill"] = docsCommand{
		usage: "skill [NAME]                how to work on Project Docs: the list, or one of them",
		run:   runner.docsSkill,
	}
	docsCommands["skills"] = docsCommand{
		usage: "skills install [--agent A] [--force]\n                              those skills as files of agent A's skills folder (claude, codex, shared, opencode)",
		run:   runner.docsSkills,
	}
}

// docsSkill prints the skills, or one of them.
func (r runner) docsSkill(args []string) error {
	if len(args) == 0 {
		r.printSkills()
		return nil
	}
	s, ok := projectdocs.FindSkill(args[0])
	if !ok {
		return fmt.Errorf("no skill %q: %s", args[0], strings.Join(skillNames(), ", "))
	}
	fmt.Fprint(r.out, s.Text)
	return nil
}

func (r runner) printSkills() {
	for _, s := range projectdocs.Skills() {
		fmt.Fprintf(r.out, "%s: %s\n", s.Name, s.Description)
	}
}

func skillNames() []string {
	var names []string
	for _, s := range projectdocs.Skills() {
		names = append(names, s.Name)
	}
	return names
}

// docsSkills installs the skills, only when asked to: docs skill serves them
// without touching the user's folders.
func (r runner) docsSkills(args []string) error {
	if len(args) == 0 {
		r.printSkills()
		return nil
	}
	if args[0] != "install" {
		return errors.New(docsSkillsUsage)
	}
	fs := flagSet("docs skills install")
	who := fs.String("agent", "claude", "")
	force := fs.Bool("force", false, "")
	if words, err := parseAll(fs, args[1:]); err != nil || len(words) > 0 {
		return errors.New(docsSkillsUsage)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("docs skills: %w", err)
	}
	kept := 0
	for _, s := range projectdocs.Skills() {
		state, path, err := installSkill(home, *who, s, *force)
		if err != nil {
			return err
		}
		fmt.Fprintf(r.out, "%s: %s\n", state, path)
		if state == skillKept {
			kept++
		}
	}
	if kept > 0 {
		return fmt.Errorf("%d changed by hand and kept: --force replaces them", kept)
	}
	return nil
}

// What installing a skill came to.
const (
	skillWritten = "wrote"
	skillSame    = "already there"
	skillKept    = "kept, changed by hand"
)

// skillAgent is the name envsync's layout table has agent's user skills under.
func skillAgent(agent string) (string, error) {
	if agent == "codex" {
		agent = "shared" // Codex reads the folder it shares with OpenCode
	}
	agents := envsync.Agents(proto.EnvSkill)
	if !slices.Contains(agents, agent) {
		return "", fmt.Errorf("no skills folder for agent %q: %s, codex", agent, strings.Join(agents, ", "))
	}
	return agent, nil
}

// installSkill writes s where agent's skills live under home and says what it
// came to and where. Only SKILL.md is written: anything the user put beside it
// stays, which envsync's Import, replacing the folder, wouldn't keep.
func installSkill(home, agent string, s projectdocs.Skill, force bool) (state, path string, err error) {
	agent, err = skillAgent(agent)
	if err != nil {
		return "", "", err
	}
	text := stampedSkill(s)
	p, err := envsync.NewSkill(agent, s.Name, []proto.EnvFile{{Path: "SKILL.md", Data: []byte(text)}})
	if err != nil {
		return "", "", fmt.Errorf("docs skills: %w", err)
	}
	path = filepath.Join(home, filepath.FromSlash(p.Files[0].Path))
	old, err := os.ReadFile(path)
	switch {
	case err == nil && string(old) == text:
		return skillSame, path, nil
	case err == nil && !force && !skillUntouched(string(old)):
		return skillKept, path, nil
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return "", "", fmt.Errorf("docs skills: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", fmt.Errorf("docs skills: %w", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", "", fmt.Errorf("docs skills: %w", err)
	}
	return skillWritten, path, nil
}

// An installed skill ends with a hash of its text. Without it a skill of an
// earlier version couldn't be told from one the user edited, and either every
// update would ask for --force or an edit would be lost.
var skillStampRe = regexp.MustCompile(`(?s)^(.*)\n<!-- enowX Project Docs skill @([0-9a-f]{12}): changed by hand, it is kept unless installed with --force -->\n$`)

func stampedSkill(s projectdocs.Skill) string {
	return fmt.Sprintf("%s\n<!-- enowX Project Docs skill @%s: changed by hand, it is kept unless installed with --force -->\n", s.Text, skillHash(s.Text))
}

func skillHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:12]
}

// skillUntouched says an installed file is as enowX wrote it, of any version.
func skillUntouched(file string) bool {
	m := skillStampRe.FindStringSubmatch(file)
	return m != nil && skillHash(m[1]) == m[2]
}
