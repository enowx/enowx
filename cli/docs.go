package cli

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// docsCommand is a subcommand of "docs": its usage line and what runs it.
type docsCommand struct {
	usage string
	run   func(r runner, args []string) error
}

// docsCommands is filled by the files beside this one, each registering its own.
var docsCommands = map[string]docsCommand{}

func docsUsage() string {
	names := make([]string, 0, len(docsCommands))
	for name := range docsCommands {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("usage: enowx-app docs COMMAND\nA project documented as notes: one project note, its documents under it (see docs/project-docs.md).\n")
	for _, name := range names {
		fmt.Fprintf(&b, "  %s\n", docsCommands[name].usage)
	}
	return strings.TrimRight(b.String(), "\n")
}

// docs is Project Docs' command line.
func (r runner) docs(args []string) error {
	if len(args) == 0 || slices.Contains([]string{"help", "--help", "-h"}, args[0]) {
		fmt.Fprintln(r.out, docsUsage())
		return nil
	}
	cmd, ok := docsCommands[args[0]]
	if !ok {
		return errors.New(docsUsage())
	}
	return cmd.run(r, args[1:])
}
