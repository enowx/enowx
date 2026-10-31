package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/enowx/enowx/proto"
)

const projectUsage = `usage: enowx-app project COMMAND
  list                             the host's projects
  add PATH                         make a folder of the host a project
  clone URL [--dir D] [--name N] [--branch B] [--depth N] [--submodules] [--no-wait]
                                   clone a repository into D/N (D: here; N: the repository's),
                                   as a new project; waits, showing git's progress, unless --no-wait
  status ID [--wait]               how a clone goes; --wait until it ends
  cancel ID                        stop a clone, its folder removed
  context P [--host H]             a project's summary, of any host, read-only: stack, branch, folders, README
  read P FILE [--host H]           a project's file, of any host, read-only
  rules [P] [--set TEXT|-] [--editors members|admins]
                                   a project's rules, which every session in it reads first
  settings [P] [--copy A,B] [--setup SCRIPT]
                                   its .enowx.toml: what new worktrees get, and their setup
list, add, clone, status, cancel, rules and settings take --host H: another host's, its paths whole
` + projectLinksUsage

// project lists, adds and clones the host's projects.
func (r runner) projectCmd(args []string) error {
	if len(args) == 0 {
		return errors.New(projectUsage)
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "copy", "links", "origin", "sync", "conflicts", "autopush", "history":
		return r.projectLinkCmd(cmd, rest)
	case "context", "read":
		return r.projectContext(cmd, rest)
	case "rules":
		return r.projectRules(rest)
	case "settings":
		return r.projectSettings(rest)
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", "", "")
	name := fs.String("name", "", "")
	branch := fs.String("branch", "", "")
	depth := fs.Int("depth", 0, "")
	submodules := fs.Bool("submodules", false, "")
	noWait := fs.Bool("no-wait", false, "")
	wait := fs.Bool("wait", false, "")
	arg, err := parseWithArg(fs, rest)
	if err != nil {
		return errors.New(projectUsage)
	}
	switch cmd {
	case "help":
		fmt.Fprintln(r.out, projectUsage)
		return nil
	case "list", "ls":
		return r.projects(nil)
	case "add":
		if arg == "" {
			return errors.New("usage: enowx-app project add PATH")
		}
		info, err := r.c.AddProject(r.hostPath(arg))
		if err != nil {
			return err
		}
		fmt.Fprintln(r.out, info.Path)
		return nil
	case "clone":
		if arg == "" {
			return errors.New(projectUsage)
		}
		if *dir == "" {
			// The folder cloned into last here, or the user's usual one.
			if prefs, err := r.c.ClonePrefs(); err == nil && prefs.Dir != "" {
				*dir = prefs.Dir
			} else if err == nil && prefs.Default != "" {
				*dir = prefs.Default
			}
		}
		if *dir == "" {
			*dir, _ = os.Getwd()
		}
		st, err := r.c.CloneProject(proto.ProjectCloneParams{URL: arg, Dir: *dir, Name: *name, Branch: *branch, Depth: *depth, Submodules: *submodules})
		if err != nil {
			return err
		}
		if *noWait {
			fmt.Fprintf(r.out, "Cloning into %s: \"enowx-app project status %s --wait\" follows it.\n", st.Path, st.ID)
			return nil
		}
		return r.followClone(st.ID)
	case "status":
		if arg == "" {
			return errors.New("usage: enowx-app project status ID [--wait]")
		}
		if *wait {
			return r.followClone(arg)
		}
		st, err := r.c.CloneStatus(arg)
		if err != nil {
			return err
		}
		return cloneResult(r.out, st, true)
	case "cancel":
		if arg == "" {
			return errors.New("usage: enowx-app project cancel ID")
		}
		return r.c.CancelClone(arg)
	}
	return errors.New(projectUsage)
}

// followClone shows clone id's progress until it ends.
func (r runner) followClone(id string) error {
	last := ""
	for {
		st, err := r.c.CloneStatus(id)
		if err != nil {
			return err
		}
		if st.State != proto.CloneRunning {
			return cloneResult(r.out, st, false)
		}
		if line := fmt.Sprintf("%s %d%%", st.Phase, st.Percent); line != last {
			fmt.Fprintln(r.out, line)
			last = line
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// cloneResult says how a clone ended, an error for one that failed.
func cloneResult(w io.Writer, st proto.CloneStatus, running bool) error {
	switch st.State {
	case proto.CloneDone:
		fmt.Fprintf(w, "Cloned %s into %s, now a project.\n", st.URL, st.Path)
	case proto.CloneFailed:
		return errors.New(st.Error)
	case proto.CloneCanceled:
		return fmt.Errorf("the clone of %s was cancelled", st.URL)
	default:
		if running {
			fmt.Fprintf(w, "Cloning %s into %s: %s %d%%\n", st.URL, st.Path, st.Phase, st.Percent)
		}
	}
	return nil
}

// cloneTools are the clones and projects as MCP tools.
var cloneTools = []mcpTool{
	{name: "projects_list", description: "The host's projects: their folders, and groups.",
		args: func(map[string]any) []string { return []string{"list"} }},
	{name: "project_add", description: "Make a folder of the host one of its projects in enowX.",
		props: map[string]mcpProp{"path": {"string", "The folder's absolute path"}}, required: []string{"path"},
		args: func(a map[string]any) []string { return []string{"add", str(a, "path")} }},
	{name: "project_clone", description: "Clone a Git repository (https or SSH address) into a new folder of the host, with its git and credentials, as a new project. Answers at once with the clone's ID; project_clone_status follows it.",
		props: map[string]mcpProp{"url": {"string", "The repository's address"}, "dir": {"string", "The folder to clone into (~ for home); the one cloned into last by default"},
			"name": {"string", "The new folder's name; the repository's by default"}, "branch": {"string", "A branch or tag"},
			"depth": {"number", "Commits of history to take; 1 for a shallow clone"}, "submodules": {"boolean", "Clone its submodules too"}},
		required: []string{"url"},
		args: func(a map[string]any) []string {
			args := []string{"clone", str(a, "url"), "--no-wait"}
			for _, k := range []string{"dir", "name", "branch", "depth", "submodules"} {
				args = withFlag(args, a, k, k)
			}
			return args
		}},
	{name: "project_clone_status", description: "How a clone goes, or with wait, how it ended: its folder, or why it failed (an address that's wrong, a repository the host can't sign in to) and what to do.",
		props: map[string]mcpProp{"id": {"string", "The clone's ID"}, "wait": {"boolean", "Wait until it ends"}}, required: []string{"id"},
		args: func(a map[string]any) []string { return withFlag([]string{"status", str(a, "id")}, a, "wait", "wait") }},
}
