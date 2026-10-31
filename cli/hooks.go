package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/enowx/enowx/proto"
)

// hooksUsage is how `enowx hooks` is used.
const hooksUsage = `usage: enowx-app hooks [list [--json] | add FLAGS | edit ID FLAGS | rm ID | on ID | off ID | test ID | log [--last N] | events]
FLAGS: --event E --do run|context|refuse|rewrite|judge|trim|notify|webhook|enowx --name N
  --command C | --context T | --reason R | --find RE --replace S | --url U --message M [--kind done|input|error]
  --agent A,B --project P --tool RE --files G,H --text RE --branch G --host H --any --not tool,paths
  --once --max N --every SECONDS --hours 09-18 --schedule "weekdays 09:00" --background --timeout SECONDS --off
  --scope host|project|session [--dir DIR] [--why TEXT]`

// hooks runs `enowx hooks …`. From a session (an agent), changes are
// proposed for the user to allow in the app.
func (r runner) hooks(args []string) error {
	if len(args) == 0 {
		return r.hooksList(false)
	}
	switch args[0] {
	case "list", "ls":
		return r.hooksList(slices.Contains(args[1:], "--json"))
	case "events":
		for _, e := range proto.HookEvents {
			fmt.Fprintln(r.out, e)
		}
		return nil
	case "log":
		return r.hooksLog(args[1:])
	case "add":
		return r.hooksChange("", args[1:])
	case "edit":
		if len(args) < 2 {
			return errors.New(hooksUsage)
		}
		return r.hooksChange(args[1], args[2:])
	case "rm", "remove", "on", "off", "test":
		if len(args) != 2 {
			return errors.New(hooksUsage)
		}
		found, err := r.findHook(args[1])
		if err != nil {
			return err
		}
		switch args[0] {
		case "test":
			run, err := r.c.TestHook(proto.HookTest{Hook: found.hook, Event: sampleEvent(found.hook.Event)})
			if err != nil {
				return err
			}
			return printRun(r.out, run)
		case "on", "off":
			found.hook.On = args[0] == "on"
			return r.applyHook(found, false, "")
		}
		return r.applyHook(found, true, "")
	}
	return errors.New(hooksUsage)
}

// foundHook is a hook and where it is kept.
type foundHook struct {
	hook             proto.Hook
	scope, dir, sess string
}

// allHooks are every hook the host keeps, with where.
func (r runner) allHooks(info proto.HooksInfo) []foundHook {
	var out []foundHook
	for _, h := range info.Hooks {
		if h.Project == "" && h.Session == "" {
			out = append(out, foundHook{hook: h, scope: proto.HookScopeHost})
		}
	}
	for _, ph := range info.Projects {
		for _, h := range ph.Hooks {
			out = append(out, foundHook{hook: h, scope: proto.HookScopeProject, dir: ph.Dir})
		}
	}
	for _, sh := range info.Sessions {
		for _, h := range sh.Hooks {
			out = append(out, foundHook{hook: h, scope: proto.HookScopeSession, sess: sh.Session})
		}
	}
	return out
}

func (r runner) findHook(id string) (foundHook, error) {
	info, err := r.c.Hooks(projectDirHere())
	if err != nil {
		return foundHook{}, err
	}
	for _, f := range r.allHooks(info) {
		if f.hook.ID == id || strings.EqualFold(f.hook.Name, id) {
			return f, nil
		}
	}
	return foundHook{}, fmt.Errorf("no hook %s (enowx-app hooks lists them)", id)
}

// projectDirHere is the folder this runs in: a session's, its project's.
func projectDirHere() string {
	wd, _ := os.Getwd()
	return wd
}

func (r runner) hooksList(asJSON bool) error {
	info, err := r.c.Hooks(projectDirHere())
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(r.out)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}
	w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tON\tWHERE\tEVENT\tDO\tNAME")
	for _, f := range r.allHooks(info) {
		where := f.scope
		switch f.scope {
		case proto.HookScopeProject:
			where = "project " + filepath.Base(f.dir)
		case proto.HookScopeSession:
			where = "session " + f.sess
		}
		on := "off"
		if f.hook.On {
			on = "on"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", f.hook.ID, on, where, f.hook.Event, f.hook.Action, f.hook.Name)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	for _, ph := range info.Pending {
		fmt.Fprintf(r.out, "\n%s has %d hooks of its own, waiting for the user to allow them in Settings → Hooks.\n", ph.Dir, len(ph.Hooks))
	}
	for _, p := range info.Proposals {
		what := "add " + p.Hook.Event + " → " + p.Hook.Action
		if p.Remove {
			what = "remove " + p.Hook.ID
		}
		fmt.Fprintf(r.out, "\nProposed by session %s (%s): %s. The user decides in Settings → Hooks.\n", p.By, p.ID, what)
	}
	return nil
}

func (r runner) hooksLog(args []string) error {
	fs := flag.NewFlagSet("hooks log", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	last := fs.Int("last", 20, "")
	if err := fs.Parse(args); err != nil {
		return errors.New("usage: enowx-app hooks log [--last N]")
	}
	runs, err := r.c.HookLog()
	if err != nil {
		return err
	}
	for i, run := range runs {
		if i >= *last {
			break
		}
		if err := printRun(r.out, run); err != nil {
			return err
		}
	}
	return nil
}

func printRun(out io.Writer, run proto.HookRun) error {
	result := "ok"
	switch {
	case run.Error != "":
		result = run.Error
	case run.Blocked:
		result = "refused"
	case run.Exit != 0:
		result = "exit " + strconv.Itoa(run.Exit)
	}
	_, err := fmt.Fprintf(out, "%s  %s  %s  %s  %d ms  %s\n", run.Start.Local().Format("15:04:05"), run.Event, nonEmptyOr(run.Name, run.Hook), result, run.Ms, firstLineOf(run.Output))
	return err
}

func nonEmptyOr(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

func firstLineOf(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

// sampleEvent is an event of kind event to test a hook on.
func sampleEvent(event string) proto.HookEvent {
	return proto.HookEvent{Event: event, Session: "test", Agent: "claude", Dir: projectDirHere(), Project: projectDirHere(),
		Prompt: "Fix the login redirect", Tool: "Bash", Command: "go test ./...", Paths: []string{"main.go"}, Time: time.Now()}
}

// hooksChange adds a hook (id "") or changes hook id from flags.
func (r runner) hooksChange(id string, args []string) error {
	var f foundHook
	if id != "" {
		var err error
		if f, err = r.findHook(id); err != nil {
			return err
		}
	} else {
		f = foundHook{hook: proto.Hook{On: true, Action: proto.HookActionRun}, scope: proto.HookScopeHost}
	}
	h := &f.hook
	fs := flag.NewFlagSet("hooks", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	str := func(name string, v *string) { fs.StringVar(v, name, *v, "") }
	list := func(name string, v *[]string) {
		fs.Func(name, "", func(s string) error {
			*v = nil
			for p := range strings.SplitSeq(s, ",") {
				if p = strings.TrimSpace(p); p != "" {
					*v = append(*v, p)
				}
			}
			return nil
		})
	}
	str("name", &h.Name)
	str("event", &h.Event)
	do := h.Action
	str("do", &do)
	str("command", &h.Command)
	str("context", &h.Context)
	str("reason", &h.Reason)
	str("find", &h.Find)
	str("replace", &h.Replace)
	str("url", &h.URL)
	str("message", &h.Message)
	str("kind", &h.Kind)
	list("agent", &h.Agents)
	list("project", &h.Projects)
	str("tool", &h.Tool)
	list("files", &h.Paths)
	str("text", &h.Text)
	list("branch", &h.Branches)
	list("host", &h.Hosts)
	fs.BoolVar(&h.Any, "any", h.Any, "")
	list("not", &h.Not)
	fs.BoolVar(&h.Once, "once", h.Once, "")
	fs.IntVar(&h.Max, "max", h.Max, "")
	fs.IntVar(&h.Every, "every", h.Every, "")
	str("hours", &h.Hours)
	str("schedule", &h.Schedule)
	fs.BoolVar(&h.Background, "background", h.Background, "")
	fs.IntVar(&h.Timeout, "timeout", h.Timeout, "")
	off := fs.Bool("off", false, "")
	scope := f.scope
	str("scope", &scope)
	dir := f.dir
	str("dir", &dir)
	why := ""
	str("why", &why)
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return errors.New(hooksUsage)
	}
	if id == "" && h.Event == "" {
		return errors.New("a hook needs --event (enowx-app hooks events lists them)\n" + hooksUsage)
	}
	h.Action = map[string]string{"refuse": proto.HookActionBlock}[do]
	if h.Action == "" {
		h.Action = do
	}
	if *off {
		h.On = false
	}
	if scope != f.scope && id != "" {
		return errors.New("a hook stays where it is kept: remove it and add it where you want it")
	}
	f.scope = scope
	switch scope {
	case proto.HookScopeProject:
		if dir == "" {
			dir = projectDirHere()
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		f.dir = abs
	case proto.HookScopeSession:
		if f.sess == "" {
			f.sess = os.Getenv(EnvSession)
		}
		if f.sess == "" {
			return errors.New("--scope session: run it in the session, as its agent")
		}
	case proto.HookScopeHost:
	default:
		return fmt.Errorf("no scope %q: host, project or session", scope)
	}
	return r.applyHook(f, false, why)
}

// applyHook saves f where it is kept, or removes it; from a session, it
// proposes that to the user instead.
func (r runner) applyHook(f foundHook, remove bool, why string) error {
	if by := os.Getenv(EnvSession); by != "" {
		p, err := r.c.ProposeHook(proto.HookProposal{Hook: f.hook, Remove: remove, Scope: f.scope, Dir: f.dir, Session: f.sess, By: by, Why: why})
		if err != nil {
			return err
		}
		fmt.Fprintf(r.out, "Proposed (%s): the user allows or rejects it in Settings → Hooks; it doesn't run before.\n", p.ID)
		return nil
	}
	info, err := r.c.Hooks(f.dir)
	if err != nil {
		return err
	}
	put := func(list []proto.Hook) []proto.Hook {
		i := slices.IndexFunc(list, func(h proto.Hook) bool { return f.hook.ID != "" && h.ID == f.hook.ID })
		switch {
		case remove && i >= 0:
			return slices.Delete(list, i, i+1)
		case i >= 0:
			list[i] = f.hook
		case !remove:
			list = append(list, f.hook)
		}
		return list
	}
	switch f.scope {
	case proto.HookScopeProject:
		var list []proto.Hook
		for _, ph := range info.Projects {
			if ph.Dir == f.dir {
				list = ph.Hooks
			}
		}
		return r.c.SaveProjectHooks(f.dir, put(list))
	case proto.HookScopeSession:
		var list []proto.Hook
		for _, sh := range info.Sessions {
			if sh.Session == f.sess {
				list = sh.Hooks
			}
		}
		return r.c.SaveSessionHooks(f.sess, put(list))
	}
	var host []proto.Hook
	for _, h := range info.Hooks {
		if h.Project == "" && h.Session == "" {
			host = append(host, h)
		}
	}
	saved, err := r.c.SaveHooks(put(host))
	if err == nil && !remove && f.hook.ID == "" && len(saved) > 0 {
		fmt.Fprintln(r.out, saved[len(saved)-1].ID)
	}
	return err
}

// hookFlags are the flags of hooks add and edit, as MCP arguments.
var hookFlags = []string{"name", "event", "do", "command", "context", "reason", "find", "replace", "url", "message", "kind", "agent", "project",
	"tool", "files", "text", "branch", "host", "any", "not", "once", "max", "every", "hours", "schedule", "background", "timeout", "off", "scope", "dir", "why"}

var hookProps = map[string]mcpProp{
	"name": {"string", "What it is for"}, "event": {"string", "When: one of `enowx hooks events` (tool.before, prompt.start, turn.done, file.changed, git.before, git.commit, schedule, …)"},
	"do":      {"string", "run (a command, the event as JSON on stdin; exit 2 refuses), context, refuse, rewrite (find/replace the command), notify, webhook, enowx (enowX's command)"},
	"command": {"string", "The command (run), or enowX's arguments (enowx)"}, "context": {"string", "Text the agent reads (context)"},
	"reason": {"string", "Why, told to the agent (refuse)"}, "find": {"string", "Regular expression in the command (rewrite)"}, "replace": {"string", "What replaces it (rewrite)"},
	"url": {"string", "Where a webhook posts"}, "message": {"string", "What a notification or webhook says: {agent}, {project}, {event}, {command}… filled in"},
	"kind": {"string", "A notification's sound: done, input or error"}, "agent": {"string", "Only these agents, comma-separated"}, "project": {"string", "Only these projects (folders or names)"},
	"tool": {"string", "Only tools or commands matching this regular expression"}, "files": {"string", "Only these files, globs comma-separated"},
	"text": {"string", "Only prompts or commands matching this regular expression"}, "branch": {"string", "Only these branches, globs"}, "host": {"string", "Only these hosts"},
	"any": {"boolean", "Any condition, not all"}, "not": {"string", "Conditions turned round, comma-separated: agents, projects, hosts, branches, tool, paths, text"},
	"once": {"boolean", "Once a session"}, "max": {"number", "At most this many times a session"}, "every": {"number", "At least this many seconds apart"},
	"hours": {"string", "Only within these hours, 09-18 or 22:00-06:00"}, "schedule": {"string", "When a schedule hook runs: hourly, daily 09:00, weekdays 18:30, weekly mon 08:00"},
	"background": {"boolean", "Don't make the agent wait"}, "timeout": {"number", "Seconds"}, "off": {"boolean", "Saved off"},
	"scope": {"string", "host (default), project (its .enowx/hooks.json) or session (this session alone)"}, "dir": {"string", "The project's folder, for scope project"},
	"why": {"string", "Why you propose it, shown to the user"},
}

func hookArgs(args []string, a map[string]any) []string {
	for _, k := range hookFlags {
		args = withFlag(args, a, k, k)
	}
	return args
}

// hookTools are the hooks as MCP tools. An agent's changes are proposals
// the user allows.
var hookTools = []mcpTool{
	{name: "hooks_list", description: "The hooks of this host, its projects and sessions (what runs when something happens in a session), those waiting for the user, and the changes proposed.",
		args: func(map[string]any) []string { return []string{"list", "--json"} }},
	{name: "hooks_add", description: "Propose a hook to the user: a command, context, a refusal, a rewrite, a notification or a webhook when an event happens, on every agent. It runs once the user allows it in Settings → Hooks.",
		props: hookProps, required: []string{"event"}, args: func(a map[string]any) []string { return hookArgs([]string{"add"}, a) }},
	{name: "hooks_edit", description: "Propose a change to a hook (by its ID), given only what changes.",
		props: withID(hookProps), required: []string{"id"}, args: func(a map[string]any) []string { return hookArgs([]string{"edit", str(a, "id")}, a) }},
	{name: "hooks_remove", description: "Propose removing a hook, by its ID.",
		props: map[string]mcpProp{"id": {"string", "The hook's ID"}}, required: []string{"id"}, args: func(a map[string]any) []string { return []string{"rm", str(a, "id")} }},
	{name: "hooks_test", description: "Run a hook now on a sample event, to see what it does.",
		props: map[string]mcpProp{"id": {"string", "The hook's ID"}}, required: []string{"id"}, args: func(a map[string]any) []string { return []string{"test", str(a, "id")} }},
	{name: "hooks_log", description: "The latest runs of the hooks: when, which, what came of it.",
		props: map[string]mcpProp{"last": {"number", "How many (20)"}}, args: func(a map[string]any) []string { return withFlag([]string{"log"}, a, "last", "last") }},
}

func withID(props map[string]mcpProp) map[string]mcpProp {
	out := map[string]mcpProp{"id": {"string", "The hook's ID"}}
	for k, v := range props {
		out[k] = v
	}
	return out
}
