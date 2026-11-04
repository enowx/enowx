package platformagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/enowx/enowx/core/platformagent/mcpclient"
	"github.com/enowx/enowx/core/platformagent/permission"
	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/core/tokens"
	"github.com/enowx/enowx/proto"
)

// Agent is the loop; it implements Runner.
//
// Who owns what: turnMu is held through a Turn, a Compact and a Clear, and whoever holds
// it owns main and everything in it (history above all), so one of those runs at a time.
// mu guards the settings, the money and the current file, for the setters and getters
// that may come from the terminal's goroutine in the middle of a turn; it is never held
// while calling out. A running turn's side-by-side tools share only emit (its file has a
// lock of its own), askSlot and mu.
type Agent struct {
	opts  Options
	front Front
	hooks Hooks

	client     *anthropic.Client // nil with no key
	secrets    []string          // what the file and errors must never hold
	firstParty bool

	engine  *permission.Engine
	mcp     *mcpclient.Servers
	prices  *tokens.Prices
	environ []string
	all     []tools.Tool // every tool the main thread offers

	// askSlot lets one question to the user out at a time, as Front promises.
	askSlot chan struct{}
	// backoff is the first wait between tries; tests shorten it.
	backoff time.Duration
	// streamIdle is how long a stream may say nothing before it counts as lost.
	streamIdle time.Duration

	turnMu sync.Mutex
	main   *thread

	mu       sync.Mutex
	conv     string
	file     *convFile
	model    string
	effort   string
	mode     string
	budget   float64 // the setting
	limit    float64 // what may be spent before asking again
	total    float64 // dollars so far
	used     int64   // tokens the main conversation fills
	unpriced bool    // said once that a model has no price
	closed   bool
}

var _ Runner = (*Agent)(nil)

// New opens or resumes the conversation Options say. No key doesn't fail it: Turn says
// where to add one.
func New(o Options) (*Agent, error) {
	if o.SessionDir == "" {
		return nil, errors.New("platformagent: no session folder")
	}
	if err := os.MkdirAll(o.SessionDir, 0o700); err != nil {
		return nil, fmt.Errorf("platformagent: session folder: %w", err)
	}
	if o.Dir == "" {
		dir, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("platformagent: working folder: %w", err)
		}
		o.Dir = dir
	}

	a := &Agent{
		opts:       o,
		front:      o.Front,
		hooks:      o.Hooks,
		prices:     tokens.NewPrices(nil),
		environ:    tools.CleanEnviron(os.Environ()),
		askSlot:    make(chan struct{}, 1),
		backoff:    time.Second,
		streamIdle: 3 * time.Minute,
	}
	if a.front == nil {
		a.front = noFront{}
	}
	a.connectAPI()

	saved, err := a.openConversation()
	if err != nil {
		return nil, err
	}

	a.model = first(o.Model, saved.model, record.DefaultModel)
	a.effort = first(o.Effort, saved.effort, record.DefaultEffort)
	a.mode = startMode(o, saved)
	switch {
	case o.NoBudget:
	case o.Budget > 0:
		a.budget = o.Budget
	case saved.found:
		a.budget = saved.budget
	default:
		a.budget = o.NewBudget
	}
	a.limit = a.budget
	a.total = saved.total
	a.used = saved.used

	a.engine = permission.New(a.mode, saved.rules)
	// permission's SetDir (relative paths, working-folder checks) may not be there yet.
	if withDir, ok := any(a.engine).(interface{ SetDir(string) }); ok {
		withDir.SetDir(o.Dir)
	}

	var problems []string
	a.all, problems = a.gatherTools()
	if os.Getenv(proto.EnvGOA) != "" {
		// GOD of Agents works through enowX's tools alone; asking for each would stop it.
		// The daemon's tool.before still judges every call. For this run only: on record
		// they would hold for a later run of the conversation that isn't GOA's.
		for _, tool := range a.all {
			if strings.HasPrefix(tool.Name(), "mcp__enowx__") {
				a.engine.AddForSession(tool.Name())
			}
		}
	}

	a.main = &thread{a: a, history: saved.history, lastPrompt: saved.lastPrompt}
	a.main.setup(a.all, a.systemBlocks())

	if saved.model != a.model || saved.effort != a.effort || saved.mode != a.mode || saved.budget != a.budget || !saved.found {
		a.writeMeta()
	}
	if saved.mode == record.ModeBypass && a.mode != record.ModeBypass && o.Mode == "" {
		a.notice("info", "This conversation was in bypass mode. It goes on asking first, as neither bypass flag was given again.")
	}
	for _, p := range problems {
		a.notice("warning", p)
	}
	// A crash or a hibernation can leave calls unanswered; the API refuses that.
	if open := saved.dangling; len(open) > 0 {
		a.main.answerWith(open, "interrupted")
		a.notice("info", "The last answer's tool calls never finished; they were marked interrupted.")
	}
	return a, nil
}

// startMode is the mode a conversation goes on in. What was kept stands, so a session's
// mode doesn't change by itself: a flag given every start can't undo the user's switching
// to something safer, and only --permission-mode says otherwise. Bypass alone doesn't come
// back from a file unasked: its flags must be there again.
func startMode(o Options, saved replayed) string {
	if o.Mode != "" {
		return o.Mode
	}
	if saved.mode != "" {
		if saved.mode == record.ModeBypass && !o.AllowBypass {
			return record.ModeDefault
		}
		return saved.mode
	}
	if o.SkipPermissions {
		return record.ModeBypass
	}
	return first(o.ModeIfNew, record.ModeDefault)
}

// connectAPI makes the client if the environment has a key. The key stays in the client
// and in secrets, in memory.
func (a *Agent) connectAPI() {
	// All the SDK reads of the environment it reads in NewClient. Forgetting it here keeps
	// whatever this process starts later from inheriting it by accident.
	defer tools.ForgetCredentials()
	base := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL"))
	a.firstParty = a.opts.FirstParty || base == "" || isAnthropicAddress(base)

	key, token := os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("ANTHROPIC_AUTH_TOKEN")
	for _, s := range []string{key, token} {
		if s != "" {
			a.secrets = append(a.secrets, s)
		}
	}
	opts := []option.RequestOption{option.WithMaxRetries(0)}
	switch {
	case key != "":
		// This SDK's WithAPIKey sends the key as a bearer token. The API documents keys
		// on x-api-key, and proxies look for them there.
		opts = append(opts, option.WithAPIKey(key), option.WithHeaderDel("Authorization"), option.WithHeader("X-Api-Key", key))
	case token != "":
		opts = append(opts, option.WithAuthToken(token))
	default:
		return
	}
	if base != "" {
		opts = append(opts, option.WithBaseURL(base))
	}
	client := anthropic.NewClient(opts...)
	a.client = &client
}

func isAnthropicAddress(base string) bool {
	u, err := url.Parse(base)
	return err == nil && strings.EqualFold(u.Hostname(), "api.anthropic.com")
}

// gatherTools is the built-in ones, AskUserQuestion, Task, the MCP servers' and the
// extra ones. Problems are what the user should be told.
func (a *Agent) gatherTools() ([]tools.Tool, []string) {
	var list []tools.Tool
	list = append(list, tools.Builtin()...)
	if ask := tools.NewAskUserQuestion(asker{a}); ask != nil {
		list = append(list, ask)
	}
	list = append(list, &taskTool{a: a})

	var problems []string
	if a.opts.MCPConfig != "" {
		servers, err := mcpclient.Connect(context.Background(), a.opts.MCPConfig, a.environ)
		if err != nil {
			problems = append(problems, "MCP servers: "+a.scrub(err.Error()))
		} else {
			a.mcp = servers
			list = append(list, servers.Tools()...)
			for _, p := range servers.Problems() {
				problems = append(problems, "MCP: "+a.scrub(p))
			}
		}
	}
	return append(list, a.opts.ExtraTools...), problems
}

// scrub takes the key out of text from elsewhere (an API's error, a proxy's echo).
func (a *Agent) scrub(s string) string {
	for _, secret := range a.secrets {
		s = strings.ReplaceAll(s, secret, "[key]")
	}
	return s
}

// scrubResult takes the key out of what a tool brought back before the model, the chat
// or the screen see it: a command, a file or an MCP server may have held it.
func (a *Agent) scrubResult(res tools.Result) tools.Result {
	res.Text, res.Old, res.New = a.scrub(res.Text), a.scrub(res.Old), a.scrub(res.New)
	return res
}

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// noFront is who answers when nobody can: refuse.
type noFront struct{}

func (noFront) Event(record.Record) {}
func (noFront) Permission(context.Context, PermissionRequest) (Decision, error) {
	return Deny, nil
}
func (noFront) Questions(context.Context, []record.Question) ([]string, error) {
	return nil, errors.New("no one to ask")
}
func (noFront) OverBudget(context.Context, float64, float64) (bool, error) { return false, nil }

// marshalStable is JSON with its bytes left alone: no HTML escaping, which would change
// what the API sent us and what we keep.
func marshalStable(v any) []byte {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic("platformagent: marshal: " + err.Error()) // only our own structs get here
	}
	return []byte(strings.TrimSuffix(b.String(), "\n"))
}
