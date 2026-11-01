package cli

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/agent"
)

var namedCommand = regexp.MustCompile(`\$ENOWX_BIN"\s+([a-z][a-z-]*)(?:\s+([a-z][a-z-]*))?`)

// Every command GOA's prompt and skills name exists, down to the words of those
// that take them, so what it's told can't fall behind what there is.
func TestGOAsTextNamesCommandsThatExist(t *testing.T) {
	sub := map[string]func() string{
		"goa": func() string { return goaUsage }, "rag": func() string { return ragUsage }, "decide": func() string { return decideUsage },
		"configs": func() string { return configsUsage },
	}
	check := func(where, text string) {
		for _, m := range namedCommand.FindAllStringSubmatch(text, -1) {
			if !slices.Contains(Commands, m[1]) {
				t.Errorf("%s names %q, which isn't a command", where, m[1])
				continue
			}
			if usage, ok := sub[m[1]]; ok && m[2] != "" && !strings.Contains(usage(), m[2]) {
				t.Errorf("%s names %q %q, which its usage doesn't have", where, m[1], m[2])
			}
		}
	}
	check("the prompt", agent.GOAGuide)
	for _, name := range agent.GOASkillNames() {
		text, err := agent.GOASkill(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(text) == "" {
			t.Fatalf("%s is empty", name)
		}
		check(name, text)
	}
	for _, f := range agent.Features {
		features, _ := agent.GOASkill("features")
		if !strings.Contains(features, f.ID) {
			t.Errorf("the features skill misses %s", f.ID)
		}
	}
	if !strings.HasPrefix(agent.GOAGuide, "You are GOD of Agents") || !strings.Contains(agent.GOAGuide, "Your job is handling every other session") {
		t.Fatal("the prompt doesn't start with who it is and its job")
	}
	// It goes into every turn: what's long lives in skills.
	if n := len(agent.GOAGuide); n > 8000 {
		t.Errorf("the prompt is %d bytes", n)
	}
	t.Logf("the prompt is %d bytes", len(agent.GOAGuide))
}
