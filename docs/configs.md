# Agent configs

An agent's **configs** are what its sessions can run with. The **official** config is the agent as installed: its own sign-in and settings, which enowX never writes to. The others are saved apart and picked per session:

- **Another account**: the agent signed in with another account, in a home of its own ([accounts.md](accounts.md)). Claude Code, Claude Agent Teams through it, and Codex.
- **An API key**: a key of the vault lent to its sessions as the vault lends credentials, the agent's own home otherwise.
- **A model API**: a gateway, OpenRouter, LiteLLM, a local server: its address, how it is spoken, its models (with their context, output and effort where the agent takes them), a model for quick work, and its key as the name of a credential of the vault.

Every config also has a name, a badge color, an effort (the agent's own effort flag), and variables and flags added to its sessions.

## Where they are

- **Settings → Agents**: every agent, installed ones first, with search: a page per agent saying how it takes a config, or why it can't, and its configs, the official one first by default, in the order sessions use them. **New config** makes one in a dialog; a config's menu edits, duplicates, signs in (an account, on this host), makes it the default for new sessions, keeps it to some projects, or removes it after asking. None of these change the official config.
- **New session** picks **Official** or a config; by default the one the preferences name ([accounts.md](accounts.md)).
- A session's row shows its config's badge; its menu switches it to another config, its conversation picked up.
- **Command line**: `enowx-app configs`, `spawn --config NAME` ([cli.md](cli.md)). MCP: `configs_list`, `configs_add`, `configs_edit`, `configs_remove`, and `session_spawn`'s `config` (feature **Agent configs**).

Configs are in the synced settings (`settings/configs.json`, [user-data-sync.md](user-data-sync.md)), so every host of the user's has them. Keys stay in the vault and are read on the host that starts the session. An account's sign-in stays on the host it was made on: signed in once per host, from the config's menu.

## Applied per session

A config is applied each time its session's agent starts (created, continued, after hibernation or a restart of the daemon, switched to): through the session's environment and command line. The agent's own files are never written; two sessions of the same agent run on two configs at once.

A model API's key is read from the vault as the session starts (the app holding the vault answers), kept in the daemon's memory for the session, masked like what the vault lends, and never written to disk nor put on a command line. With no app to lend it, the session doesn't start; one restored when the daemon starts waits, stopped, to be continued.

| Agent | How it takes a model API per session |
|---|---|
| Claude Code (and Agent Teams) | `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN` (the key), `ANTHROPIC_MODEL`, `ANTHROPIC_DEFAULT_HAIKU_MODEL`, `CLAUDE_CODE_MAX_CONTEXT_TOKENS`, `CLAUDE_CODE_MAX_OUTPUT_TOKENS` in its environment; the same but the key, the model picker's options and each model's effort in the `--settings` enowX's hooks already pass (flag settings win over the user's own) |
| Codex | `-c` overrides: `model_provider="enowx"`, `model_providers.enowx.*` (`base_url`, `env_key`, `wire_api = "responses"`), `model`, `model_reasoning_effort`, `model_context_window`, a profile `enowx-<model>` for each other model; the key in `ENOWX_CODEX_API_KEY` |
| OpenCode | `OPENCODE_CONFIG`: a config of the session's own merged over the user's, its provider `enowx` with `apiKey: {env:ENOWX_OPENCODE_API_KEY}`, the key in that variable |
| Gemini CLI | `GOOGLE_GEMINI_BASE_URL` (https, unless it's localhost, 127.0.0.1 or [::1]: a config is refused otherwise, as the CLI would), `GEMINI_API_KEY`, `GEMINI_CLI_AUTH_OVERRIDE=gemini-api-key` (the key's sign-in for this run, the user's settings untouched), `GEMINI_MODEL` and `--model` |
| Antigravity | `GOOGLE_GEMINI_BASE_URL`, `GEMINI_API_KEY`; `--model`, optional |
| GitHub Copilot | `COPILOT_PROVIDER_*`, `COPILOT_MODEL`; `COPILOT_PROVIDERS_CONFIG` to an empty file of the session's own, as the user's `providers.json` wins over those variables when it lists anything |
| OpenClaude | `CLAUDE_CODE_USE_OPENAI`, `OPENAI_*`, or `ANTHROPIC_*` |
| Aider | `OPENAI_API_BASE`, `OPENAI_API_KEY`; `--model openai/<model>`, `--weak-model`, `--reasoning-effort`, `--model-metadata-file` (a file of the session's own) for its limits |
| Kilo Code | `KILO_CONFIG_CONTENT`: a config merged over the user's and a project's, its provider `enowx` with `apiKey: {env:ENOWX_KILO_API_KEY}`, the key in that variable |
| Qwen Code | `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_MODEL`, or `ANTHROPIC_*`; `--auth-type` and `--model`, which win over the user's settings |
| Goose | `GOOSE_PROVIDER`, `GOOSE_MODEL`, `GOOSE_PROVIDER__TYPE`, `GOOSE_PROVIDER__HOST`, `GOOSE_PROVIDER__API_KEY`; `OPENAI_BASE_URL` and `OPENAI_API_KEY`, or `ANTHROPIC_HOST` and `ANTHROPIC_API_KEY`; `GOOSE_CONTEXT_LIMIT`, `GOOSE_MAX_TOKENS` |
| Kimi | `KIMI_MODEL_NAME`, `KIMI_MODEL_API_KEY`, `KIMI_MODEL_BASE_URL`, `KIMI_MODEL_PROVIDER_TYPE`, `KIMI_MODEL_MAX_CONTEXT_SIZE`, `KIMI_MODEL_CAPABILITIES` |
| Continue | `--config` to a file of the session's own, with `apiKey: ${{ secrets.ENOWX_CONTINUE_API_KEY }}`, the key in that variable |
| Crush | `CRUSH_GLOBAL_CONFIG` and `CRUSH_GLOBAL_DATA` to folders of the session's own, its `crush.json` with `api_key: $ENOWX_CRUSH_API_KEY`, the key in that variable. Those sessions don't read the user's own `crush.json` |

The other agents [agent-api.md](agent-api.md) lists (Droid, Mistral Vibe, Pi) take a model API only in their own settings: their page in Settings → Agents still sets one there, backed up first, as before.

## What reads a session's config

- **Falling back at a limit**: the order and fallback of [accounts.md](accounts.md) are across all of an agent's configs, a model API's included.
- **The judge's policy** "the session's agent's provider" ([decide.md](decide.md)) reads the session's config's provider (from its API's address), not the agent's.
- **`SessionInfo.Config`** has the config's ID, name, kind, badge, provider, address and model, for what shows the session and token tracking.

## Moving over what was set before

- Accounts kept in `accounts.json` become configs of the account kind, their homes where they were; a team member's account on a host they visit stays that host's own.
- A model API set in an agent's own files (before configs) becomes a config named "Model API" once its key is kept in the vault: the daemon asks the app (the user confirms) when it starts and whenever an app connects. The config becomes the agent's default for new sessions, so they go on as before, and the agent's files are put back as the user had them (the first backup enowX kept), byte for byte, when nothing else changed them since; otherwise only what enowX added is taken out, the user's later changes kept. Until then the API keeps working from the files.

## Checked

- **Claude Code and Codex:** tested with stand-in programs that record the variables and arguments each session gets (`daemon/configs_test.go`).
- **OpenCode:** run for real, `OPENCODE_REAL=1 go test ./core/agentapi -run OpenCode`. A session config's `OPENCODE_CONFIG` made it call the config's API, with the key from its environment and the model named; the key wasn't written to the file.
- **Gemini CLI:** checked against its source. `GEMINI_CLI_AUTH_OVERRIDE`, `GOOGLE_GEMINI_BASE_URL`, `GEMINI_API_KEY`, `GEMINI_MODEL` and `--model` are what it reads.
- **Aider:** checked against its docs. `OPENAI_API_BASE`, `OPENAI_API_KEY`, `--model openai/<model>` and `--model-metadata-file` (a JSON keyed by `provider/model`).
- **Kilo Code, Qwen Code, Goose, Kimi, Continue, Crush, GitHub Copilot's providers file:** read from their docs and source, none run: tested only for what a session gets, the key in no file or argument (`core/agentapi/session_routes_test.go`). Each needs a real run before it's relied on.
- **Antigravity:** `--model` is in `agy --help`. Whether it takes a model named by your API wasn't run.

## Every agent

What each agent's CLI or docs offered on 2026-11-01 or 02, beside what enowX drives (`core/accounts/ways.go`, the list Settings → Agents, `configs` and `agent_api_list` read; a test fails when a new agent isn't in it). **Checked** is `run` (here), `help` (its `--help`, not run with a config) or `docs` (read, not run). "Could" is a route its docs offer that enowX doesn't drive yet.

| Agent | Takes | Checked | Why not more |
|---|---|---|---|
| Claude Code | another account, a model API per session, an API key | run |  |
| Codex | another account, a model API per session, an API key | run |  |
| OpenCode | a model API per session, an API key | run | No separate account: it has no documented folder for one sign-in alone. |
| Gemini CLI | a model API per session, an API key | docs | GEMINI_CLI_HOME could give another account; enowX doesn't drive it yet. Its API's address is https unless it's on localhost. |
| Kiro CLI | an API key | help | It uses its own models and documents no address for your API. KIRO_HOME could give another account; enowX doesn't drive it yet. |
| Claude Agent Teams | no config | help | Its sessions are Claude Code's, so they use Claude Code's configs. |
| OpenClaude | a model API per session, an API key | docs | OPENCLAUDE_CONFIG_DIR could give another account; enowX doesn't drive it yet. |
| Grok | an API key | docs | Its model API is in ~/.grok/config.toml, from a third party's docs: x.ai's couldn't be read. enowX doesn't write it. |
| GitHub Copilot | a model API per session, an API key | docs | COPILOT_HOME could give another account; enowX doesn't drive it yet. |
| OpenCode 2 | an API key | docs | Only opencode.jsonc takes a model API; enowX doesn't write it yet. |
| MiMo Code | an API key | docs | MIMOCODE_HOME moves all of its settings, and its own settings hold the API; enowX drives neither yet. |
| Ante | an API key | docs | ANTE_HOME moves all of its settings, and catalog.json holds the API; enowX drives neither yet. |
| Trae | an API key | docs | Its 2.0 settings for a model API aren't documented. |
| Muse | an API key | help | Its --base-url is for Meta's provider alone: no API of your own is documented. |
| DeepSeek Harness | an API key | docs | DSH_HOME and settings.yaml are documented by a third party alone: its own site couldn't be read. |
| Qoder CLI | an API key | docs | A model is added only in its /model screen: its docs say not to edit the settings by hand. |
| Qoder CLI China | an API key | docs | A model is added only in its /model screen: its docs say not to edit the settings by hand. |
| ZCode | no config | docs | Its docs describe a desktop app, with no command line to give a config to. |
| Pi | a model API in its own settings, an API key | docs | PI_CODING_AGENT_DIR could give another account; enowX doesn't drive it yet. |
| OMP | an API key | run | It takes a provider's address, but only for models named in models.yml, which enowX doesn't write yet: a model of your own wasn't found when run. |
| Prime Agent | an API key | docs | Only a models.json takes a model API, at a place its docs don't name. |
| Antigravity | a model API per session, an API key | help | No separate account is documented. |
| Aider | a model API per session, an API key | docs | No account: it has no sign-in to separate, only API keys. |
| Goose | a model API per session, an API key | docs | GOOSE_PATH_ROOT could give another account; enowX doesn't drive it yet. Its variables take one model, with no effort documented. |
| Amp | an API key | docs | Its Custom URL connection is made in its own command, and isn't documented to go per session. |
| Kilo Code | a model API per session, an API key | docs | KILO_CONFIG_DIR and its data folder could give another account; enowX doesn't drive them yet. |
| Crush | a model API per session, an API key | docs | A config's sessions read its settings alone, not your own crush.json: CRUSH_GLOBAL_CONFIG replaces that file rather than adding to it. A model picked in a project's .crush folder can still win. |
| Auggie | no config | docs | It uses Augment's models and documents no address or key for your API. |
| Autohand Code | an API key | docs | AUTOHAND_AI_BASE_URL is for Autohand's own provider and models, with its sign-in. Another provider's key is a plain value of its config file, which enowX doesn't write a key to. |
| Cline | an API key | docs | cline auth takes the key on its command line, which enowX doesn't do. |
| Codebuff | an API key | docs | Its user docs don't cover a model API of your own. |
| Freebuff | no config | help | It runs free on its own models, with no option for your own API or key. |
| Command Code | an API key | help | providers.json takes a model API, and no per-session way is documented; enowX doesn't write it yet. |
| Continue | a model API per session, an API key | docs | Another account wasn't checked: it needs a real sign-in to test. |
| Cursor | an API key | help | It runs through Cursor's backend: the only provider of your own documented is AWS Bedrock. |
| Droid | a model API in its own settings, an API key | help | Its --settings isn't documented to carry a model; enowX writes its settings. |
| Kimi | a model API per session, an API key | docs | KIMI_CODE_HOME could give another account; enowX doesn't drive it yet. KIMI_MODEL_* takes one model, by OpenAI's or Anthropic's API, and needs a key. |
| Mistral Vibe | a model API in its own settings, an API key | docs | VIBE_HOME moves all of its settings; enowX writes them for now. |
| Qwen Code | a model API per session, an API key | docs | Its variables take one model, by OpenAI's chat API or Anthropic's: the Responses API, several models and their limits are only in its settings. |
| Rovo Dev | no config | docs | It uses Atlassian's models and credits, and documents no API of your own. |
| Hermes | an API key | docs | HERMES_HOME could give another account and config.yaml a model API; enowX drives neither yet. |
| Devin | no config | docs | Only Devin's own models are documented, air-gapped builds aside. |
| OpenClaw | an API key | docs | OPENCLAW_CONFIG_PATH replaces its whole config, and its terminal normally talks to a Gateway that reads its own, so a session's variables wouldn't reach the model. |
| CodeBuddy | an API key | run | CODEBUDDY_BASE_URL and its key do work per session, but run here it wrote the key into its own logs, so enowX doesn't send it that way. |
| Jcode | an API key | docs | JCODE_OPENAI_API_BASE and OPENAI_API_KEY exist, but its sessions share one server: they don't reach a server already running, and one a session starts keeps the key after it ends. Its own sign-in also wins over OPENAI_API_KEY. |

Run for real on this Mac: OpenCode and CodeBuddy against a local stand-in API. CodeBuddy took its address and key from the environment and answered through it, but wrote the key into its own logs, so enowX doesn't send it that way. OMP refused a model outside its `models.yml`.

Not checked: x.ai's and DeepSeek Harness's own sites couldn't be read, so Grok and DeepSeek Harness rest on third parties' docs. GitHub's docs imply an empty `providers.json` leaves its variables working, without saying so. Goose's docs name `GOOSE_PROVIDER__*`, which weren't found in the source of its OpenAI and Anthropic providers, so a session gets those providers' own variables too. Codex's `--profile` now layers a file, which may not select `-c profiles.*`. These wait for those agents to be installed here.
