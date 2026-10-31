package cli

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/enowx/enowx/proto"
)

// The Ports tool and the Run menu, for agents: what a session listens on,
// and the project's commands, run in a tab of the session.

func (r runner) ports(args []string) error {
	fs := flagSet("ports")
	session := fs.String("session", "self", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := r.session(*session)
	if err != nil {
		return err
	}
	if fs.NArg() > 0 {
		if fs.Arg(0) != "preview" || fs.NArg() != 2 {
			return errors.New("usage: enowx-app ports [--session S] [preview PORT]")
		}
		// The Browser tool shows a port of the session as it would.
		return r.browser([]string{"--session", s.ID, proto.BrowserOpen, "localhost:" + fs.Arg(1)})
	}
	ps, err := r.c.Ports(s.ID)
	if err != nil {
		return err
	}
	if !ps.Supported {
		return errors.New("ports: this host can't tell which ports a session listens on")
	}
	if len(ps.Ports) == 0 {
		fmt.Fprintln(r.out, "No ports open.")
		return nil
	}
	w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tADDRESS\tPROCESS\tPID")
	for _, p := range ps.Ports {
		fmt.Fprintf(w, ":%d\t%s\t%s\t%d\n", p.Port, p.Addr, p.Process, p.PID)
	}
	return w.Flush()
}

func (r runner) run(args []string) error {
	if slices.ContainsFunc(args, func(a string) bool { return !strings.HasPrefix(a, "-") }) {
		if err := goaRefuses("run"); err != nil { // listing them stays allowed
			return err
		}
	}
	fs := flagSet("run")
	session := fs.String("session", "self", "")
	save := fs.String("save", "", "")
	forget := fs.Bool("forget", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := r.session(*session)
	if err != nil {
		return err
	}
	if *save != "" {
		cmd := strings.Join(fs.Args(), " ")
		if cmd == "" {
			return errors.New("usage: enowx-app run --save NAME COMMAND")
		}
		_, err := r.c.SaveCommand(s.ID, proto.RunCommand{Name: *save, Command: cmd}, false)
		return err
	}
	cmds, err := r.c.ProjectCommands(s.ID)
	if err != nil {
		return err
	}
	if fs.NArg() == 0 {
		w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
		for i, c := range cmds {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", i+1, c.Name, c.Command, c.Source)
		}
		return w.Flush()
	}
	want := strings.Join(fs.Args(), " ")
	pick := -1
	if n, err := strconv.Atoi(want); err == nil && n >= 1 && n <= len(cmds) {
		pick = n - 1
	}
	for i, c := range cmds {
		if pick < 0 && (c.Name == want || c.Command == want) {
			pick = i
		}
	}
	if *forget {
		if pick < 0 {
			return fmt.Errorf("run: no command %q", want)
		}
		_, err := r.c.SaveCommand(s.ID, cmds[pick], true)
		return err
	}
	cmd := proto.RunCommand{Name: want, Command: want}
	if pick >= 0 {
		cmd = cmds[pick]
	}
	tab, err := r.c.RunCommand(s.ID, cmd)
	if err != nil {
		return err
	}
	fmt.Fprintf(r.out, "%s\nRunning %q in tab %s: \"enowx-app read %s\" shows it.\n", tab.ID, cmd.Command, tab.ID, tab.ID)
	return nil
}
