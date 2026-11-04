package projectdocs

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

// Skill is a guide an agent follows to work on Project Docs, as a SKILL.md
// any agent's skills folder takes.
type Skill struct {
	Name, Description string
	Text              string // the whole of it, front matter first
}

//go:embed skills/*.md
var skillFiles embed.FS

// Every skill may be installed alone, so each carries the rules whole rather
// than pointing at a file that may not be beside it.
//
//go:embed skills/rules.txt
var skillRules string

// Skills is every skill, by name.
func Skills() []Skill {
	entries, err := fs.ReadDir(skillFiles, "skills")
	if err != nil {
		return nil
	}
	out := make([]Skill, 0, len(entries))
	for _, e := range entries {
		data, err := skillFiles.ReadFile("skills/" + e.Name())
		if err != nil {
			continue
		}
		name, desc := skillFront(string(data))
		out = append(out, Skill{Name: name, Description: desc,
			Text: strings.TrimRight(string(data), "\n") + "\n\n" + skillRules})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// FindSkill is the skill called name, false for none.
func FindSkill(name string) (Skill, bool) {
	for _, s := range Skills() {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return Skill{}, false
}

// skillFront reads the name and description of a skill's front matter.
func skillFront(text string) (name, description string) {
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return "", ""
	}
	front, _, _ := strings.Cut(rest, "\n---")
	for _, line := range strings.Split(front, "\n") {
		key, value, _ := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		switch key {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return name, description
}
