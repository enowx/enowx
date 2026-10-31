package cli

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/enowx/enowx/proto"
)

const settingsUsage = `usage: enowx-app settings [COMMAND] [--host H]
  (none) | show                    the host's settings, then the app's (when it is open)
  set KEY VALUE                    a host's setting:
      max-agents N                 agents working at once on the host, 0 no limit
      hibernate-after MINUTES      idle agents stopped after that long, 0 never
      auto-hibernate on|off        new sessions hibernate on their own (every host)
      shell PATH                   the shell its terminals start, "" the login shell
  env                              the variables every session on the host gets, values masked
  env set NAME VALUE               set one (a secret: from an agent, the user says yes in the app)
  env unset NAME                   remove one
  app                              the app's own settings: New session's defaults, terminal, theme…
  app set KEY VALUE                change one (the app must be open)
A project's settings: "project settings P"; its rules: "project rules P"; features: "features"`

// settings reads and changes a host's settings and the app's.
func (r runner) settings(args []string) error {
	if len(args) == 0 {
		args = []string{"show"}
	}
	switch cmd, rest := args[0], args[1:]; cmd {
	case "show":
		if err := r.showHostSettings(); err != nil {
			return err
		}
		if app, err := r.c.AppSettings(nil); err == nil {
			fmt.Fprintln(r.out, "\nThe app's:")
			r.printAppSettings(app, false)
		}
		return nil
	case "set":
		if len(rest) != 2 {
			return errors.New(settingsUsage)
		}
		return r.setHostSetting(rest[0], rest[1])
	case "env":
		return r.envSettings(rest)
	case "app":
		switch {
		case len(rest) == 0:
			app, err := r.c.AppSettings(nil)
			if err != nil {
				return err
			}
			r.printAppSettings(app, true)
			return nil
		case len(rest) == 3 && rest[0] == "set":
			_, err := r.c.AppSettings(map[string]string{rest[1]: rest[2]})
			return err
		}
	case "help":
		fmt.Fprintln(r.out, settingsUsage)
		return nil
	}
	return errors.New(settingsUsage)
}

func (r runner) showHostSettings() error {
	st, err := r.c.Settings()
	if err != nil {
		return err
	}
	prefs, _ := r.c.HibernatePrefs()
	shells, _ := r.c.Shells()
	w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "max-agents\t%d\t(0: no limit)\n", st.MaxAgents)
	fmt.Fprintf(w, "hibernate-after\t%d\t(minutes; 0: never)\n", st.HibernateAfter)
	fmt.Fprintf(w, "auto-hibernate\t%s\t(new sessions, every host)\n", onOff(!prefs.NeverByDefault))
	fmt.Fprintf(w, "shell\t%s\t\n", nonEmpty(shells.Default, "(the login shell)"))
	return w.Flush()
}

func (r runner) setHostSetting(key, value string) error {
	switch key {
	case "max-agents", "hibernate-after":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("settings: %s takes a number", key)
		}
		st, err := r.c.Settings()
		if err != nil {
			return err
		}
		if key == "max-agents" {
			st.MaxAgents = n
		} else {
			st.HibernateAfter = n
		}
		_, err = r.c.SetSettings(st)
		return err
	case "auto-hibernate":
		if value != "on" && value != "off" {
			return errors.New("settings: auto-hibernate on or off")
		}
		return r.c.SetAutoHibernate(proto.SessionAutoHibernateParams{Default: true, Never: value == "off"})
	case "shell":
		_, err := r.c.SetShell(value)
		return err
	}
	return fmt.Errorf("settings: no host setting %q (settings help)", key)
}

func (r runner) envSettings(args []string) error {
	var vars []proto.SessionEnvVar
	var err error
	switch {
	case len(args) == 0:
		vars, err = r.c.SessionEnv()
	case len(args) == 3 && args[0] == "set":
		err = r.guarded(proto.MethodSessionEnvSet, proto.SessionEnvVar{Name: args[1], Value: args[2]}, &vars)
	case len(args) == 2 && args[0] == "unset":
		vars, err = r.c.SetSessionEnv(args[1], "")
	default:
		return errors.New(settingsUsage)
	}
	if err != nil {
		return err
	}
	if len(vars) == 0 {
		fmt.Fprintln(r.out, "No variables.")
	}
	for _, v := range vars {
		fmt.Fprintf(r.out, "%s=%s\n", v.Name, v.Value)
	}
	return nil
}

func (r runner) printAppSettings(app proto.AppSettings, about bool) {
	keys := make([]string, 0, len(app.Values))
	for k := range app.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	for _, k := range keys {
		if about {
			fmt.Fprintf(w, "%s\t%s\t%s\n", k, nonEmpty(app.Values[k], `""`), app.About[k])
		} else {
			fmt.Fprintf(w, "%s\t%s\n", k, nonEmpty(app.Values[k], `""`))
		}
	}
	w.Flush()
}
