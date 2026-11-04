package accounts

import (
	"strings"

	"github.com/enowx/enowx/core/agent"
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
// An agent with a home variable has another account only where its docs name
// the sign-in's files: without them share would link the sign-in to the
// user's own, and signing in would change that. Those it has are read from
// docs and never signed in (kind.docsOnly); each Why says so.
var agentWays = map[string]Way{
	"claude":             {Key: true, Checked: "run"},
	agent.PlatformID:     {Key: true, Why: "No other account: it signs in with nothing but an API key from the vault, and a config's model API speaks Anthropic's protocol.", Checked: "run"},
	"claude-agent-teams": {Base: "claude", Why: "Its sessions are Claude Code's, so they use Claude Code's configs.", Checked: "help"},
	"codex":              {Key: true, Checked: "run"},
	"opencode":           {Key: true, Why: "No separate account: it has no documented folder for one sign-in alone.", Checked: "run"},
	"gemini":             {Key: true, Why: "No other account: GEMINI_CLI_HOME moves its folder, but its docs name no file for the sign-in, and reports have it in the system keychain, which that doesn't move. Its API's address is https unless it's on localhost.", Checked: "docs"},
	"kiro":               {Key: true, Why: "It uses its own models and documents no address for your API. No other account: KIRO_HOME moves its sign-ins (V3), but its docs name no file for them, so what to keep apart isn't known.", Checked: "help"},
	"openclaude":         {Key: true, Why: "No other account: OPENCLAUDE_CONFIG_DIR moves its folder, but its docs don't say where in it, if there, a sign-in is kept.", Checked: "docs"},
	"grok":               {Key: true, Why: "Its model API is in ~/.grok/config.toml, from a third party's docs: x.ai's couldn't be read. enowX doesn't write it.", Checked: "docs"},
	"copilot":            {Key: true, Why: "No other account: its docs keep the token in the system keychain under one service, which COPILOT_HOME doesn't move, without saying how it's keyed. Its own /user switch changes accounts.", Checked: "docs"},
	"opencode2":          {Key: true, Why: "No separate account: it has no documented folder for one sign-in alone.", Checked: "run"},
	"mimo-code":          {Key: true, Why: "No other account: MIMOCODE_HOME stands for several folders at once (config, data, state, cache), which a home linked from one folder can't share. Its own settings hold the API; enowX doesn't write them yet.", Checked: "docs"},
	"ante":               {Key: true, Why: "No other account: ANTE_HOME moves all of its settings, but its docs name no file for a sign-in. catalog.json holds the API; enowX doesn't write it yet.", Checked: "docs"},
	"trae":               {Key: true, Why: "Its 2.0 settings for a model API aren't documented. No other account: its docs document neither TRAE_HOME nor a file for the sign-in.", Checked: "docs"},
	"muse":               {Key: true, Why: "Its --base-url is for Meta's provider alone: no API of your own is documented.", Checked: "help"},
	"dsh":                {Key: true, Why: "DSH_HOME, .credentials.yaml and settings.yaml are documented by third parties alone: its own site couldn't be read, so it has no other account or model API here.", Checked: "docs"},
	"qoder":              {Key: true, Why: "A model is added only in its /model screen: its docs say not to edit the settings by hand. No other account: QODER_CONFIG_DIR moves its folder, but its docs name no file for the sign-in.", Checked: "docs"},
	"qoder-cn":           {Key: true, Why: "A model is added only in its /model screen: its docs say not to edit the settings by hand. No other account: QODERCN_CONFIG_DIR moves its folder, but its docs name no file for the sign-in.", Checked: "docs"},
	"zcode":              {Why: "Its docs describe a desktop app, with no command line to give a config to.", Checked: "docs"},
	"pi":                 {Key: true, Why: "Another account is read from its docs, not signed in: auth.json in PI_CODING_AGENT_DIR is the account's. It has no sign-in command: use /login in a session on the config.", Checked: "docs"},
	"omp":                {Key: true, Why: "A config's sessions read only the config's models, not your own ~/.omp settings, skills and sign-in: a folder of the session's own is the only way to give one another models.yml.", Checked: "run"},
	"prime-agent":        {Key: true, Why: "Only a models.json takes a model API, at a place its docs don't name.", Checked: "docs"},
	"antigravity":        {Key: true, Why: "No separate account is documented.", Checked: "help"},
	"aider":              {Key: true, Why: "No account: it has no sign-in to separate, only API keys.", Checked: "docs"},
	"goose":              {Key: true, Why: "No other account: GOOSE_PATH_ROOT stands for several folders, and its secrets are in the system keyring, which that doesn't move. Its variables take one model, with no effort documented.", Checked: "docs"},
	"amp":                {Key: true, Why: "Its Custom URL connection is made in its own command, and isn't documented to go per session.", Checked: "docs"},
	"kilo":               {Key: true, Why: "KILO_CONFIG_DIR and its data folder could give another account; enowX doesn't drive them yet.", Checked: "docs"},
	"crush":              {Key: true, Why: "A config's sessions read its settings alone, not your own crush.json: CRUSH_GLOBAL_CONFIG replaces that file rather than adding to it. A model picked in a project's .crush folder can still win.", Checked: "docs"},
	"aug":                {Why: "It uses Augment's models and documents no address or key for your API.", Checked: "docs"},
	"autohand":           {Key: true, Why: "AUTOHAND_AI_BASE_URL is for Autohand's own provider and models, with its sign-in. Another provider's key is a plain value of its config file, which enowX doesn't write a key to. No other account: AUTOHAND_HOME moves its folder, but its token is in that same file, with all its settings.", Checked: "docs"},
	"cline":              {Key: true, Why: "cline auth takes the key on its command line, which enowX doesn't do. No other account: CLINE_DIR moves its folder, but its keys are in data/settings/providers.json, with its provider settings.", Checked: "docs"},
	"codebuff":           {Key: true, Why: "Its user docs don't cover a model API of your own.", Checked: "docs"},
	"freebuff":           {Why: "It runs free on its own models, with no option for your own API or key.", Checked: "help"},
	"command-code":       {Key: true, Why: "providers.json takes a model API, and no per-session way is documented; enowX doesn't write it yet.", Checked: "help"},
	"continue":           {Key: true, Why: "No other account: its docs document neither CONTINUE_GLOBAL_DIR nor where cn login keeps its sign-in.", Checked: "docs"},
	"cursor":             {Key: true, Why: "It runs through Cursor's backend: the only provider of your own documented is AWS Bedrock.", Checked: "help"},
	"droid":              {Key: true, Why: "Its --settings isn't documented to carry a model; enowX writes its settings.", Checked: "help"},
	"kimi":               {Key: true, Why: "Another account is read from its docs, not signed in: credentials/ in KIMI_CODE_HOME is the account's. It has no sign-in command: use /login in a session on the config. KIMI_MODEL_* takes one model, by OpenAI's or Anthropic's API, and needs a key.", Checked: "docs"},
	"mistral-vibe":       {Key: true, Why: "Another account is read from its docs, not signed in: .env in VIBE_HOME holds the account's key, asked for on its first run, and a MISTRAL_API_KEY in the environment may win over it. A model API in its own settings keeps its key in the same .env, so an account doesn't have it.", Checked: "docs"},
	"qwen-code":          {Key: true, Why: "Its variables take one model, by OpenAI's chat API or Anthropic's: the Responses API, several models and their limits are only in its settings. No other account: its auth docs don't name QWEN_HOME, keep keys in settings.json or .env with its other settings, and name no file for a browser sign-in.", Checked: "docs"},
	"rovo":               {Why: "It uses Atlassian's models and credits, and documents no API of your own.", Checked: "docs"},
	"hermes":             {Key: true, Why: "No other account: HERMES_HOME moves its folder and auth.json holds its sign-ins, but its docs have a profile read the root's login until a sign-in made by profile name, and write refreshes back to it. config.yaml holds a model API; enowX doesn't write it yet.", Checked: "docs"},
	"devin":              {Why: "Only Devin's own models are documented, air-gapped builds aside.", Checked: "docs"},
	"openclaw":           {Key: true, Why: "OPENCLAW_CONFIG_PATH replaces its whole config, and its terminal normally talks to a Gateway that reads its own, so a session's variables wouldn't reach the model. No other account: OPENCLAW_HOME stands for the whole home, its sign-ins deep in agents/<id>/agent/auth-profiles.json, and the Gateway service doesn't follow it.", Checked: "docs"},
	"codebuddy":          {Key: true, Why: "CODEBUDDY_BASE_URL and its key do work per session, but run here it wrote the key into its own logs, so enowX doesn't send it that way. No other account: CODEBUDDY_CONFIG_DIR moves its folder, but its docs name no file or keychain item for the sign-in.", Checked: "run"},
	"jcode":              {Key: true, Why: "JCODE_OPENAI_API_BASE and OPENAI_API_KEY exist, but its sessions share one server: they don't reach a server already running, and one a session starts keeps the key after it ends. Its own sign-in also wins over OPENAI_API_KEY. No other account: JCODE_HOME isn't in its README, and that server wouldn't follow it.", Checked: "docs"},
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
