package cli

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/enowx/enowx/proto"
)

const configsUsage = `usage: enowx-app configs COMMAND
  (no command), list [--agent A]   each agent's configs (the official one is the agent as installed)
  add AGENT NAME FLAGS...          save a config: another account (--account), an API key of the vault's
                                   (--cred C) or a model API (--url URL)
  edit AGENT NAME FLAGS...         change one: the flags given, --name NEW renames it
  rm AGENT NAME                    remove one (an account's sign-in on this host with it)
FLAGS
  --url URL --key-cred C           a model API and the vault's credential holding its key
  --model SPEC                     ID[,vision][,context=TOKENS][,output=TOKENS][,effort=E]; several, the first the default
  --small-model M --protocol P     its model for quick work; anthropic, openai, openai-responses or gemini
  --effort E                       the agent's reasoning effort
  --env NAME=VALUE --arg A         added to its sessions' environment, to the agent's command line (repeat them)
  --color #RRGGBB --icon NAME      its badge
Start a session on one with spawn --config NAME.`

// configsCmd runs `enowx configs …`: agents' configs, which sessions
// pick as they start (docs/configs.md).
func (r runner) configsCmd(args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list", "ls":
		fs := flagSet("configs list")
		only := fs.String("agent", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return errors.New(configsUsage)
		}
		return r.configsList(*only)
	case "add", "edit":
		if len(args) < 3 {
			return errors.New(configsUsage)
		}
		return r.configsSave(args[0] == "add", args[1], args[2], args[3:])
	case "rm", "remove":
		if len(args) != 3 {
			return errors.New(configsUsage)
		}
		c, err := r.config(args[1], args[2])
		if err != nil {
			return err
		}
		if err := r.c.RemoveAccount(c.ID); err != nil {
			return err
		}
		fmt.Fprintf(r.out, "removed %s's config %s\n", c.Agent, c.Name)
		return nil
	case "help":
		fmt.Fprintln(r.out, configsUsage)
		return nil
	}
	return errors.New(configsUsage)
}

func (r runner) configsList(only string) error {
	info, err := r.c.AccountsInfo()
	if err != nil {
		return err
	}
	list, err := r.c.Accounts()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tCONFIG\tKIND\tWHAT\tSTATE\tID")
	for _, st := range info.Accounts {
		if only != "" && st.Agent != only {
			continue
		}
		c := st.Account
		for _, x := range list {
			if x.ID == c.ID && c.ID != "" {
				c = x
			}
		}
		name, kind, what := c.Name, "account", "its own sign-in"
		switch {
		case st.Own:
			name, kind, what = "official", "official", "the agent as installed"
		case c.Kind == proto.ConfigKindAPI && c.API != nil:
			kind, what = "api", c.API.BaseURL
			if c.API.Model != "" {
				what += " · " + c.API.Model
			}
			if c.API.KeyCred != "" {
				what += " · key " + c.API.KeyCred
			}
		case c.Cred != "":
			kind, what = "key", "key "+c.Cred+" from the vault"
		}
		if c.Effort != "" {
			what += " · effort " + c.Effort
		}
		state := st.State
		if !st.Until.IsZero() {
			state += " until " + st.Until.Local().Format("Jan 2 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", st.Agent, name, kind, what, state, orDash(c.ID))
	}
	return w.Flush()
}

// config is agent's config named name (or of that ID).
func (r runner) config(agentID, name string) (proto.Account, error) {
	list, err := r.c.Accounts()
	if err != nil {
		return proto.Account{}, err
	}
	for _, a := range list {
		if a.Member == "" && (a.ID == name || a.Agent == baseAgent(agentID) && strings.EqualFold(a.Name, name)) {
			return a, nil
		}
	}
	return proto.Account{}, fmt.Errorf("configs: %s has no config %s (enowx-app configs lists them)", agentID, name)
}

// baseAgent is the agent whose configs a preset shares.
func baseAgent(id string) string {
	if id == "claude-agent-teams" {
		return "claude"
	}
	return id
}

// configsSave adds agent's config name, or edits it, by flags.
func (r runner) configsSave(add bool, agentID, name string, args []string) error {
	fs := flagSet("configs")
	newName := fs.String("name", "", "")
	account := fs.Bool("account", false, "")
	cred := fs.String("cred", "", "")
	url := fs.String("url", "", "")
	keyCred := fs.String("key-cred", "", "")
	small := fs.String("small-model", "", "")
	protocol := fs.String("protocol", "", "")
	effort := fs.String("effort", "", "")
	color := fs.String("color", "", "")
	icon := fs.String("icon", "", "")
	var models, env, extra multiFlag
	fs.Var(&models, "model", "")
	fs.Var(&env, "env", "")
	fs.Var(&extra, "arg", "")
	if err := fs.Parse(args); err != nil || len(fs.Args()) > 0 {
		return errors.New(configsUsage)
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	c := proto.Account{Agent: baseAgent(agentID), Name: name}
	if !add {
		var err error
		if c, err = r.config(agentID, name); err != nil {
			return err
		}
	}
	if *newName != "" {
		c.Name = *newName
	}
	switch {
	case add && *url != "":
		c.Kind, c.API = proto.ConfigKindAPI, &proto.ConfigAPI{}
	case add && *cred != "":
		c.Cred = *cred
	case add && !*account:
		return errors.New("configs add: --account, --cred C or --url URL")
	}
	if c.Kind == proto.ConfigKindAPI {
		api := *c.API
		if set["url"] {
			api.BaseURL = *url
		}
		if set["key-cred"] {
			api.KeyCred = *keyCred
		}
		if set["small-model"] {
			api.SmallModel = *small
		}
		if set["protocol"] {
			api.Protocol = *protocol
		}
		if len(models) > 0 {
			api.Models, api.Model = nil, ""
			for _, spec := range models {
				m, err := parseModelSpec(spec)
				if err != nil {
					return err
				}
				api.Models = append(api.Models, m)
			}
			api.Model = api.Models[0].ID
		}
		c.API = &api
	} else if set["url"] || set["key-cred"] || len(models) > 0 {
		return errors.New("configs: " + c.Name + " is an account, not a model API")
	}
	if set["cred"] && !add {
		c.Cred = *cred
	}
	if set["effort"] {
		c.Effort = *effort
	}
	if set["color"] {
		c.Color = *color
	}
	if set["icon"] {
		c.Icon = *icon
	}
	if len(env) > 0 {
		c.Env = env
	}
	if len(extra) > 0 {
		c.Args = extra
	}
	saved, err := r.c.SaveConfig(c)
	if err != nil {
		return err
	}
	verb := "changed"
	if add {
		verb = "saved"
	}
	fmt.Fprintf(r.out, "%s %s's config %s (%s): sessions pick it at New session, or spawn --agent %s --config %q\n", verb, saved.Agent, saved.Name, saved.ID, saved.Agent, saved.Name)
	if add && saved.Kind == proto.ConfigKindAccount && saved.Cred == "" {
		fmt.Fprintln(r.out, "Sign it in once on this host: Settings → Agents, its menu → Sign in.")
	}
	return nil
}
