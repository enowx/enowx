package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/enowx/enowx/internal/quietcmd"
	"github.com/enowx/enowx/proto"
)

const decideUsage = `usage: enowx-app decide [status | test [JUDGE] | log [--use USE] [--last N] | mark ID [--right] [--note TEXT] | ask USE --command CMD | trim -- COMMAND]`

// ExitError is a command's exit code to pass on (decide trim's): the
// program exits with it, saying nothing more.
type ExitError struct{ Code int }

func (e ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// Exit is the code err asks the program to exit with: an ExitError's,
// else 1.
func Exit(err error) int {
	var e ExitError
	if errors.As(err, &e) {
		return e.Code
	}
	return 1
}

// decideCmd runs `enowx decide …`: the judge (docs/decide.md).
func (r runner) decideCmd(args []string) error {
	if len(args) == 0 {
		args = []string{"status"}
	}
	fs := flag.NewFlagSet("decide", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	use := fs.String("use", "", "")
	last := fs.Int("last", 20, "")
	right := fs.Bool("right", false, "")
	note := fs.String("note", "", "")
	command := fs.String("command", "", "")
	sub := args[0]
	var words []string
	rest := args[1:]
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return errors.New(decideUsage)
		}
		rest = fs.Args()
		if len(rest) > 0 {
			words, rest = append(words, rest[0]), rest[1:]
		}
	}
	switch sub {
	case "status", "judges":
		var st proto.DecideSettings
		var judges []proto.Judge
		if err := r.c.Call(proto.MethodDecideSettings, nil, &st); err != nil {
			return err
		}
		if err := r.c.Call(proto.MethodDecideJudges, nil, &judges); err != nil {
			return err
		}
		fmt.Fprintf(r.out, "Policy: %s\n", policyName(st))
		for _, u := range proto.DecideUses {
			on := "off"
			if st.Uses[u].On {
				on = "on"
			}
			fmt.Fprintf(r.out, "  %-8s %s\n", u, on)
		}
		fmt.Fprintln(r.out, "Judges, in the order tried:")
		if len(judges) == 0 {
			fmt.Fprintln(r.out, "  none found: install Claude Code or Codex, add a model API key to the vault, or run Ollama")
		}
		for _, j := range judges {
			fmt.Fprintf(r.out, "  %s  %s%s\n", j.ID, judgeState(j, st), "")
		}
		return nil
	case "test":
		id := ""
		if len(words) > 0 {
			id = words[0]
		}
		var judges []proto.Judge
		if err := r.c.Call(proto.MethodDecideTest, proto.JudgeRef{ID: id}, &judges); err != nil {
			return err
		}
		if len(judges) == 0 {
			return errors.New("no such judge: enowx decide status lists them")
		}
		for _, j := range judges {
			fmt.Fprintf(r.out, "%s  %s\n", j.ID, judgeState(j, proto.DecideSettings{}))
		}
		return nil
	case "log":
		var list []proto.Decision
		if err := r.c.Call(proto.MethodDecideLog, proto.DecideLogParams{Use: *use, Limit: *last}, &list); err != nil {
			return err
		}
		for _, d := range list {
			fmt.Fprintln(r.out, decisionLine(d))
		}
		return nil
	case "mark":
		if len(words) != 1 {
			return errors.New(decideUsage)
		}
		return r.c.Call(proto.MethodDecideMark, proto.DecideMark{ID: words[0], Wrong: !*right, Note: *note}, nil)
	case "ask":
		if len(words) != 1 {
			return errors.New(decideUsage)
		}
		var d proto.Decision
		a := proto.DecideAsk{Use: words[0], Session: os.Getenv(EnvSession), Command: *command}
		if a.Use == proto.DecideTrim || a.Use == proto.DecideState {
			data, _ := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
			a.Output, a.Screen = string(data), string(data)
		}
		if err := r.c.Call(proto.MethodDecideAsk, a, &d); err != nil {
			return err
		}
		fmt.Fprintln(r.out, decisionLine(d))
		if d.Text != "" {
			fmt.Fprintln(r.out, d.Text)
		}
		return nil
	}
	return errors.New(decideUsage)
}

func policyName(st proto.DecideSettings) string {
	switch st.Policy {
	case proto.DecideOneProvider:
		return "only " + st.Provider + " (and local models)"
	case proto.DecideAnyProvider:
		return "any provider"
	case proto.DecideLocalOnly:
		return "local models only"
	}
	return "the session's agent's provider (and local models)"
}

func judgeState(j proto.Judge, st proto.DecideSettings) string {
	for _, off := range st.Off {
		if off == j.ID {
			return "off"
		}
	}
	switch {
	case j.Tested.IsZero():
		return "not tested"
	case !j.OK:
		return "✗ " + j.Error
	}
	s := fmt.Sprintf("✓ %d ms", j.Ms)
	if j.Cost > 0 {
		s += fmt.Sprintf(", $%.4f a call", j.Cost)
	}
	return s
}

func decisionLine(d proto.Decision) string {
	when := d.Time.Local().Format("Jan 2 15:04")
	if d.Judge == "" {
		return fmt.Sprintf("%s  %s  %s  no answer: %s", d.ID, when, d.Use, d.Error)
	}
	line := fmt.Sprintf("%s  %s  %s  %s (%.2f) by %s in %d ms: %s", d.ID, when, d.Use, d.Verdict, d.Confidence, d.Judge, d.Ms, d.Why)
	if d.Acted != "" {
		line += "  → " + d.Acted
	}
	if d.Wrong {
		line += "  [marked wrong]"
	}
	return line
}

// Long output is what is worth trimming.
const (
	trimLines = 150
	trimChars = 12000
)

// trim runs `enowx decide trim -- COMMAND` (hook action trim): command in
// a shell, its output passed on whole when short; when long, what the
// judge keeps of it, the whole saved to a file named after it. It exits as
// command did; with no judge, nothing is lost.
func trim(args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return errors.New(decideUsage)
	}
	command := strings.Join(args, " ")
	shell, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/C"
	} else if _, err := exec.LookPath("bash"); err == nil {
		shell = "bash"
	}
	cmd := quietcmd.Command(shell, flag, command)
	var buf bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, &buf, &buf
	runErr := cmd.Run()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(runErr, &exit):
		code = exit.ExitCode()
	case runErr != nil:
		return runErr
	}
	output := buf.String()
	done := func() error {
		if code != 0 {
			return ExitError{code}
		}
		return nil
	}
	if strings.Count(output, "\n") <= trimLines && len(output) <= trimChars {
		io.WriteString(out, output)
		return done()
	}
	var d proto.Decision
	c, err := Dial()
	if err == nil {
		err = c.Call(proto.MethodDecideAsk, proto.DecideAsk{Use: proto.DecideTrim, Session: os.Getenv(EnvSession), Command: command, Output: output, Exit: code}, &d)
		c.Close()
	}
	if err != nil || d.Text == "" {
		io.WriteString(out, output)
		return done()
	}
	full := filepath.Join(os.TempDir(), "enowx-trim", time.Now().Format("20060102-150405")+fmt.Sprintf("-%d.log", os.Getpid()))
	saved := ""
	if os.MkdirAll(filepath.Dir(full), 0o700) == nil && os.WriteFile(full, buf.Bytes(), 0o600) == nil {
		saved = " The whole output: " + full
	}
	fmt.Fprintf(out, "%s\n\n[enowX kept %d of %d lines of this output (%s).%s]\n", strings.TrimRight(d.Text, "\n"),
		strings.Count(d.Text, "\n")+1, strings.Count(output, "\n")+1, d.Why, saved)
	return done()
}

// decideTools are the judge as MCP tools, for agents to see why they were
// paused or refused.
var decideTools = []mcpTool{
	{name: "decide_status", description: "enowX's judge on this host: which uses are on (approving commands, trimming long output, stopping loops, a quiet session's state), the policy, the judges.",
		args: func(map[string]any) []string { return []string{"status"} }},
	{name: "decide_log", description: "The judge's latest decisions: verdict, why, what enowX did. Read it when a command was refused or the turn was stopped.",
		props: map[string]mcpProp{"use": {"string", "Only this use: state, approve, trim, loop"}, "last": {"number", "How many (20)"}},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"log"}, a, "use", "use"), a, "last", "last")
		}},
}
