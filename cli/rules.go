package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/enowx/enowx/proto"
)

const rulesUsage = `usage: enowx-app project rules [P] [--set TEXT|-] [--editors members|admins]
  a project's rules, which every session in it reads first (P: its folder, "." or none for this one);
  --set replaces them ("-" reads stdin, "" clears them); --editors says who edits a team's project's (its admins)`

// projectRules shows a project's rules, or changes them.
func (r runner) projectRules(args []string) error {
	fs := flag.NewFlagSet("rules", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	set := fs.String("set", "", "")
	editors := fs.String("editors", "", "")
	arg, err := parseWithArg(fs, args)
	if err != nil {
		return errors.New(rulesUsage)
	}
	if arg == "" || arg == "." {
		arg, _ = os.Getwd()
	}
	path := r.hostPath(arg)
	if !isSet(fs, "set") && *editors == "" {
		rules, err := r.c.ProjectRules(path)
		if err != nil {
			return err
		}
		whose := "your own"
		if rules.Team != "" {
			whose = "the team's, edited by its " + rules.Editors
		}
		fmt.Fprintf(r.out, "# Rules of %s (%s)\n\n", filepath.Base(path), whose)
		if rules.Rules == "" {
			fmt.Fprintln(r.out, "None yet: \"enowx-app project rules --set TEXT\" writes them.")
			return nil
		}
		fmt.Fprintln(r.out, rules.Rules)
		return nil
	}
	cur, err := r.c.ProjectRules(path)
	if err != nil {
		return err
	}
	next := proto.ProjectRules{Path: path, Rules: cur.Rules, Editors: *editors}
	if isSet(fs, "set") {
		next.Rules = *set
		if *set == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			next.Rules = string(data)
		}
	}
	_, err = r.c.SaveProjectRules(next)
	return err
}

// rulesTools read and change a project's rules, as MCP tools.
var rulesTools = []mcpTool{
	{name: "project_rules", description: "A project's rules, which every session in it reads first: the user's own for their projects, the team's for a team's project. The session's own project when none is named.",
		props: map[string]mcpProp{"project": {"string", "The project's folder (default: this session's)"}},
		args:  func(a map[string]any) []string { return []string{"rules", str(a, "project")} }},
	{name: "project_rules_set", description: "Replace a project's rules (Markdown; empty clears them), only when the user asked: they go into every session in it. A team's project's rules are edited by its members or only its admins, as the admins say; rules holding a secret are refused.",
		props: map[string]mcpProp{"project": {"string", "The project's folder (default: this session's)"}, "rules": {"string", "The rules, whole"},
			"editors": {"string", "Who edits a team's project's rules: members or admins (its admins)"}},
		required: []string{"rules"},
		args: func(a map[string]any) []string {
			return withFlag(append([]string{"rules", str(a, "project"), "--set"}, str(a, "rules")), a, "editors", "editors")
		}},
}
