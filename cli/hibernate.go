package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
)

const hibernateUsage = `usage: enowx-app hibernate SESSION... [--idle] [--when-done] [--force] [--dry-run]
       enowx-app hibernate SESSION --auto off|on | --auto-default off|on
       enowx-app wake SESSION
  Hibernating stops a session's agent, freeing the memory and CPU it held;
  waking brings it back on its conversation. A session that would lose
  work (its agent working, a task under way, a dev server, an app shown, a
  program left in the background) is only planned, unless --force stops it anyway or
  --when-done waits for its agent to finish. --idle: every session with
  nothing to lose. --dry-run only says what it would stop and free.
  --auto off keeps a session from hibernating on its own; --auto-default
  sets that for new sessions, on every host. From an agent's session, the
  user confirms in the app`

// hibernate hibernates sessions, or keeps one from hibernating on its own.
func (r runner) hibernate(args []string) error {
	var p proto.SessionHibernateParams
	auto, autoDefault := "", ""
	var refs []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--idle":
			p.Idle = true
		case "--when-done":
			p.WhenDone = true
		case "--force":
			p.Force = true
		case "--dry-run":
			p.DryRun = true
		case "--auto", "--auto-default":
			if i+1 >= len(args) || args[i+1] != "on" && args[i+1] != "off" {
				return errors.New(hibernateUsage)
			}
			if a == "--auto" {
				auto = args[i+1]
			} else {
				autoDefault = args[i+1]
			}
			i++
		case "help", "-h", "--help":
			fmt.Fprintln(r.out, hibernateUsage)
			return nil
		default:
			if strings.HasPrefix(a, "-") {
				return errors.New(hibernateUsage)
			}
			refs = append(refs, a)
		}
	}
	if autoDefault != "" {
		return r.c.SetAutoHibernate(proto.SessionAutoHibernateParams{Default: true, Never: autoDefault == "off"})
	}
	for _, ref := range refs {
		s, err := r.session(ref)
		if err != nil {
			return err
		}
		p.IDs = append(p.IDs, s.ID)
	}
	if auto != "" {
		if len(p.IDs) != 1 {
			return errors.New(hibernateUsage)
		}
		return r.c.SetAutoHibernate(proto.SessionAutoHibernateParams{ID: p.IDs[0], Never: auto == "off"})
	}
	if len(p.IDs) == 0 && !p.Idle {
		return errors.New(hibernateUsage)
	}
	p.From = os.Getenv("ENOWX_SESSION")
	res, err := r.c.HibernateSessions(p)
	if err != nil {
		return err
	}
	done := map[string]string{}
	for _, id := range res.Hibernated {
		done[id] = "hibernated"
	}
	for _, id := range res.Waiting {
		done[id] = "hibernates once its agent is done"
	}
	if len(res.Plans) == 0 {
		fmt.Fprintln(r.out, "nothing to hibernate")
	}
	for _, pl := range res.Plans {
		name := pl.ID
		if pl.Name != "" {
			name += " (" + pl.Name + ")"
		}
		switch {
		case pl.Blocked != "":
			fmt.Fprintf(r.out, "%s: not hibernated: %s\n", name, pl.Blocked)
			continue
		case done[pl.ID] != "":
			fmt.Fprintf(r.out, "%s: %s", name, done[pl.ID])
		case p.DryRun:
			fmt.Fprintf(r.out, "%s: would hibernate", name)
		default:
			fmt.Fprintf(r.out, "%s: not hibernated, it would stop work (--force, or --when-done)", name)
		}
		fmt.Fprintf(r.out, "; frees about %s, %.0f%% CPU\n", textfmt.Bytes(pl.RSS), pl.CPU)
		if pl.Working {
			fmt.Fprintln(r.out, "  its agent is working")
		}
		if pl.Task != "" {
			fmt.Fprintf(r.out, "  GOD of Agents' task %s is under way\n", pl.Task)
		}
		for _, name := range pl.Background {
			fmt.Fprintf(r.out, "  background agent %s stops with it, and what it hasn't saved may be lost\n", name)
		}
		for _, port := range pl.Ports {
			fmt.Fprintf(r.out, "  serves port %d\n", port)
		}
		if pl.Display {
			fmt.Fprintln(r.out, "  shows an app on a display")
		}
		for _, pr := range pl.Procs {
			left := ""
			if pr.Left {
				left = ", in the background"
			}
			fmt.Fprintf(r.out, "  stops %s (pid %d, %s%s)\n", pr.Name, pr.PID, textfmt.Bytes(pr.RSS), left)
		}
	}
	return nil
}

// wake wakes a hibernated session on its conversation.
func (r runner) wake(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: enowx-app wake SESSION")
	}
	s, err := r.session(args[0])
	if err != nil {
		return err
	}
	if !s.Hibernated && !s.Exited {
		return fmt.Errorf("%s is awake", s.ID)
	}
	_, err = r.c.WakeSession(s.ID)
	return err
}

// hibernateTools are hibernating and waking sessions as MCP tools.
var hibernateTools = []mcpTool{
	{name: "session_hibernate", description: "Hibernate sessions (or all idle ones) to free the host's memory and CPU: their agents stop and wake later on their conversation. One that would lose work (its agent working, a task under way, a dev server, an app shown) is only planned, unless force or when_done. Hibernating sessions you spawned, once done with them, frees the user's host; the user confirms in the app.",
		props: map[string]mcpProp{"ids": {"array", "The sessions"}, "idle": {"boolean", "Every session with nothing to lose"},
			"when_done": {"boolean", "Hibernate a working one once its agent is done"}, "force": {"boolean", "Stop them even if that loses work"},
			"dry_run": {"boolean", "Only say what it would stop and free"}},
		args: func(a map[string]any) []string {
			args := []string{"hibernate"}
			if ids, ok := a["ids"].([]any); ok {
				for _, id := range ids {
					args = append(args, fmt.Sprint(id))
				}
			}
			for _, k := range []string{"idle", "when_done", "force", "dry_run"} {
				args = withFlag(args, a, k, strings.ReplaceAll(k, "_", "-"))
			}
			return args
		}},
	{name: "session_wake", description: "Wake a hibernated session on its conversation.",
		props: map[string]mcpProp{"id": {"string", "The session"}}, required: []string{"id"},
		args: func(a map[string]any) []string { return []string{"wake", str(a, "id")} }},
}
