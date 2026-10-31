// Package cli is enowX's command line: what runs on a host (sessions, the
// agents in them, projects) listed, started, read, typed into and waited
// for, through the host's daemon. Agents run it too, as $ENOWX_BIN, to
// start other agents and follow them.
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
	"strings"
	"text/tabwriter"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
)

// EnvSession names the session a program runs in (session.EnvSession),
// which "self" stands for.
const EnvSession = "ENOWX_SESSION"

// Commands are the command line's commands, for the programs that also
// do other things with their arguments to tell them apart.
var Commands = []string{"ls", "list", "sessions", "peek", "status", "spawn", "send", "read", "wait", "kill", "hibernate", "wake", "agents", "projects", "project", "shells", "notes", "features", "creds", "canvas", "review", "sync", "icons", "attach", "pipeline", "pipelines", "note", "automation", "automations", "hooks", "accounts", "configs", "rag", "decide", "browser", "traffic", "android", "mcp", "guide", "ports", "run", "api", "notify", "cloud", "team", "help"}

// Is reports whether args start with one of Commands.
func Is(args []string) bool { return len(args) > 0 && slices.Contains(Commands, args[0]) }

const usage = `usage: enowx-app <command> [arguments]

  ls [--json]                      sessions on this host
  status [SESSION]                 what each agent is doing
  spawn [flags] [PROMPT]           start an agent; prints its session
      --agent ID       the agent (enowx agents lists them), default claude
      --agents A,B     several agents given the prompt, each on --branch
                       with its name after it, to compare
      --account NAME   the agent's account to sign in as (Settings → Agents)
      --config NAME    the agent's config to run with (configs lists them), by
                       default the preferences'; "official" for the agent as installed
      --project DIR    the project, default this session's or this folder
      --branch NAME    a new branch in a worktree of its own
      --on BRANCH      an existing branch instead
      --base BRANCH    what --branch starts from
      --name NAME      the session's name
      --yolo           skip permission prompts
      --tab            a tab of this session, in its folder
  send SESSION TEXT...             type a message into a session and send it
      --no-enter       type it without sending
  read SESSION                     print what a session's screen shows
  wait SESSION [--timeout 30m]     wait until its agent is done or asks
  note SESSION [TEXT...]           what the session is at, shown in the app ("" clears)
  kill SESSION                     end a session
  hibernate SESSION... [--idle] [--when-done] [--force] [--dry-run]
                                   stop sessions' agents to free the host, to wake later on
                                   their conversation; --auto off|on keeps one from (or lets
                                   it) hibernating on its own ("hibernate help")
  wake SESSION                     wake a hibernated session
  agents [--all]                   the agents installed here (--all: every one)
  projects                         the projects of this host, in the user's order, with their groups
  project list|add PATH|clone URL [--dir D]|status ID|cancel ID
                                   add a folder, or clone a repository as a project ("project help")
  projects group PATH GROUP        put a project in a group ("" takes it out)
  projects move PATH [--before P]  move a project before another, or last
  projects move-group GROUP [--before P]
                                   move a group's projects before project P, or last
  projects rename-group FROM TO    rename a group ("" ungroups its projects)
  notify [sound|set|status]        sounds the user's enowX app plays ("notify help")
  cloud [login|logout|devices|approve|url]
                                   this host's sign-in to a team cloud ("cloud help")
  team [list|create|members|invite|join|role|remove|leave]
                                   your teams in the cloud ("team help")
  notes [show|new|write|move|rm|import|export]
                                   the user's notes on this host ("notes help")
  icons search WORD [--lib L]      SVG icons for notes and groups (lib:name), by name and keyword
  canvas COMMAND                   mockups, wireframes and diagrams the user watches built live
  review -                         as a design's independent reviewer: send the verdict (Score:, Issue:, Improve:, Keep:, Next:)
                                   ("canvas help"; "canvas guide" on designing them)
  shells [use PATH|login]          the host's shells, and the one its terminals start with
  attach SESSION                   use a session's terminal here (Ctrl+] leaves)
  pipeline [flags] TASK            plan, implement and review TASK, one agent after another
      --branch NAME    its branch (default pipeline/<task>)
      --agents A,B,C   the agent of each stage (default claude,claude,claude)
      --project DIR    as for spawn
      --yolo           skip permission prompts
  pipelines                        the pipelines of this host
  automations                      the host's automations: prompts on a schedule
  automation add [flags] PROMPT    one more: --name, --schedule "weekdays 09:00", --agent,
                                   --project, --branch (a new one each run), --yolo, --disabled
  automation run|enable|disable|remove ID
  decide [status|test|log|mark|ask|trim -- COMMAND]
                                   enowX's judge: approving commands, trimming output, loops
  rag [search|remember|memories|forget|status|index]
                                   the project found by meaning (Settings → RAG)
  accounts [list|status|use|priority|fallback|log]
                                   agents' accounts: their limits, the order they're used in,
                                   falling back at a limit, a session's switched
  hooks [list|add|edit|rm|on|off|test|log|events]
                                   what runs when something happens in a session, on every agent;
                                   from a session, changes are proposed for the user to allow
  browser [--session S] ACTION     drive the session's Browser in the app (default: self)
      open URL                     load a page (a URL, a host, or a path of its dev server)
      snapshot                     the page as text, its controls numbered
      click REF                    click a control: its number, or a CSS selector
      fill REF TEXT                type TEXT into a field
      screenshot FILE              save a PNG of the page
      back | forward | reload
      eval JAVASCRIPT              run it in the page, print its value (its console, DOM, storage…)
      capture on|off               the Browser's previews go through Traffic, or not
  android COMMAND                  the host's Android devices: setup, status, start, stop,
                                   logcat, run (android help)
  mcp                              serve enowX's commands to an agent as MCP tools
  guide                            what enowX gives an agent in a session, and how to use it
  ports [--session S] [preview PORT]
                                   the ports the session listens on; preview one in the Browser
  run [--session S] [N|NAME|COMMAND]
                                   the project's commands (the Run menu), or run one in a new tab;
                                   --save NAME COMMAND, --forget N|NAME
  api [show|set|clear|restore AGENT]
                                   point agents at a model API of yours (api help), settings kept first
  features [list|on|off|lazy|preset]
                                   which of enowX's features agents get (features help)
  help NAME                        how to use a feature (traffic, android, canvas…)
  creds [list|use|env|add|test|rm|login|audit]
                                   the user's credential vault, by name, never a value (creds help)
  traffic COMMAND                  HTTP traffic the host caught: list, show, watch, mock,
                                   breakpoint, rules, send, export, attach… (traffic help)

SESSION is an ID, the start of one, a session's name, or "self".
`

// Run runs the command line with args (its command first), writing to out.
// Errors that are the user's (unknown session, bad flag) come back as
// such; Run doesn't exit.
func Run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" {
		if len(args) == 2 && featureHelp(out, args[1]) {
			return nil
		}
		fmt.Fprint(out, usage)
		return nil
	}
	switch args[0] {
	case "android":
		return runner{out: out}.android(args[1:])
	case "icons":
		return runner{out: out}.icons(args[1:])
	case "guide":
		fmt.Fprintln(out, sessionGuide(len(args) > 1 && args[1] == "--all"))
		return nil
	case "decide":
		if len(args) > 1 && args[1] == "trim" {
			return trim(args[2:], out) // the command first, the daemon only if needed
		}
	}
	c, err := Dial()
	if err != nil {
		return err
	}
	defer c.Close()
	return runner{c: c, out: out}.command(args)
}

// command runs one of the commands that need the daemon, args its name
// first.
func (r runner) command(args []string) error {
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "ls", "list":
		return r.list(rest)
	case "sessions":
		return r.sessionsEverywhere(rest)
	case "peek":
		return r.peek(rest)
	case "status":
		return r.status(rest)
	case "spawn":
		return r.spawn(rest)
	case "send":
		return r.send(rest)
	case "read":
		return r.read(rest)
	case "wait":
		return r.wait(rest)
	case "kill":
		return r.kill(rest)
	case "hibernate":
		return r.hibernate(rest)
	case "wake":
		return r.wake(rest)
	case "note":
		return r.note(rest)
	case "browser":
		return r.browser(rest)
	case "traffic":
		return r.traffic(rest)
	case "mcp":
		return r.mcp(os.Stdin, r.out)
	case "automation":
		return r.automation(rest)
	case "automations":
		return r.automations()
	case "hooks":
		return r.hooks(rest)
	case "accounts":
		return r.accountsCmd(rest)
	case "configs":
		return r.configsCmd(rest)
	case "rag":
		return r.ragCmd(rest)
	case "decide":
		return r.decideCmd(rest)
	case "agents":
		return r.agents(rest)
	case "projects":
		return r.projects(rest)
	case "project":
		return r.projectCmd(rest)
	case "shells":
		return r.shells(rest)
	case "notify":
		return r.notify(rest)
	case "cloud":
		return r.cloud(rest)
	case "team":
		return r.team(rest)
	case "sync":
		return r.sync(rest)
	case "features":
		return r.features(rest)
	case "creds":
		return r.creds(rest)
	case "notes":
		return r.notes(rest)
	case "icons":
		return r.icons(rest)
	case "canvas":
		return r.canvasCmd(rest)
	case "review":
		return r.review(rest)
	case "attach":
		return r.attach(rest)
	case "pipeline":
		return r.pipeline(rest)
	case "pipelines":
		return r.pipelines()
	case "android":
		return r.android(rest)
	case "guide":
		fmt.Fprintln(r.out, agent.Guide)
		return nil
	case "help":
		fmt.Fprint(r.out, usage)
		return nil
	case "ports":
		return r.ports(rest)
	case "run":
		return r.run(rest)
	case "api":
		return r.api(rest)
	}
	return fmt.Errorf("unknown command %q: enowx-app help lists them", cmd)
}

// Dial connects to this host's daemon, which must be running: the app or
// enowxd starts it.
func Dial() (*client.Client, error) {
	path, err := transport.SocketPath()
	if err != nil {
		return nil, err
	}
	conn, err := transport.DialLocal(path, nil)
	if err != nil {
		return nil, errors.New("enowX isn't running on this host: open the app, or run enowxd serve")
	}
	return client.New(conn, nil)
}

type runner struct {
	c   *client.Client
	out io.Writer
}

// session finds the session ref names: "self", an ID, its start, or a name.
func (r runner) session(ref string) (proto.SessionInfo, error) {
	if ref == "self" || ref == "." {
		ref = os.Getenv(EnvSession)
		if ref == "" {
			return proto.SessionInfo{}, errors.New("self: this isn't running in an enowX session")
		}
	}
	list, err := r.c.Sessions()
	if err != nil {
		return proto.SessionInfo{}, err
	}
	var found []proto.SessionInfo
	for _, s := range list {
		if s.ID == ref {
			return s, nil
		}
		if strings.HasPrefix(s.ID, ref) || s.Name == ref {
			found = append(found, s)
		}
	}
	switch len(found) {
	case 0:
		return proto.SessionInfo{}, fmt.Errorf("no session %q", ref)
	case 1:
		return found[0], nil
	}
	return proto.SessionInfo{}, fmt.Errorf("%q names %d sessions: give more of the ID", ref, len(found))
}

func (r runner) list(args []string) error {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	list, err := r.c.Sessions()
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(r.out, list)
	}
	w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tAGENT\tSTATE\tPROJECT\tBRANCH\tNAME\tNOTE")
	for _, s := range list {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", s.ID, orDash(s.Agent), State(s), orDash(filepath.Base(s.Project)), orDash(s.Branch), orDash(sessionName(s)), orDash(s.Note))
	}
	return w.Flush()
}

func (r runner) status(args []string) error {
	if len(args) == 0 {
		return r.list(nil)
	}
	s, err := r.session(args[0])
	if err != nil {
		return err
	}
	fmt.Fprintln(r.out, State(s))
	return nil
}

// State says what a session's program is doing, in a word: working,
// waiting (for the user), done, idle, queued, or exited (with its code).
func State(s proto.SessionInfo) string {
	switch {
	case s.Hibernated && s.HibernatedHow != nil && s.HibernatedHow.Manual:
		return "hibernated(by hand)"
	case s.Hibernated:
		return "hibernated"
	case s.Exited:
		return fmt.Sprintf("exited(%d)", s.ExitCode)
	case s.Queued:
		return "queued"
	case s.State != "":
		return s.State
	case s.Active:
		return "working"
	}
	return "idle"
}

func (r runner) spawn(args []string) error {
	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	agentID := fs.String("agent", "claude", "")
	agents := fs.String("agents", "", "")
	account := fs.String("account", "", "")
	config := fs.String("config", "", "")
	projectDir := fs.String("project", "", "")
	branch := fs.String("branch", "", "")
	on := fs.String("on", "", "")
	base := fs.String("base", "", "")
	name := fs.String("name", "", "")
	yolo := fs.Bool("yolo", false, "")
	tab := fs.Bool("tab", false, "")
	asJSON := fs.Bool("json", false, "")
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("spawn: %w (enowx-app help)", err)
	}
	p := proto.SessionCreateParams{
		Agent: *agentID, Name: *name, Prompt: strings.Join(fs.Args(), " "), SkipPermissions: *yolo,
		BranchMode: proto.BranchNone, Cols: 120, Rows: 36,
	}
	switch {
	case *tab:
		self, err := r.session("self")
		if err != nil {
			return err
		}
		p.Parent = self.ID
	case *projectDir != "":
		abs, err := filepath.Abs(*projectDir)
		if err != nil {
			return err
		}
		p.Project = abs
	case os.Getenv(EnvSession) != "":
		self, err := r.session("self")
		if err != nil {
			return err
		}
		p.Project = self.Project
		if p.Project == "" {
			p.Dir = self.Dir
		}
	default:
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		p.Project = wd
	}
	switch {
	case *branch != "" && *on != "":
		return errors.New("spawn: --branch makes a new branch, --on takes one that exists: give one")
	case *branch != "":
		p.BranchMode, p.Branch, p.Base = proto.BranchNew, *branch, *base
	case *on != "":
		p.BranchMode, p.Branch = proto.BranchExisting, *on
	}
	if p.Project != "" {
		if _, err := r.c.AddProject(p.Project); err != nil {
			return err
		}
	}
	switch {
	case *config != "" && (ownName(*config) == "" || strings.EqualFold(*config, "official")):
		// The agent as installed.
	case *config != "":
		c, err := r.config(p.Agent, *config)
		if err != nil {
			return err
		}
		p.Account = c.ID
	case *account != "":
		id, err := r.account(*account)
		if err != nil {
			return err
		}
		p.Account = id
	default:
		p.AccountAuto = true // the preferences' default
	}
	if *agents != "" {
		return r.fanOut(p, strings.Split(*agents, ","), *asJSON)
	}
	s, err := r.c.CreateSession(p)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(r.out, s)
	}
	fmt.Fprintln(r.out, s.ID)
	return nil
}

// account finds an account by its name or ID.
func (r runner) account(name string) (string, error) {
	list, err := r.c.Accounts()
	if err != nil {
		return "", err
	}
	for _, a := range list {
		if a.ID == name || strings.EqualFold(a.Name, name) {
			return a.ID, nil
		}
	}
	return "", fmt.Errorf("spawn: no account %s (Settings → Agent accounts)", name)
}

// fanOut gives p's prompt to each of agents, a session each on a branch of
// its own, and prints their IDs.
func (r runner) fanOut(p proto.SessionCreateParams, agents []string, asJSON bool) error {
	if p.BranchMode != proto.BranchNew {
		return errors.New("spawn: --agents needs --branch: each agent works on a branch of its own")
	}
	for i := range agents {
		agents[i] = strings.TrimSpace(agents[i])
	}
	list, err := r.c.FanOut(proto.FanOutParams{SessionCreateParams: p, Agents: agents})
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(r.out, list)
	}
	for _, s := range list {
		fmt.Fprintln(r.out, s.ID)
	}
	return nil
}

func (r runner) send(args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	noEnter := fs.Bool("no-enter", false, "")
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil || fs.NArg() < 2 {
		return errors.New("usage: enowx-app send [--no-enter] SESSION TEXT")
	}
	s, err := r.session(fs.Arg(0))
	if err != nil {
		return err
	}
	return r.c.Input(s.ID, strings.Join(fs.Args()[1:], " "), true, !*noEnter)
}

func (r runner) read(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: enowx-app read SESSION")
	}
	s, err := r.session(args[0])
	if err != nil {
		return err
	}
	text, err := r.c.Screen(s.ID)
	if err != nil {
		return err
	}
	fmt.Fprintln(r.out, strings.TrimRight(text, "\n "))
	return nil
}

// quietFor is how long an agent that reports no state must show nothing
// new to count as done.
const quietFor = 3 * time.Second

func (r runner) wait(args []string) error {
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 30*time.Minute, "")
	fs.SetOutput(io.Discard)
	// The help writes the session first, as people do.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append(args[1:], args[0])
	}
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 {
		return errors.New("usage: enowx-app wait SESSION [--timeout 30m]")
	}
	s, err := r.session(fs.Arg(0))
	if err != nil {
		return err
	}
	deadline := time.Now().Add(*timeout)
	var quietSince time.Time
	for {
		state := State(s)
		switch {
		case s.Exited, state == proto.StateDone, state == proto.StateWaiting:
			fmt.Fprintln(r.out, state)
			return nil
		case state == "idle":
			if quietSince.IsZero() {
				quietSince = time.Now()
			} else if time.Since(quietSince) >= quietFor {
				fmt.Fprintln(r.out, state)
				return nil
			}
		default:
			quietSince = time.Time{}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wait: %s is still %s after %s", s.ID, state, *timeout)
		}
		time.Sleep(500 * time.Millisecond)
		if s, err = r.session(s.ID); err != nil {
			return err
		}
	}
}

func (r runner) note(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: enowx-app note SESSION [TEXT...]")
	}
	s, err := r.session(args[0])
	if err != nil {
		return err
	}
	return r.c.SetNote(s.ID, strings.Join(args[1:], " "))
}

func (r runner) kill(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: enowx-app kill SESSION")
	}
	s, err := r.session(args[0])
	if err != nil {
		return err
	}
	return r.c.KillSession(s.ID)
}

func (r runner) agents(args []string) error {
	fs := flag.NewFlagSet("agents", flag.ContinueOnError)
	all := fs.Bool("all", false, "")
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	list, err := r.c.Agents()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tCOMMAND\tINSTALLED")
	for _, a := range list {
		if a.Command == "" || !a.Installed && !*all {
			continue
		}
		installed := "yes"
		if !a.Installed {
			installed = "no"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.ID, a.Name, a.Command, installed)
	}
	return w.Flush()
}

func sessionName(s proto.SessionInfo) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Title
}

func orDash(s string) string {
	if s == "" || s == "." {
		return "-"
	}
	return s
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// sessionGuide is the guide of the features on for this session (its
// project's), or every feature's with all or when the daemon can't say.
func sessionGuide(all bool) string {
	if all {
		return agent.Guide
	}
	c, err := Dial()
	if err != nil {
		return agent.Guide
	}
	defer c.Close()
	g, err := c.FeaturesGuide(proto.FeaturesParams{Session: os.Getenv(EnvSession)})
	if err != nil || g.Guide == "" {
		return agent.Guide
	}
	return g.Guide
}
