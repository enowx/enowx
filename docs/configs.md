# Agent configs

An agent's **configs** are what its sessions can run with. The **official** config is the agent as installed: its own sign-in and settings, which enowX never writes to. The others are saved apart and picked per session:

- **Another account**: the agent signed in with another account, in a home of its own ([accounts.md](accounts.md)). Claude Code and Codex.
- **An API key**: a key of the vault lent to its sessions as the vault lends credentials, the agent's own home otherwise.
- **A model API**: a gateway, OpenRouter, LiteLLM, a local server: its address, how it is spoken, its models (with their context, output and effort where the agent takes them), a model for quick work, and its key as the name of a credential of the vault.

Every config also has a name, a badge color, an effort (the agent's own effort flag), and variables and flags added to its sessions.

## Where they are

- **Settings → Agents**: a page per agent with its configs, the official one first by default, in the order sessions use them. **New config** makes one in a dialog; a config's menu edits, duplicates, signs in (an account, on this host), makes it the default for new sessions, keeps it to some projects, or removes it after asking. None of these change the official config.
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
| Gemini CLI, Antigravity | `GOOGLE_GEMINI_BASE_URL`, `GEMINI_API_KEY`, `GEMINI_DEFAULT_AUTH_TYPE=gemini-api-key`, `GEMINI_MODEL` and `--model` (Gemini CLI) |
| GitHub Copilot | `COPILOT_PROVIDER_*`, `COPILOT_MODEL` |
| OpenClaude | `CLAUDE_CODE_USE_OPENAI`, `OPENAI_*`, or `ANTHROPIC_*` |
| Aider | `OPENAI_API_BASE`, `OPENAI_API_KEY`; `--model openai/<model>`, `--weak-model`, `--reasoning-effort`, `--model-metadata-file` (a file of the session's own) for its limits |

The other agents [agent-api.md](agent-api.md) lists (Kilo Code, Qwen Code, Droid, Goose, Kimi, Mistral Vibe, Continue, Pi) take a model API only in their own settings: their page in Settings → Agents still sets one there, backed up first, as before.

## What reads a session's config

- **Falling back at a limit**: the order and fallback of [accounts.md](accounts.md) are across all of an agent's configs, a model API's included.
- **The judge's policy** "the session's agent's provider" ([decide.md](decide.md)) reads the session's config's provider (from its API's address), not the agent's.
- **`SessionInfo.Config`** has the config's ID, name, kind, badge, provider, address and model, for what shows the session and token tracking.

## Moving over what was set before

- Accounts kept in `accounts.json` become configs of the account kind, their homes where they were; a team member's account on a host they visit stays that host's own.
- A model API set in an agent's own files (before configs) becomes a config named "Model API" once its key is kept in the vault: the daemon asks the app (the user confirms) when it starts and whenever an app connects. The config becomes the agent's default for new sessions, so they go on as before, and the agent's files are put back as the user had them (the first backup enowX kept), byte for byte, when nothing else changed them since; otherwise only what enowX added is taken out, the user's later changes kept. Until then the API keeps working from the files.
