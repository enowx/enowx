package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/internal/quietcmd"
	"github.com/enowx/enowx/proto"
)

const credsUsage = `usage: enowx-app creds COMMAND
  (none) | list [--json]           what the vault holds: names, platforms, the variables each
                                   sets, statuses (never a value)
  use [NAME...] -- COMMAND...      run COMMAND with the credentials' variables (no NAME: the
                                   session's, creds env); what it prints is masked
  env NAME...                      lend credentials to this session: the programs it starts from
                                   now on get their variables
  add NAME --platform P [--account A] [--value TEXT|-] [--field KEY=VALUE]...
                                   keep a new credential; the user confirms in the app
  test NAME                        check it still works, with its platform
  rm NAME                          remove it; the user confirms in the app
  login PLATFORM                   ask the user to log in to PLATFORM in the app
  audit [--limit N]                the credentials used on this host: which, when, by whom
  platforms [QUERY]                the platforms the vault knows, and the variables each sets

The app holding the vault asks the user the first time a credential is
used on a host. Values never reach this command's output`

// creds uses the user's credential vault, through the app holding it.
func (r runner) creds(args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	cmd, rest := args[0], args[1:]
	session := os.Getenv(EnvSession)
	switch cmd {
	case "help":
		fmt.Fprintln(r.out, credsUsage)
		return nil
	case "list", "ls":
		return r.credsList(slices.Contains(rest, "--json"))
	case "platforms":
		return r.credsPlatforms(strings.Join(rest, " "))
	case "use":
		return r.credsUse(session, rest)
	case "env":
		if len(rest) == 0 {
			return errors.New(credsUsage)
		}
		if session == "" {
			return errors.New("creds env lends credentials to a session: run it in one, or use creds use")
		}
		names, err := r.c.VaultAttach(session, rest)
		if err != nil {
			return err
		}
		fmt.Fprintf(r.out, "This session has %s: programs it starts from now on get their variables, and so does creds use without a name.\n", strings.Join(names, ", "))
		return nil
	case "add":
		return r.credsAdd(session, rest)
	case "test", "rm", "remove":
		if len(rest) != 1 {
			return errors.New(credsUsage)
		}
		action := proto.VaultTest
		if cmd != "test" {
			action = proto.VaultRemove
		}
		lend, err := r.c.VaultAsk(proto.VaultAsk{Action: action, Name: rest[0], Session: session})
		if err != nil {
			return err
		}
		fmt.Fprintln(r.out, lend.Message)
		return nil
	case "login":
		if len(rest) != 1 {
			return errors.New(credsUsage)
		}
		lend, err := r.c.VaultAsk(proto.VaultAsk{Action: proto.VaultLogin, Platform: rest[0], Session: session})
		if err != nil {
			return err
		}
		fmt.Fprintln(r.out, lend.Message)
		return nil
	case "audit":
		fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		limit := fs.Int("limit", 50, "")
		if err := fs.Parse(rest); err != nil {
			return errors.New(credsUsage)
		}
		return r.credsAudit(*limit)
	case "git-helper":
		return r.gitHelper(session, rest, os.Stdin)
	}
	return errors.New(credsUsage)
}

func (r runner) credsList(asJSON bool) error {
	idx, err := r.c.VaultIndex()
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(r.out)
		enc.SetIndent("", "  ")
		return enc.Encode(idx)
	}
	if !idx.Present {
		fmt.Fprintln(r.out, "No vault on this host yet: the user makes one in the app (Credentials), or asks you to with creds login.")
		return nil
	}
	if len(idx.Credentials) == 0 {
		fmt.Fprintln(r.out, "The vault is empty: creds login PLATFORM asks the user to add one.")
		return nil
	}
	w := tabwriter.NewWriter(r.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPLATFORM\tACCOUNT\tSTATUS\tSETS")
	for _, c := range idx.Credentials {
		name := c.Name
		if c.Default {
			name += " *"
		}
		sets := strings.Join(c.Env, " ")
		switch {
		case c.PublicKey != "":
			sets = "ssh agent"
		case len(c.GitHosts) > 0 && sets != "":
			sets += " (git: " + strings.Join(c.GitHosts, ", ") + ")"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, vault.Lookup(c.Platform).Name, dash(c.Account), c.Status, dash(sets))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(r.out, "\n* the platform's default. Use one with: creds use NAME -- COMMAND")
	return nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func (r runner) credsPlatforms(q string) error {
	w := tabwriter.NewWriter(r.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPLATFORM\tSETS")
	for _, p := range vault.Search(q) {
		env := make([]string, 0, len(p.Env))
		for k := range p.Env {
			env = append(env, k)
		}
		slices.Sort(env)
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.Name, dash(strings.Join(env, " ")))
	}
	return w.Flush()
}

// credsUse runs a command with credentials' variables, masking their
// values in what it prints.
func (r runner) credsUse(session string, args []string) error {
	if err := goaRefuses("creds use"); err != nil {
		return err
	}
	i := slices.Index(args, "--")
	if i < 0 || i == len(args)-1 {
		return errors.New("usage: enowx-app creds use [NAME...] -- COMMAND [ARG...]")
	}
	names, command := args[:i], args[i+1:]
	if len(names) == 0 {
		names = []string{""} // the session's
	}
	env := map[string]string{}
	var secrets, cleanup []string
	defer func() {
		for _, d := range cleanup {
			_ = os.RemoveAll(d)
		}
	}()
	for _, name := range names {
		lend, err := r.c.VaultAsk(proto.VaultAsk{Action: proto.VaultEnv, Name: name, Session: session, Command: strings.Join(command, " ")})
		if err != nil {
			return err
		}
		for k, v := range lend.Env {
			env[k] = v
			if _, isFile := lend.Files[k]; isFile {
				cleanup = append(cleanup, filepath.Dir(v))
				if data, err := os.ReadFile(v); err == nil {
					secrets = append(secrets, string(data))
				}
				continue
			}
			secrets = append(secrets, v)
		}
	}
	cmd := quietcmd.Command(command[0], command[1:]...)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out := vault.NewRedactor(r.out, secrets)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, out
	if r.out != os.Stdout {
		cmd.Stdin = nil // an MCP tool's: no terminal
	}
	err := cmd.Run()
	_ = out.Close()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Errorf("%s exited with status %d", command[0], exit.ExitCode())
	}
	return err
}

func (r runner) credsAdd(session string, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	platform := fs.String("platform", "", "")
	account := fs.String("account", "", "")
	value := fs.String("value", "", "")
	fields := map[string]string{}
	fs.Func("field", "", func(s string) error {
		k, v, ok := strings.Cut(s, "=")
		if !ok || k == "" {
			return errors.New("--field KEY=VALUE")
		}
		fields[k] = v
		return nil
	})
	name, err := parseWithArg(fs, args)
	if err != nil || *platform == "" {
		return errors.New(credsUsage)
	}
	if *value == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		*value = strings.TrimSpace(string(data))
	}
	p := vault.Lookup(*platform)
	if *value != "" {
		fields[p.Fields[0].Key] = *value
	}
	for k, v := range fields {
		if v == "-" {
			return fmt.Errorf("--field %s=- : read one value from stdin with --value -", k)
		}
	}
	lend, err := r.c.VaultAsk(proto.VaultAsk{Action: proto.VaultAdd, Name: name, Platform: *platform, Account: *account, Fields: fields, Session: session})
	if err != nil {
		return err
	}
	fmt.Fprintln(r.out, lend.Message)
	return nil
}

func (r runner) credsAudit(limit int) error {
	list, err := r.c.VaultAudit(limit)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintln(r.out, "No credential used on this host yet.")
		return nil
	}
	w := tabwriter.NewWriter(r.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WHEN\tCREDENTIAL\tFOR\tSESSION\tRESULT")
	for _, e := range list {
		result := "allowed"
		if !e.Allowed {
			result = "refused"
		}
		what := e.Action
		if e.Detail != "" {
			what += ": " + e.Detail
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.Time.Local().Format("2 Jan 15:04"), dash(e.Credential), what, dash(e.Session), result)
	}
	return w.Flush()
}

// shellCommand is a command line run by the system's shell.
func shellCommand(line string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", line}
	}
	return []string{"sh", "-c", line}
}
