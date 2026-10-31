package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// Control of sessions on any host (docs/cli.md, "Control"): the same
// commands with --host H reach host H's daemon through this host's and the
// app (host.call); without it, this host. What can't be undone takes
// --yes, and from an agent the user says yes in the app too.

const controlUsage = `usage: enowx-app COMMAND SESSION ... [--host H]
  interrupt SESSION                stop what its agent is doing (its Escape or Ctrl+C)
  stop SESSION                     end its agent now, even mid-work, keeping the session
                                   (hibernate --force; wake or continue brings it back)
  continue SESSION                 start an ended or hibernated session's agent again, on its conversation
  rename SESSION NAME              rename it
  close SESSION --yes              end it and its tabs (kill is the same); from an agent the user confirms
  chat SESSION [--last N]          its last messages, as its agent recorded them
  answer SESSION [allow|always|deny|N]
                                   what its agent asks (a tool to allow, a question), or answer it
  switch SESSION [--model M] [--effort E] [--mode M] [--account A]
                                   change its model, effort, permission mode or account mid-session
  changes SESSION [status|diff|commit -m MSG|push --yes|pr [--title T] [--body B] --yes
                   |merge --yes|discard [--delete-branch] --yes] [--base B]
                                   its changes against its base, and what to do with them
  control log                      the requests sessions made through control on this host
--host H: a host as the app names it (enowx-app sessions lists them), or its daemon's ID;
other hosts need the enowX app open. With it, paths are that host's: give them whole.`

// hostCommands take --host: their calls go to that host. (peek and
// project context/read have a --host of their own, features a scope.)
var hostCommands = []string{"ls", "list", "status", "spawn", "send", "read", "wait", "kill", "close", "hibernate", "wake",
	"agents", "projects", "interrupt", "stop", "continue", "rename", "chat", "answer", "switch", "changes", "settings", "control"}

// hostFlag takes --host H (or --host=H) out of args, before a "--", for a
// command that goes to a host.
func hostFlag(args []string) (host string, rest []string, found bool) {
	if len(args) == 0 {
		return "", args, false
	}
	switch {
	case slices.Contains(hostCommands, args[0]):
	case args[0] == "project" && len(args) > 1 && slices.Contains([]string{"list", "ls", "add", "clone", "status", "cancel", "rules", "settings"}, args[1]):
	default:
		return "", args, false
	}
	for i, a := range args {
		switch {
		case a == "--":
			return "", args, false
		case a == "--host" && i+1 < len(args):
			return args[i+1], append(slices.Clone(args[:i]), args[i+2:]...), true
		case strings.HasPrefix(a, "--host="):
			return strings.TrimPrefix(a, "--host="), append(slices.Clone(args[:i]), args[i+1:]...), true
		}
	}
	return "", args, false
}

// controlCommand runs one of control's commands.
func (r runner) controlCommand(cmd string, args []string) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		switch cmd {
		case "changes":
			fmt.Fprintln(r.out, changesUsage)
		case "settings":
			fmt.Fprintln(r.out, settingsUsage)
		default:
			fmt.Fprintln(r.out, controlUsage)
		}
		return nil
	}
	switch cmd {
	case "interrupt":
		return r.interrupt(args)
	case "stop":
		return r.stop(args)
	case "continue":
		return r.continueSession(args)
	case "rename":
		return r.rename(args)
	case "close":
		return r.closeSession(args)
	case "chat":
		return r.chat(args)
	case "answer":
		return r.answer(args)
	case "switch":
		return r.switchSession(args)
	case "changes":
		return r.changes(args)
	case "settings":
		return r.settings(args)
	}
	return r.controlCmd(args)
}

// onHost runs args with every call going to host.
func (r runner) onHost(host string, args []string) error {
	if host == "" {
		return r.command(args)
	}
	rc, err := r.c.OnHost(host, os.Getenv(EnvSession))
	if err != nil {
		return err
	}
	defer rc.Close()
	return runner{c: rc, out: r.out, host: host}.command(args)
}

// hostPath is path as the host takes it: whole on this one, as given on
// another.
func (r runner) hostPath(path string) string {
	if r.host != "" {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// guarded calls a method that can't be undone: from an agent's session on
// this host, through host.call so that the user says yes in the app first
// (with --host, the relay does it already).
func (r runner) guarded(method string, params, out any) error {
	from := os.Getenv(EnvSession)
	if r.host != "" || from == "" {
		return r.c.Call(method, params, out)
	}
	return r.c.HostCall("", from, method, params, out)
}

// needYes refuses what can't be undone without --yes.
func needYes(yes bool, what string) error {
	if yes {
		return nil
	}
	return fmt.Errorf("%s can't be undone: run it again with --yes if that is what the user wants", what)
}

// sessionAndFlags parses args as SESSION then fs's flags, in any order.
func (r runner) sessionAndFlags(fs *flag.FlagSet, args []string, usage string) (proto.SessionInfo, []string, error) {
	fs.SetOutput(io.Discard)
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return proto.SessionInfo{}, nil, errors.New(usage)
		}
		args = fs.Args()
		if len(args) > 0 {
			pos, args = append(pos, args[0]), args[1:]
		}
	}
	if len(pos) == 0 {
		return proto.SessionInfo{}, nil, errors.New(usage)
	}
	s, err := r.session(pos[0])
	return s, pos[1:], err
}

func (r runner) interrupt(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: enowx-app interrupt SESSION [--host H]")
	}
	s, err := r.session(args[0])
	if err != nil {
		return err
	}
	return r.c.InterruptSession(s.ID)
}

func (r runner) stop(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: enowx-app stop SESSION [--host H]")
	}
	s, err := r.session(args[0])
	if err != nil {
		return err
	}
	res, err := r.c.HibernateSessions(proto.SessionHibernateParams{IDs: []string{s.ID}, Force: true, From: os.Getenv(EnvSession)})
	if err != nil {
		return err
	}
	if len(res.Hibernated) == 0 {
		why := "it can't be stopped now"
		if len(res.Plans) > 0 && res.Plans[0].Blocked != "" {
			why = res.Plans[0].Blocked
		}
		return fmt.Errorf("stop: %s", why)
	}
	fmt.Fprintf(r.out, "Stopped %s: \"continue %s\" brings it back on its conversation.\n", s.ID, s.ID)
	return nil
}

func (r runner) continueSession(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: enowx-app continue SESSION [--host H]")
	}
	s, err := r.session(args[0])
	if err != nil {
		return err
	}
	switch {
	case s.Hibernated:
		_, err = r.c.WakeSession(s.ID)
	case s.Exited:
		_, err = r.c.ContinueSession(s.ID)
	default:
		return fmt.Errorf("continue: %s is %s, not ended", s.ID, State(s))
	}
	return err
}

func (r runner) rename(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: enowx-app rename SESSION NAME [--host H]")
	}
	s, err := r.session(args[0])
	if err != nil {
		return err
	}
	return r.c.RenameSession(s.ID, strings.Join(args[1:], " "))
}

// closeSession ends a session, with --yes; a tab of one goes freely.
func (r runner) closeSession(args []string) error {
	fs := flag.NewFlagSet("close", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "")
	s, rest, err := r.sessionAndFlags(fs, args, "usage: enowx-app close SESSION --yes [--host H]")
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New("usage: enowx-app close SESSION --yes [--host H]")
	}
	if s.Parent != "" {
		return r.c.KillSession(s.ID) // a session's own tab
	}
	if err := needYes(*yes, "closing "+nonEmpty(sessionName(s), s.ID)+" (its terminal, tabs and what runs in them end)"); err != nil {
		return err
	}
	return r.guarded(proto.MethodSessionKill, proto.SessionRef{ID: s.ID}, nil)
}

// chat prints a session's last messages.
func (r runner) chat(args []string) error {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	last := fs.Int("last", 0, "")
	s, _, err := r.sessionAndFlags(fs, args, "usage: enowx-app chat SESSION [--last N] [--host H]")
	if err != nil {
		return err
	}
	p, err := r.c.PeekSession(proto.SessionPeekParams{Session: s.ID, Last: *last})
	if err != nil {
		return err
	}
	if r.host != "" {
		p.HostName = r.host
	}
	r.printPeek(p)
	return nil
}

// answer shows what a session's agent asks, or answers it.
func (r runner) answer(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: enowx-app answer SESSION [allow|always|deny|N] [--host H]")
	}
	s, err := r.session(args[0])
	if err != nil {
		return err
	}
	if len(args) == 2 {
		_, err := r.c.AnswerSession(s.ID, args[1])
		return err
	}
	ask, err := r.c.SessionAsk(s.ID)
	if err != nil {
		return err
	}
	if !ask.Waiting {
		fmt.Fprintf(r.out, "%s isn't waiting for an answer (%s).\n", s.ID, State(s))
		return nil
	}
	if ask.Tool != "" {
		fmt.Fprintf(r.out, "It asks to use %s", ask.Tool)
		if ask.Detail != "" {
			fmt.Fprintf(r.out, ":\n  %s", strings.ReplaceAll(ask.Detail, "\n", "\n  "))
		}
		fmt.Fprintln(r.out)
	}
	for i, q := range ask.Questions {
		fmt.Fprintf(r.out, "Question %d: %s\n", i+1, q.Question)
		for j, o := range q.Options {
			fmt.Fprintf(r.out, "  %d. %s\n", j+1, o)
		}
	}
	if ask.Reason != "" {
		fmt.Fprintln(r.out, ask.Reason)
	}
	if len(ask.Answers) > 0 {
		fmt.Fprintf(r.out, "Answer with: enowx-app answer %s %s\n", s.ID, strings.Join(ask.Answers, "|"))
	}
	return nil
}

// switchSession changes a session's model, effort, mode or account.
func (r runner) switchSession(args []string) error {
	fs := flag.NewFlagSet("switch", flag.ContinueOnError)
	model := fs.String("model", "", "")
	effort := fs.String("effort", "", "")
	mode := fs.String("mode", "", "")
	account := fs.String("account", "", "")
	const usage = "usage: enowx-app switch SESSION [--model M] [--effort E] [--mode M] [--account A] [--host H]"
	s, rest, err := r.sessionAndFlags(fs, args, usage)
	if err != nil {
		return err
	}
	if len(rest) > 0 || *model == "" && *effort == "" && *mode == "" && *account == "" {
		return errors.New(usage)
	}
	set := agent.SettingsOf(s.Agent)
	typeIn := func(what, command, v string, choices []agent.Choice) error {
		if command == "" {
			return fmt.Errorf("switch: %s's %s can't be changed while it runs", nonEmpty(s.Agent, "this program"), what)
		}
		if len(choices) > 0 && !slices.ContainsFunc(choices, func(c agent.Choice) bool { return c.ID == v }) {
			var ids []string
			for _, c := range choices {
				ids = append(ids, c.ID)
			}
			fmt.Fprintf(r.out, "(%s isn't among the %ss enowX knows of %s: %s; sent anyway)\n", v, what, s.Agent, strings.Join(ids, ", "))
		}
		return r.c.Input(s.ID, fmt.Sprintf(command, v), false, true)
	}
	if *model != "" {
		if err := typeIn("model", set.ModelCommand, *model, set.Models); err != nil {
			return err
		}
	}
	if *effort != "" {
		if err := typeIn("effort", set.EffortCommand, *effort, set.Efforts); err != nil {
			return err
		}
	}
	if *mode != "" {
		if set.ModeKey == "" {
			return fmt.Errorf("switch: %s's mode can't be changed while it runs", nonEmpty(s.Agent, "this program"))
		}
		if err := r.c.SwitchMode(s.ID, *mode); err != nil {
			return err
		}
	}
	if *account != "" {
		id, err := r.account(*account)
		if err != nil {
			return err
		}
		if _, err := r.c.SwitchAccount(proto.AccountSwitchParams{Session: s.ID, Account: id, Continue: true, Reason: "switched from another session"}); err != nil {
			return err
		}
	}
	return nil
}

// controlCmd is "control log".
func (r runner) controlCmd(args []string) error {
	if len(args) != 1 || args[0] != "log" {
		return errors.New("usage: enowx-app control log [--host H]")
	}
	log, err := r.c.ControlLog()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TIME\tFROM\tHOST\tMETHOD\tLEVEL\tRESULT")
	for _, e := range log {
		res := "ok"
		if e.Err != "" {
			res = e.Err
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Time.Format("01-02 15:04:05"), orDash(e.From), nonEmpty(e.Host, "here"), e.Method, e.Level, res)
	}
	return w.Flush()
}
