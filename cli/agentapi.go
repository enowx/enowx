package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/enowx/enowx/proto"
)

const apiUsage = `usage: enowx-app api COMMAND
  (no command)                     the agents that take a model API of yours, and what each is set to
  show AGENT                       one agent: its API, the files it is set in, their backups
  set AGENT --url URL [--key KEY | --key-env VAR | --key-stdin] [--model SPEC]... [--small-model M]
          [--effort E] [--protocol P]
                                   point the agent at the API, its settings kept first. SPEC is a model
                                   and what the agent is told of it, as it takes it (api show AGENT):
                                   ID[,vision][,context=TOKENS][,output=TOKENS][,effort=E]; repeat it
                                   for several, the first the default. --effort is for agents with one
                                   effort for every model. P: anthropic, openai, openai-responses or gemini
  clear AGENT                      back to its own API, the rest of its settings kept
  restore AGENT [BACKUP]           its settings as a backup kept them; without one, as you had them`

// api points the host's agents at a model API of the user's choosing.
func (r runner) api(args []string) error {
	if len(args) == 0 {
		list, err := r.c.AgentAPIs()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "AGENT\tAPI\tMODEL\tKEY\tINSTALLED")
		for _, a := range list {
			api, model, key := "its own", "", ""
			if a.PerSession {
				api = "its own (configs give it others)"
			}
			if c := a.Current; c != nil {
				api, model, key = c.BaseURL, c.Model, c.APIKey
			}
			inst := ""
			if a.Installed {
				inst = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", a.Agent, api, orDash(model), orDash(key), inst)
		}
		return w.Flush()
	}
	cmd, rest := args[0], args[1:]
	if cmd == "help" {
		fmt.Fprintln(r.out, apiUsage)
		return nil
	}
	if len(rest) == 0 {
		return errors.New(apiUsage)
	}
	agent := rest[0]
	switch cmd {
	case "show":
		list, err := r.c.AgentAPIs()
		if err != nil {
			return err
		}
		for _, a := range list {
			if a.Agent == agent {
				r.writeAPI(a)
				return nil
			}
		}
		return fmt.Errorf("api: %s takes no API of yours (enowx-app api lists those that do)", agent)
	case "set":
		fs := flagSet("api set")
		url := fs.String("url", "", "")
		key := fs.String("key", "", "")
		keyEnv := fs.String("key-env", "", "")
		keyStdin := fs.Bool("key-stdin", false, "")
		var models multiFlag
		fs.Var(&models, "model", "")
		small := fs.String("small-model", "", "")
		effort := fs.String("effort", "", "")
		protocol := fs.String("protocol", "", "")
		if err := fs.Parse(rest[1:]); err != nil || *url == "" {
			return errors.New(apiUsage)
		}
		k := *key
		switch {
		case *keyEnv != "":
			if k = os.Getenv(*keyEnv); k == "" {
				return fmt.Errorf("api: $%s is empty", *keyEnv)
			}
		case *keyStdin:
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil && line == "" {
				return fmt.Errorf("api: reading the key: %w", err)
			}
			k = strings.TrimSpace(line)
		}
		api := proto.AgentAPI{BaseURL: *url, APIKey: k, SmallModel: *small, Effort: *effort, Protocol: *protocol}
		for _, spec := range models {
			m, err := parseModelSpec(spec)
			if err != nil {
				return err
			}
			api.Models = append(api.Models, m)
		}
		if len(api.Models) > 0 {
			api.Model = api.Models[0].ID
		}
		info, err := r.c.SetAgentAPI(agent, api)
		if err != nil {
			return err
		}
		r.writeAPI(info)
		fmt.Fprintln(r.out, "\nSessions of it started from now on use it; \"enowx-app api restore "+agent+"\" puts its settings back.")
		return nil
	case "clear":
		info, err := r.c.ClearAgentAPI(agent)
		if err != nil {
			return err
		}
		r.writeAPI(info)
		return nil
	case "restore":
		backup := ""
		if len(rest) > 1 {
			backup = rest[1]
		}
		info, err := r.c.RestoreAgentAPI(agent, backup)
		if err != nil {
			return err
		}
		r.writeAPI(info)
		return nil
	}
	return errors.New(apiUsage)
}

func (r runner) writeAPI(a proto.AgentAPIInfo) {
	fmt.Fprintf(r.out, "%s (%s)\n", a.Name, a.Agent)
	if a.Error != "" {
		fmt.Fprintf(r.out, "  problem: %s\n", a.Error)
	}
	if c := a.Current; c != nil {
		fmt.Fprintf(r.out, "  API      %s (%s)\n  key      %s\n", c.BaseURL, orDash(c.Protocol), orDash(c.APIKey))
		for i, m := range c.Models {
			what := "model   "
			if i == 0 {
				what = "default "
			}
			fmt.Fprintf(r.out, "  %s %s\n", what, modelSpec(m))
		}
		if c.SmallModel != "" {
			fmt.Fprintf(r.out, "  small    %s\n", c.SmallModel)
		}
		if c.Effort != "" {
			fmt.Fprintf(r.out, "  effort   %s\n", c.Effort)
		}
	} else {
		fmt.Fprintln(r.out, "  API      its own")
	}
	fmt.Fprintf(r.out, "  speaks   %s\n  files    %s\n  takes    %s\n", strings.Join(a.Protocols, ", "), strings.Join(a.Files, ", "), capsText(a))
	if a.Note != "" {
		fmt.Fprintf(r.out, "  note     %s\n", a.Note)
	}
	for _, b := range a.Backups {
		what := b.Note
		if b.Original {
			what = "as you had it"
		}
		fmt.Fprintf(r.out, "  backup   %s  %s  %s\n", b.ID, b.Time.Format("2006-01-02 15:04"), what)
	}
}

// parseModelSpec reads ID[,vision][,context=N][,output=N][,effort=E].
func parseModelSpec(spec string) (proto.AgentAPIModel, error) {
	parts := strings.Split(spec, ",")
	m := proto.AgentAPIModel{ID: strings.TrimSpace(parts[0])}
	if m.ID == "" {
		return m, fmt.Errorf("api: --model %q: the model's id first", spec)
	}
	for _, p := range parts[1:] {
		k, v, _ := strings.Cut(strings.TrimSpace(p), "=")
		var err error
		switch k {
		case "vision", "images":
			m.Vision = true
		case "context", "ctx":
			m.Context, err = strconv.Atoi(v)
		case "output", "out":
			m.Output, err = strconv.Atoi(v)
		case "effort":
			m.Effort = v
		default:
			return m, fmt.Errorf("api: --model %q: %q is none of vision, context=, output=, effort=", spec, p)
		}
		if err != nil {
			return m, fmt.Errorf("api: --model %q: %s is a number of tokens", spec, k)
		}
	}
	return m, nil
}

// modelSpec writes a model as --model takes it.
func modelSpec(m proto.AgentAPIModel) string {
	out := m.ID
	if m.Vision {
		out += ",vision"
	}
	if m.Context > 0 {
		out += ",context=" + strconv.Itoa(m.Context)
	}
	if m.Output > 0 {
		out += ",output=" + strconv.Itoa(m.Output)
	}
	if m.Effort != "" {
		out += ",effort=" + m.Effort
	}
	return out
}

// capsText says what an agent takes of an API's models.
func capsText(a proto.AgentAPIInfo) string {
	c := a.Caps
	var out []string
	switch {
	case a.NoModel:
		return "no model: the API's own"
	case c.Multi:
		out = append(out, "several models")
	default:
		out = append(out, "one model")
	}
	if a.Small {
		out = append(out, "a model for quick work")
	}
	if c.Vision {
		out = append(out, "vision")
	}
	scope := func(what, s string) {
		switch s {
		case "model":
			out = append(out, what+" per model")
		case "global":
			out = append(out, what+" (the default's, for all)")
		}
	}
	scope("context", c.Context)
	scope("output", c.Output)
	switch c.Effort {
	case "model":
		out = append(out, "effort per model: "+strings.Join(c.Efforts, "|"))
	case "global":
		out = append(out, "one effort for all (--effort): "+strings.Join(c.Efforts, "|"))
	}
	return strings.Join(out, ", ")
}
