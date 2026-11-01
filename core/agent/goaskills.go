package agent

import (
	"embed"
	"fmt"
	"slices"
	"strings"
)

// GOA's built-in skills: longer guides it reads when it needs them, so its
// prompt stays short. They ship inside the program, on every host and for any
// agent, as commands' output: no agent's own skill system is needed.

//go:embed goaskills/*.md
var goaSkillFiles embed.FS

// GOASkillNames are the skills, in the order they're offered.
func GOASkillNames() []string { return []string{"setup", "features"} }

// GOASkill is skill name's text.
func GOASkill(name string) (string, error) {
	if name == "features" {
		return goaFeaturesSkill(), nil
	}
	if !slices.Contains(GOASkillNames(), name) {
		return "", fmt.Errorf("no skill %q: %s", name, strings.Join(GOASkillNames(), ", "))
	}
	data, err := goaSkillFiles.ReadFile("goaskills/" + name + ".md")
	return string(data), err
}

// goaFeaturesSkill lists every feature from the registry itself, so it can't
// fall behind: what each does, and where its how-to is.
func goaFeaturesSkill() string {
	var b strings.Builder
	b.WriteString("# enowX's features\n\nWhat each does; `\"$ENOWX_BIN\" help ID` prints how to use it, on or off, with its commands; `\"$ENOWX_BIN\" goa doc` lists the docs on how each works. Answer from those, not from memory; say what you can't confirm.\n\n")
	for _, f := range Features {
		fmt.Fprintf(&b, "- **%s** (`%s`, %s): %s\n", f.Name, f.ID, f.Group, f.About)
	}
	return b.String()
}
