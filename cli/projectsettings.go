package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/enowx/enowx/proto"
)

const projectSettingsUsage = `usage: enowx-app project settings [P] [--copy A,B] [--setup SCRIPT] [--host H]
  a project's settings (.enowx.toml, committed with it): the files git ignores that new
  worktrees get from its folder (--copy, "" none), and the script that readies one (--setup, "" none)`

// projectSettings shows a project's settings, or changes them.
func (r runner) projectSettings(args []string) error {
	fs := flag.NewFlagSet("settings", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	copies := fs.String("copy", "", "")
	setup := fs.String("setup", "", "")
	arg, err := parseWithArg(fs, args)
	if err != nil {
		return errors.New(projectSettingsUsage)
	}
	if arg == "" || arg == "." {
		if r.host != "" {
			return errors.New("project settings --host: give the project's folder on that host")
		}
		arg, _ = os.Getwd()
	}
	path := r.hostPath(arg)
	cfg, err := r.c.ProjectConfig(path)
	if err != nil {
		return err
	}
	if isSet(fs, "copy") || isSet(fs, "setup") {
		if isSet(fs, "copy") {
			cfg.Copy = nil
			for _, c := range strings.Split(*copies, ",") {
				if c = strings.TrimSpace(c); c != "" {
					cfg.Copy = append(cfg.Copy, c)
				}
			}
		}
		if isSet(fs, "setup") {
			cfg.Setup = *setup
		}
		if cfg, err = r.c.SaveProjectConfig(cfg); err != nil {
			return err
		}
	}
	printProjectConfig(r, cfg)
	return nil
}

func printProjectConfig(r runner, cfg proto.ProjectConfig) {
	fmt.Fprintf(r.out, "# Settings of %s (.enowx.toml)\n", cfg.Path)
	fmt.Fprintf(r.out, "copy: %s\n", nonEmpty(strings.Join(cfg.Copy, ", "), "(nothing)"))
	fmt.Fprintf(r.out, "setup: %s\n", nonEmpty(cfg.Setup, "(none)"))
}
