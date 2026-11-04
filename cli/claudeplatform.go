package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode"

	"github.com/enowx/enowx/core/platformagent"
	"github.com/enowx/enowx/core/platformagent/hookbridge"
	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/terminal"
	"github.com/enowx/enowx/version"
)

// claudePlatformFlags are `enowx claude-platform`'s command line, with Claude Code's
// names, since enowX's launching passes the same.
type claudePlatformFlags struct {
	version bool

	model, effort, mode string
	skipPermissions     bool // --dangerously-skip-permissions: bypass from the start
	allowBypass         bool // --allow-dangerously-skip-permissions: bypass on offer

	continueLast bool
	resume       string
	sessionDir   string

	systemPromptFile string
	appendPrompts    []string // text and files' text, in the order given
	mcpConfigs       []string
	budget           float64
	budgetSet        bool
	prompt           string
}

// parseClaudePlatformFlags reads the command line. Files named are read here, for one
// missing to fail before the terminal changes.
func parseClaudePlatformFlags(args []string) (claudePlatformFlags, error) {
	var f claudePlatformFlags
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		// value is the flag's value, from =value or the next argument.
		needValue := func() (string, error) {
			if hasValue {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("claude-platform: %s needs a value", name)
			}
			i++
			return args[i], nil
		}
		var err error
		switch name {
		case "--version", "-v":
			f.version = true
		case "--model":
			f.model, err = needValue()
		case "--effort":
			f.effort, err = needValue()
		case "--permission-mode":
			f.mode, err = needValue()
			if err == nil && !validMode(f.mode) {
				err = fmt.Errorf("claude-platform: unknown --permission-mode %q: default, acceptEdits, plan or bypassPermissions", f.mode)
			}
		case "--dangerously-skip-permissions":
			f.skipPermissions = true
		case "--allow-dangerously-skip-permissions":
			f.allowBypass = true
		case "--continue", "-c":
			f.continueLast = true
		case "--resume", "-r":
			f.resume, err = needValue()
		case "--session-dir":
			f.sessionDir, err = needValue()
		case "--system-prompt-file":
			f.systemPromptFile, err = needValue()
		case "--append-system-prompt":
			var text string
			if text, err = needValue(); err == nil {
				f.appendPrompts = append(f.appendPrompts, text)
			}
		case "--append-system-prompt-file":
			var path string
			if path, err = needValue(); err == nil {
				var text []byte
				if text, err = os.ReadFile(path); err == nil {
					f.appendPrompts = append(f.appendPrompts, string(text))
				} else {
					err = fmt.Errorf("claude-platform: --append-system-prompt-file: %w", err)
				}
			}
		case "--mcp-config":
			var v string
			if v, err = needValue(); err == nil {
				f.mcpConfigs = append(f.mcpConfigs, v)
			}
		case "--max-budget":
			var v string
			if v, err = needValue(); err == nil {
				f.budget, err = strconv.ParseFloat(strings.TrimPrefix(v, "$"), 64)
				if err != nil || f.budget < 0 {
					err = fmt.Errorf("claude-platform: --max-budget: %q isn't a number of dollars", v)
				}
				f.budgetSet = true
			}
		default:
			err = fmt.Errorf("claude-platform: unknown flag %s", name)
		}
		if err != nil {
			return f, err
		}
	}
	f.prompt = strings.Join(positional, " ")
	if f.skipPermissions {
		f.mode = record.ModeBypass
	}
	if f.continueLast && f.resume != "" {
		return f, errors.New("claude-platform: --continue and --resume ask for different conversations")
	}
	return f, nil
}

func validMode(mode string) bool {
	switch mode {
	case record.ModeDefault, record.ModeAcceptEdits, record.ModePlan, record.ModeBypass:
		return true
	}
	return false
}

// options are the Agent's options these flags say, in folder dir, with home the user's
// folder. The Front and Hooks are the caller's.
func (f claudePlatformFlags) options(dir, home string) (platformagent.Options, error) {
	o := platformagent.Options{
		Dir:      dir,
		Resume:   f.resume,
		Continue: f.continueLast,
		Model:    f.model,
		Effort:   f.effort,
		Mode:     f.mode,
		Append:   strings.Join(f.appendPrompts, "\n\n"),
	}
	o.SessionDir = f.sessionDir
	if o.SessionDir == "" {
		o.SessionDir = filepath.Join(home, ".enowx-app", "claude-platform", "sessions", sanitizeSessionDir(dir))
	}
	switch {
	case f.budgetSet:
		o.Budget = f.budget
	case f.resume == "" && !f.continueLast:
		o.Budget = record.DefaultBudget // a conversation resumed keeps its own
	}
	switch {
	case f.systemPromptFile != "":
		text, err := os.ReadFile(f.systemPromptFile)
		if err != nil {
			return o, fmt.Errorf("claude-platform: --system-prompt-file: %w", err)
		}
		o.SystemPrompt = string(text)
	default:
		// What the app's editor keeps; without it, the built-in prompt.
		text, err := os.ReadFile(filepath.Join(home, ".enowx-app", "claude-platform", "system-prompt.md"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return o, fmt.Errorf("claude-platform: the system prompt: %w", err)
		}
		o.SystemPrompt = string(text)
	}
	mcp, err := mergeMCPConfigs(f.mcpConfigs)
	if err != nil {
		return o, err
	}
	o.MCPConfig = mcp
	return o, nil
}

// sanitizeSessionDir is a folder's path as one folder's name: what isn't an ASCII letter
// or digit is a dash. The chat's reader of conversations finds them by the same rule.
func sanitizeSessionDir(dir string) string {
	return strings.Map(func(r rune) rune {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return r
		}
		return '-'
	}, dir)
}

// mergeMCPConfigs joins --mcp-config values, each an mcp.json's path or its text, into
// the one the agent takes; a later server of a name wins. One value goes as it is.
func mergeMCPConfigs(values []string) (string, error) {
	switch len(values) {
	case 0:
		return "", nil
	case 1:
		return values[0], nil
	}
	servers := map[string]json.RawMessage{}
	for _, v := range values {
		text := []byte(v)
		if !strings.HasPrefix(strings.TrimSpace(v), "{") {
			var err error
			if text, err = os.ReadFile(v); err != nil {
				return "", fmt.Errorf("claude-platform: --mcp-config: %w", err)
			}
		}
		var cfg struct {
			Servers map[string]json.RawMessage `json:"mcpServers"`
		}
		if err := json.Unmarshal(text, &cfg); err != nil {
			return "", fmt.Errorf("claude-platform: --mcp-config: %w", err)
		}
		for name, s := range cfg.Servers {
			servers[name] = s
		}
	}
	merged, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		return "", fmt.Errorf("claude-platform: --mcp-config: %w", err)
	}
	return string(merged), nil
}

// claudePlatform runs Claude Platform, enowX's own agent, in this terminal: what a
// session's PTY starts as `"$ENOWX_BIN" claude-platform`. Everything else the agent
// needs comes from the environment the session was given.
func claudePlatform(args []string, out io.Writer) error {
	f, err := parseClaudePlatformFlags(args)
	if err != nil {
		return err
	}
	if f.version {
		fmt.Fprintf(out, "%s (enowX) %s\n", record.AgentID, version.String)
		return nil
	}
	return runClaudePlatform(f, os.Stdin, out, func(o platformagent.Options) (platformagent.Runner, error) {
		a, err := platformagent.New(o)
		if err != nil {
			return nil, err
		}
		return a, nil
	})
}

// runClaudePlatform is claudePlatform with what starts the agent given, for tests.
func runClaudePlatform(f claudePlatformFlags, in io.Reader, out io.Writer, start func(platformagent.Options) (platformagent.Runner, error)) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("claude-platform: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("claude-platform: %w", err)
	}
	o, err := f.options(dir, home)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	cfg := terminal.Config{
		In:          in,
		Out:         out,
		AllowBypass: f.allowBypass || f.mode == record.ModeBypass,
		FirstPrompt: f.prompt,
	}
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	if inOK && outOK && terminal.IsTerminal(inFile, outFile) {
		cfg.Interactive = true
		cfg.Cols = terminal.ColsOf(outFile)
	}
	bridge := hookbridge.New()
	bridge.SetMode(f.mode)
	if extra := bridge.Start(ctx); extra != "" {
		o.Append = strings.TrimSpace(o.Append + "\n\n" + extra)
	}
	defer bridge.End()

	t := terminal.New(cfg)
	o.Front = hookbridge.Front{Front: t, Bridge: bridge, Text: terminal.AssistantText}
	o.Hooks = bridge
	runner, err := start(o)
	if err != nil {
		return fmt.Errorf("claude-platform: %w", err)
	}
	if cfg.Interactive {
		restore, err := terminal.Raw(inFile, outFile)
		if err != nil {
			runner.Close()
			return err
		}
		defer restore()
	}
	return t.Run(ctx, runner)
}
