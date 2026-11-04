package cli

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
)

const backgroundUsage = "usage: enowx-app background [SESSION] | background stop SESSION PID --yes"

// background lists a session's background agents, or stops one.
func (r runner) background(args []string) error {
	if len(args) > 0 && args[0] == "stop" {
		return r.backgroundStop(args[1:])
	}
	if len(args) > 1 {
		return errors.New(backgroundUsage)
	}
	list, err := r.c.Sessions()
	if err != nil {
		return err
	}
	only := ""
	if len(args) == 1 {
		s, err := r.session(args[0])
		if err != nil {
			return err
		}
		only = s.ID
	}
	w := tabwriter.NewWriter(r.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SESSION\tPID\tAGENT\tFOR\tCPU\tMEMORY\tNOTE")
	now := time.Now()
	for _, s := range list {
		if only != "" && s.ID != only {
			continue
		}
		for _, b := range s.Background {
			note := ""
			switch {
			case b.Stuck:
				note = "stuck: next to no CPU for a while"
			case b.Left:
				note = "left from under its agent"
			}
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n", s.ID, b.PID, b.Name, textfmt.Age(b.Since, now), textfmt.CPU(b.CPUPercent), textfmt.Bytes(b.Memory), note)
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(r.out, "Seen by the process tree: a subagent inside its agent's process, at an API, or under a CLI's own background service (OpenCode's server) doesn't show.")
	return nil
}

func (r runner) backgroundStop(args []string) error {
	fs := flag.NewFlagSet("background stop", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "")
	s, rest, err := r.sessionAndFlags(fs, args, backgroundUsage)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return errors.New(backgroundUsage)
	}
	pid, err := strconv.Atoi(rest[0])
	if err != nil {
		return errors.New(backgroundUsage)
	}
	for _, b := range s.Background {
		if b.PID != pid {
			continue
		}
		if err := needYes(*yes, "stopping "+b.Name+" (what it hasn't saved is lost)"); err != nil {
			return err
		}
		return r.guarded(proto.MethodSessionStopBackground, proto.StopBackgroundParams{Session: s.ID, PID: pid, Start: b.Start}, nil)
	}
	return fmt.Errorf("process %d isn't a background agent of %s", pid, s.ID)
}
