package accounts

import (
	"strings"

	"github.com/enowx/enowx/core/agentapi"
)

// Way says how an agent takes a config, for Settings → Agents, configs and
// agent_api_list alike. Checked is how it was checked: "run" here for real,
// "help" its --help, "docs" read from its docs and not run.
type Way struct {
	Account  bool   // another account, in a home of its own
	Session  bool   // a model API per session
	Settings bool   // a model API in its own settings, backed up first
	Key      bool   // an API key lent to its sessions
	Base     string // another agent's configs it uses
	Why      string // why enowX offers no more than this; what its docs offer besides
	Checked  string
}

// None says the agent can have no config of its own at all.
func (w Way) None() bool { return !w.Account && !w.Session && !w.Settings && !w.Key }

// Says is w in a line.
func (w Way) Says() string {
	var ways []string
	if w.Account {
		ways = append(ways, "another account")
	}
	if w.Session {
		ways = append(ways, "a model API per session")
	}
	if w.Settings {
		ways = append(ways, "a model API in its own settings")
	}
	if w.Key {
		ways = append(ways, "an API key")
	}
	if len(ways) == 0 {
		return "no config"
	}
	return strings.Join(ways, ", ")
}

// agentWays are what each agent's CLI or docs said on 2026-11-01 or 02, beside what
// enowX drives (kinds, agentapi): a test fails when a preset is missing here.
var agentWays = map[string]Way{
	"claude":             {Key: true, Checked: "run"},
	"claude-agent-teams": {Base: "claude", Why: "Its sessions are Claude Code's, so they use Claude Code's configs.", Checked: "help"},
	"codex":              {Key: true, Checked: "run"},
	"opencode":           {Key: true, Why: "No separate account: it has no documented folder for one sign-in alone.", Checked: "run"},
	"gemini":             {Key: true, Why: "GEMINI_CLI_HOME could give another account; enowX doesn't drive it yet. Its API's address is https unless it's on localhost.", Checked: "docs"},
	"kiro":               {Key: true, Why: "It uses its own models and documents no address for your API. KIRO_HOME could give another account; enowX doesn't drive it yet.", Checked: "help"},
	"openclaude":         {Key: true, Why: "OPENCLAUDE_CONFIG_DIR could give another account; enowX doesn't drive it yet.", Checked: "docs"},
	"grok":               {Key: true, Why: "Its model API is in ~/.grok/config.toml, from a third party's docs: x.ai's couldn't be read. enowX doesn't write it.", Checked: "docs"},
	"copilot":            {Key: true, Why: "COPILOT_HOME could give another account; enowX doesn't drive it yet.", Checked: "docs"},
	"opencode2":          {Key: true, Why: "Only opencode.jsonc takes a model API; enowX doesn't write it yet.", Checked: "docs"},
	"mimo-code":          {Key: true, Why: "MIMOCODE_HOME moves all of its settings, and its own settings hold the API; enowX drives neither yet.", Checked: "docs"},
	"ante":               {Key: true, Why: "ANTE_HOME moves all of its settings, and catalog.json holds the API; enowX drives neither yet.", Checked: "docs"},
	"trae":               {Key: true, Why: "Its 2.0 settings for a model API aren't documented.", Checked: "docs"},
	"muse":               {Key: true, Why: "Its --base-url is for Meta's provider alone: no API of your own is documented.", Checked: "help"},
	"dsh":                {Key: true, Why: "DSH_HOME and settings.yaml are documented by a third party alone: its own site couldn't be read.", Checked: "docs"},
	"qoder":              {Key: true, Why: "A model is added only in its /model screen: its docs say not to edit the settings by hand.", Checked: "docs"},
	"qoder-cn":           {Key: true, Why: "A model is added only in its /model screen: its docs say not to edit the settings by hand.", Checked: "docs"},
	"zcode":              {Why: "Its docs describe a desktop app, with no command line to give a config to.", Checked: "docs"},
	"pi":                 {Key: true, Why: "PI_CODING_AGENT_DIR could give another account; enowX doesn't drive it yet.", Checked: "docs"},
	"omp":                {Key: true, Why: "It takes a provider's address, but only for models named in models.yml, which enowX doesn't write yet: a model of your own wasn't found when run.", Checked: "run"},
	"prime-agent":        {Key: true, Why: "Only a models.json takes a model API, at a place its docs don't name.", Checked: "docs"},
	"antigravity":        {Key: true, Why: "No separate account is documented.", Checked: "help"},
	"aider":              {Key: true, Why: "No account: it has no sign-in to separate, only API keys.", Checked: "docs"},
	"goose":              {Key: true, Why: "GOOSE_PATH_ROOT could give another account; enowX doesn't drive it yet. Its variables take one model, with no effort documented.", Checked: "docs"},
	"amp":                {Key: true, Why: "Its Custom URL connection is made in its own command, and isn't documented to go per session.", Checked: "docs"},
	"kilo":               {Key: true, Why: "KILO_CONFIG_DIR and its data folder could give another account; enowX doesn't drive them yet.", Checked: "docs"},
	"crush":              {Key: true, Why: "A config's sessions read its settings alone, not your own crush.json: CRUSH_GLOBAL_CONFIG replaces that file rather than adding to it. A model picked in a project's .crush folder can still win.", Checked: "docs"},
	"aug":                {Why: "It uses Augment's models and documents no address or key for your API.", Checked: "docs"},
	"autohand":           {Key: true, Why: "AUTOHAND_AI_BASE_URL is for Autohand's own provider and models, with its sign-in. Another provider's key is a plain value of its config file, which enowX doesn't write a key to.", Checked: "docs"},
	"cline":              {Key: true, Why: "cline auth takes the key on its command line, which enowX doesn't do.", Checked: "docs"},
	"codebuff":           {Key: true, Why: "Its user docs don't cover a model API of your own.", Checked: "docs"},
	"freebuff":           {Why: "It runs free on its own models, with no option for your own API or key.", Checked: "help"},
	"command-code":       {Key: true, Why: "providers.json takes a model API, and no per-session way is documented; enowX doesn't write it yet.", Checked: "help"},
	"continue":           {Key: true, Why: "Another account wasn't checked: it needs a real sign-in to test.", Checked: "docs"},
	"cursor":             {Key: true, Why: "It runs through Cursor's backend: the only provider of your own documented is AWS Bedrock.", Checked: "help"},
	"droid":              {Key: true, Why: "Its --settings isn't documented to carry a model; enowX writes its settings.", Checked: "help"},
	"kimi":               {Key: true, Why: "KIMI_CODE_HOME could give another account; enowX doesn't drive it yet. KIMI_MODEL_* takes one model, by OpenAI's or Anthropic's API, and needs a key.", Checked: "docs"},
	"mistral-vibe":       {Key: true, Why: "VIBE_HOME moves all of its settings; enowX writes them for now.", Checked: "docs"},
	"qwen-code":          {Key: true, Why: "Its variables take one model, by OpenAI's chat API or Anthropic's: the Responses API, several models and their limits are only in its settings.", Checked: "docs"},
	"rovo":               {Why: "It uses Atlassian's models and credits, and documents no API of your own.", Checked: "docs"},
	"hermes":             {Key: true, Why: "HERMES_HOME could give another account and config.yaml a model API; enowX drives neither yet.", Checked: "docs"},
	"devin":              {Why: "Only Devin's own models are documented, air-gapped builds aside.", Checked: "docs"},
	"openclaw":           {Key: true, Why: "OPENCLAW_CONFIG_PATH replaces its whole config, and its terminal normally talks to a Gateway that reads its own, so a session's variables wouldn't reach the model.", Checked: "docs"},
	"codebuddy":          {Key: true, Why: "CODEBUDDY_BASE_URL and its key do work per session, but run here it wrote the key into its own logs, so enowX doesn't send it that way.", Checked: "run"},
	"jcode":              {Key: true, Why: "JCODE_OPENAI_API_BASE and OPENAI_API_KEY exist, but its sessions share one server: they don't reach a server already running, and one a session starts keeps the key after it ends. Its own sign-in also wins over OPENAI_API_KEY.", Checked: "docs"},
}

// WayOf is how agent id takes a config, what enowX drives filled in.
func WayOf(id string) Way {
	w := agentWays[id]
	if w.Base != "" {
		return w
	}
	w.Account = Supported(id)
	w.Session = agentapi.PerSession(id)
	for _, s := range agentapi.Supported() {
		w.Settings = w.Settings || s == id && !w.Session
	}
	return w
}
