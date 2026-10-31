package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/enowx/enowx/proto"
)

const peekUsage = `usage: enowx-app sessions               the sessions of every host the app reaches
       enowx-app peek SESSION [--host H] [--last N]
                                       a session's last messages (40) and its project's folder;
                                       its code then with "project context P --host H", "project read P FILE --host H"
Only when the user asks: another session is theirs to share`

// sessionsEverywhere lists the sessions of every host the app reaches.
func (r runner) sessionsEverywhere(args []string) error {
	if len(args) > 0 {
		return errors.New(peekUsage)
	}
	hosts, err := r.c.SessionsEverywhere()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "HOST\tSESSION\tNAME\tAGENT\tSTATE\tPROJECT")
	for _, h := range hosts {
		if h.Err != "" {
			fmt.Fprintf(w, "%s\t-\t(%s)\t\t\t\n", h.HostName, h.Err)
			continue
		}
		for _, s := range h.Sessions {
			if s.NotesFor != "" || s.CanvasFor != "" {
				continue // the Notes and Canvas pages' own agents
			}
			state := string(s.State)
			if s.Exited {
				state = "ended"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", h.HostName, s.ID, s.Name, s.Agent, state, s.Dir)
		}
	}
	if len(hosts) == 1 {
		fmt.Fprintln(w, "(only this host: open the enowX app to reach the others)\t\t\t\t\t")
	}
	return w.Flush()
}

// peek prints a session's last messages and where its project is.
func (r runner) peek(args []string) error {
	fs := flag.NewFlagSet("peek", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	host := fs.String("host", "", "")
	last := fs.Int("last", 0, "")
	arg, err := parseWithArg(fs, args)
	if err != nil || arg == "" {
		return errors.New(peekUsage)
	}
	p, err := r.c.PeekSession(proto.SessionPeekParams{Host: *host, Session: arg, Last: *last})
	if err != nil {
		return err
	}
	r.printPeek(p)
	return nil
}

// printPeek prints a session's last messages and where its project is.
func (r runner) printPeek(p proto.SessionPeek) {
	s := p.Session
	fmt.Fprintf(r.out, "# %s (%s) on %s\n", nonEmpty(s.Name, s.ID), s.Agent, p.HostName)
	fmt.Fprintf(r.out, "folder: %s\n", s.Dir)
	if s.Project != "" {
		fmt.Fprintf(r.out, "project: %s  (read its code: enowx-app project context %s --host %s)\n", s.Project, quoteArg(s.Project), quoteArg(p.HostName))
	}
	if s.Branch != "" {
		fmt.Fprintf(r.out, "branch: %s\n", s.Branch)
	}
	if p.Reason != "" {
		fmt.Fprintf(r.out, "\nNo conversation: %s\n", p.Reason)
		return
	}
	if p.Earlier > 0 {
		fmt.Fprintf(r.out, "\n(%d earlier messages: --last N reads more)\n", p.Earlier)
	}
	for _, m := range p.Messages {
		fmt.Fprintf(r.out, "\n[%s]\n%s\n", m.Role, strings.TrimSpace(m.Text))
	}
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// peekTools read other sessions, as MCP tools.
var peekTools = []mcpTool{
	{name: "sessions_everywhere", description: "The sessions of every host the user's enowX app reaches: host, ID, name, agent, state, folder. Only when the user asks to look at another session.",
		props: map[string]mcpProp{},
		args:  func(map[string]any) []string { return []string{"sessions"} }},
	{name: "session_peek", description: "Another session's last messages and its project's folder, read-only, only when the user asks (to copy a feature from another project, say); its code then with project_context and project_read on its host.",
		props: map[string]mcpProp{"session": {"string", "Its ID or name"}, "host": {"string", "Its host, as named in the app (default: this one)"},
			"last": {"string", "How many of its last messages (default 40)"}},
		required: []string{"session"},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"peek", str(a, "session")}, a, "host", "host"), a, "last", "last")
		}},
}
