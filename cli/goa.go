package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// GOD of Agents' commands (docs/goa.md): its board, handing tasks over,
// settling them, its own RAG collection, the agents it can run on.

const goaUsage = `usage: enowx-app goa COMMAND
  skill [NAME]                     GOA's built-in guides: setup (every setting), features (every feature)
  doc [NAME] [--section HEADING]   enowX's docs, in the program: their list, or one of them
      --search WORDS   which docs and sections hold these words
  board [--json]                  the tasks, the session each went to, status and result
  delegate [flags] PROMPT          hand a task to the project's session, queued if it's busy; a new one only if it has none
      --session S      an existing session (else a new one:)
      --project DIR    its project: the project's idle or stopped session takes it, else a new one
      --branch B       always a new one, on a new branch in a worktree of its own
      --agent A        the agent of the session (default claude)
      --host H         the host (default this one)
      --model M --effort E --config C
                       chosen by hand, over the judge's route; --no-route keeps the agent's own
      --title T        the task's title on the board; --yolo skips the worker's permission prompts
  task ID [--status working|waiting|done|failed|stopped] [--result TEXT]
                                   settle a task, or say where it stands
  report --outcome done|partly|nothing|blocked|refused|failed TEXT
                                   a session's report of its turn, whatever came of it
  remember [--kind task|delegation|outcome|decision|preference] TEXT
                                   keep it in GOA's own collection
  recall QUERY [--project P [--host H]]...
                                   search GOA's collection, and projects read-only
  rules                            the user's rules for GOA, which they edit in the app
  stall [--blank D --idle D --stuck D]
                                   how long a task may not move before GOA is told, on this host (90s, 10m; 0 its default)
  agents                           the agents GOA can run on, and how far each holds it
  flags                            what a GOA only watched did itself
  start [--agent A]                start GOD of Agents on this host`

// goaReport is a session reporting how its turn ended, to its own host's daemon.
func (r runner) goaReport(args []string) error {
	fs := flag.NewFlagSet("goa report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outcome := fs.String("outcome", "", "")
	words, err := parseAll(fs, args)
	if err != nil || *outcome == "" {
		return errors.New("usage: enowx-app goa report --outcome " + strings.Join(proto.GOAOutcomes, "|") + " TEXT")
	}
	session := os.Getenv(EnvSession)
	if session == "" {
		return errors.New("goa report: it runs inside a session")
	}
	return r.c.Call(proto.MethodGOAReport, proto.GOAReportParams{Session: session, Outcome: *outcome, Text: strings.Join(words, " ")}, nil)
}

// goaCmd runs `enowx goa …`.
func (r runner) goaCmd(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(r.out, goaUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "skill":
		if len(args) < 2 {
			fmt.Fprintln(r.out, "Skills: "+strings.Join(agent.GOASkillNames(), ", ")+`: enowx-app goa skill NAME`)
			return nil
		}
		text, err := agent.GOASkill(args[1])
		if err != nil {
			return err
		}
		fmt.Fprint(r.out, text)
		return nil
	case "doc":
		return r.goaDoc(rest)
	case "board":
		return r.goaBoard(rest)
	case "delegate":
		return r.goaDelegate(rest)
	case "task":
		return r.goaTask(rest)
	case "report":
		return r.goaReport(rest)
	case "remember":
		return r.goaRemember(rest)
	case "recall":
		return r.goaRecall(rest)
	case "rules":
		var rules proto.GOARules
		if err := r.c.Call(proto.MethodGOARules, nil, &rules); err != nil {
			return err
		}
		if rules.Rules == "" {
			fmt.Fprintln(r.out, "The user set no rules for GOD of Agents.")
			return nil
		}
		fmt.Fprintln(r.out, rules.Rules)
		return nil
	case "stall":
		return r.goaStall(rest)
	case "agents":
		return r.goaAgents()
	case "flags":
		return r.goaFlags()
	case "start":
		return r.goaStart(rest)
	}
	return errors.New(goaUsage)
}

// taskModel is what a task really ran on, as its agent said; else what it
// was asked to run on, marked so; else unknown.
func taskModel(t proto.GOATask) string {
	if n := len(t.Ran); n > 0 {
		return t.Ran[n-1]
	}
	if t.Model != "" {
		return t.Model + " · " + nonEmpty(t.Effort, "unknown") + " (asked)"
	}
	return "unknown"
}

// taskPlace is a task's project: its name and path, "no project" for a session
// outside any, and "unknown" for a task from before it kept them.
func taskPlace(t proto.GOATask) string {
	switch {
	case t.Project != "":
		return filepath.Base(t.Project) + " " + t.Project
	case t.Dir != "":
		return "no project"
	}
	return "unknown"
}

func (r runner) goaBoard(args []string) error {
	var b proto.GOABoard
	if err := r.c.Call(proto.MethodGOABoard, nil, &b); err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "--json" {
		return r.printJSON(b)
	}
	if b.Session == "" {
		fmt.Fprintln(r.out, "No GOD of Agents runs on this host.")
	} else {
		fmt.Fprintf(r.out, "GOD of Agents: session %s (%s, %s)\n", b.Session, b.Agent, b.Level)
	}
	if len(b.Tasks) == 0 {
		fmt.Fprintln(r.out, "No tasks yet.")
		return nil
	}
	w := tabwriter.NewWriter(r.out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TASK\tSTATUS\tSESSION\tMODEL · EFFORT\tPROJECT\tTITLE")
	for _, t := range b.Tasks {
		where := short(t.Session)
		if t.Host != "" {
			where += "@" + t.Host
		}
		if t.SessionName != "" {
			where = t.SessionName + " " + where
		}
		status := t.Status
		if t.Asleep {
			status = "asleep"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", t.ID, status, where, taskModel(t), taskPlace(t), t.Title)
	}
	w.Flush()
	for _, t := range b.Tasks {
		if t.Asleep {
			fmt.Fprintf(r.out, "%s: its session is hibernated, and takes nothing up until something is sent to it or it is woken (it was %s)\n", t.ID, t.Status)
		}
		if t.Urgent != "" {
			fmt.Fprintf(r.out, "%s waits on the user (%s): %s\n", t.ID, t.Urgent, t.Asks)
		} else if t.Asks != "" {
			fmt.Fprintf(r.out, "%s asks: %s\n", t.ID, t.Asks)
		}
	}
	return nil
}

func (r runner) goaDelegate(args []string) error {
	fs := flagSet("goa delegate")
	p := proto.GOADelegateParams{From: os.Getenv(EnvSession)}
	fs.StringVar(&p.Session, "session", "", "")
	fs.StringVar(&p.Agent, "agent", "", "")
	fs.StringVar(&p.Project, "project", "", "")
	fs.StringVar(&p.Branch, "branch", "", "")
	fs.StringVar(&p.Host, "host", "", "")
	fs.StringVar(&p.Model, "model", "", "")
	fs.StringVar(&p.Effort, "effort", "", "")
	fs.StringVar(&p.Config, "config", "", "")
	fs.StringVar(&p.Title, "title", "", "")
	fs.BoolVar(&p.NoRoute, "no-route", false, "")
	fs.BoolVar(&p.SkipPermissions, "yolo", false, "")
	note := fs.String("note", "", "")
	items := fs.String("items", "", "")
	asJSON := fs.Bool("json", false, "")
	words, err := parseAll(fs, args)
	if err != nil || len(words) == 0 {
		return errors.New(goaUsage)
	}
	p.Prompt = strings.Join(words, " ")
	if *note != "" {
		n, err := r.findNote(*note)
		if err != nil {
			return err
		}
		p.Note = n.ID
		for _, item := range strings.Split(*items, "\n") {
			if item = strings.TrimSpace(item); item != "" {
				p.Items = append(p.Items, item)
			}
		}
	}
	if p.Project != "" && p.Host == "" {
		if abs, err := filepath.Abs(expandTilde(p.Project)); err == nil {
			p.Project = abs
		}
	}
	var t proto.GOATask
	if err := r.c.Call(proto.MethodGOADelegate, p, &t); err != nil {
		return err
	}
	if *asJSON {
		return r.printJSON(t)
	}
	where := "session " + t.Session
	if t.Host != "" {
		where += " on " + t.Host
	}
	switch {
	case t.Reused && t.Woken:
		where += ", the project's, brought back"
	case t.Reused:
		where += ", the project's"
	case t.Woken:
		where += ", brought back"
	}
	fmt.Fprintf(r.out, "Task %s → %s (%s)\n", t.ID, where, t.Agent)
	fmt.Fprintf(r.out, "asked for model: %s · effort: %s", nonEmpty(t.Model, "its own"), nonEmpty(t.Effort, "its own"))
	if t.Config != "" {
		fmt.Fprintf(r.out, " · config: %s", t.Config)
	}
	fmt.Fprintf(r.out, "\nwhy: %s\n", nonEmpty(t.Why, "—"))
	fmt.Fprintln(r.out, "You'll be told when it finishes or asks something ([enowX GOA] …); no need to wait on it.")
	return nil
}

func (r runner) goaTask(args []string) error {
	fs := flagSet("goa task")
	var p proto.GOATaskParams
	fs.StringVar(&p.Status, "status", "", "")
	fs.StringVar(&p.Result, "result", "", "")
	words, err := parseAll(fs, args)
	if err != nil || len(words) != 1 {
		return errors.New("usage: enowx-app goa task ID [--status S] [--result TEXT]")
	}
	p.ID = words[0]
	var t proto.GOATask
	if err := r.c.Call(proto.MethodGOATask, p, &t); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "%s: %s\n", t.ID, t.Status)
	for _, item := range t.Open {
		fmt.Fprintf(r.out, "Not ticked in its note: %s\n", item)
	}
	return nil
}

func (r runner) goaRemember(args []string) error {
	fs := flagSet("goa remember")
	kind := fs.String("kind", "", "")
	words, err := parseAll(fs, args)
	if err != nil || len(words) == 0 {
		return errors.New("usage: enowx-app goa remember [--kind task|delegation|outcome|decision|preference] TEXT")
	}
	var m proto.RAGMemory
	if err := r.c.Call(proto.MethodGOARemember, proto.GOARememberParams{Session: os.Getenv(EnvSession), Kind: *kind, Text: strings.Join(words, " ")}, &m); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Kept in GOA's collection (%s).\n", m.ID)
	return nil
}

func (r runner) goaRecall(args []string) error {
	var projects, hosts multiFlag
	fs := flagSet("goa recall")
	fs.Var(&projects, "project", "")
	fs.Var(&hosts, "host", "")
	k := fs.Int("k", 6, "")
	asJSON := fs.Bool("json", false, "")
	words, err := parseAll(fs, args)
	if err != nil || len(words) == 0 {
		return errors.New("usage: enowx-app goa recall QUERY [--project P [--host H]]... [--k N] [--json]")
	}
	p := proto.GOARecallParams{Session: os.Getenv(EnvSession), Query: strings.Join(words, " "), K: *k}
	for i, path := range projects {
		pr := proto.GOAProject{Path: path}
		if i < len(hosts) {
			pr.Host = hosts[i]
		}
		if pr.Host == "" {
			if abs, err := filepath.Abs(expandTilde(path)); err == nil {
				pr.Path = abs
			}
		}
		p.Projects = append(p.Projects, pr)
	}
	var hits []proto.RAGHit
	if err := r.c.Call(proto.MethodGOARecall, p, &hits); err != nil {
		return err
	}
	if *asJSON {
		return r.printJSON(hits)
	}
	if len(hits) == 0 {
		fmt.Fprintln(r.out, "Nothing found.")
	}
	for _, h := range hits {
		where := h.File
		if h.Start > 0 {
			where += fmt.Sprintf(":%d-%d", h.Start, h.End)
		}
		fmt.Fprintf(r.out, "%s %s\n%s\n\n", h.Source, where, strings.TrimSpace(h.Snippet))
	}
	return nil
}

func (r runner) goaAgents() error {
	var list []proto.GOAAgent
	if err := r.c.Call(proto.MethodGOAAgents, nil, &list); err != nil {
		return err
	}
	w := tabwriter.NewWriter(r.out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tLEVEL\tINSTALLED\tWHY")
	for _, a := range list {
		level, why := a.Level, a.Why
		if a.NotYet != "" {
			level, why = "not offered", a.NotYet
		}
		fmt.Fprintf(w, "%s\t%s\t%v\t%s\n", a.Agent, level, a.Installed, why)
	}
	return w.Flush()
}

func (r runner) goaFlags() error {
	var list []proto.GOAFlag
	if err := r.c.Call(proto.MethodGOAFlags, nil, &list); err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintln(r.out, "Nothing flagged.")
	}
	for _, f := range list {
		what := "ran " + f.Command
		if f.File != "" {
			what = f.Change + " " + filepath.Join(f.Project, f.File)
		}
		fmt.Fprintf(r.out, "%s %s %s\n", f.ID, f.Time.Format(time.Kitchen), what)
	}
	return nil
}

func (r runner) goaStart(args []string) error {
	fs := flagSet("goa start")
	agentID := fs.String("agent", "claude", "")
	yolo := fs.Bool("yolo", false, "")
	if _, err := parseAll(fs, args); err != nil {
		return errors.New("usage: enowx-app goa start [--agent A] [--yolo]")
	}
	var info proto.SessionInfo
	if err := r.c.Call(proto.MethodSessionCreate, proto.SessionCreateParams{Agent: *agentID, GOA: true, SkipUnheard: *yolo, Cols: 120, Rows: 40}, &info); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "GOD of Agents started: session %s (%s)\n", info.ID, info.Agent)
	return nil
}

func (r runner) printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(r.out, string(data))
	return nil
}

// expandTilde is path with a leading ~ as the home folder.
func expandTilde(path string) string {
	if rest, ok := strings.CutPrefix(path, "~"); ok && (rest == "" || rest[0] == '/') {
		if home, err := os.UserHomeDir(); err == nil {
			return home + rest
		}
	}
	return path
}
