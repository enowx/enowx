# Model APIs

A model API of the user's (a company gateway, OpenRouter, LiteLLM, a local server) is a **config** of an agent for the agents that can take one per session: Claude Code, Codex, OpenCode, Gemini CLI, Antigravity, GitHub Copilot, OpenClaude, Aider, Kilo Code, Qwen Code, Goose, Kimi, Continue, Crush, OMP. Their own settings are never written; a session started on the config gets the API through its environment and command line ([configs.md](configs.md)). `agentapi.set` refuses them, and a model API set in their files before becomes a config.

The other agents below take one only in their own settings. For them, their page in **Settings → Agents** writes the API's address, key and model into the agent's own settings, the way that agent takes them (`core/agentapi`), so its sessions use it, and keeps the files as they were first, to put them back. Agents do the same from the command line (`enowx-app api`) and over MCP (`agent_api_*`). What follows is how each agent takes an API in its files, which is also what a config gives it per session where it can; for the agents marked *moved*, enowX no longer writes those files: they are where it finds an API set before, to make it a config. [configs.md](configs.md#applied-per-session) has what each gets per session.

## What each agent gets

What enowX adds is named `enowx`, so it finds it again and leaves the user's own providers alone. Agents that read their key from the environment only get it in the sessions enowX starts of them, from `~/.enowx-app/agentapi/<agent>.env` (readable by the user only).

| Agent | Protocols | Where | Source |
|---|---|---|---|
| Claude Code (and Agent Teams) | Anthropic | `~/.claude/settings.json` `env`: `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_MODEL`, `ANTHROPIC_DEFAULT_HAIKU_MODEL` (quick work). A key set wins over the claude.ai sign-in, which stays saved | code.claude.com/docs/en/llm-gateway-connect |
| Codex | OpenAI Responses | `~/.codex/config.toml`: `model_provider = "enowx"`, `model`, `[model_providers.enowx]` (`base_url`, `env_key`, `wire_api = "responses"`); the key in `ENOWX_CODEX_API_KEY` of its sessions | Codex's config reference |
| OpenCode, Kilo Code (*moved*) | OpenAI chat, OpenAI Responses, Anthropic | `~/.config/opencode/opencode.json`, `~/.config/kilo/kilo.json`: `provider.enowx` (`npm` by protocol, `options.baseURL`, `options.apiKey`, `models`), `model = "enowx/<model>"` | opencode.ai/docs/providers; kilo.ai/docs/code-with-ai/platforms/cli |
| Gemini CLI | Gemini | `~/.gemini/.env` (`GOOGLE_GEMINI_BASE_URL`, `GEMINI_API_KEY`), `~/.gemini/settings.json` (`security.auth.selectedType = "gemini-api-key"`, `model.name`). The address is https unless it's localhost | geminicli.com/docs/reference/configuration |
| Antigravity | Gemini | its sessions' `GOOGLE_GEMINI_BASE_URL`, `GEMINI_API_KEY`, `--model` | antigravity.google/docs; `agy --help` |
| Qwen Code (*moved*) | OpenAI chat, Anthropic | `~/.qwen/settings.json`: an entry of `modelProviders.openai` or `.anthropic` (`envKey`, `baseUrl`, `wireApi`), the key in its `env` block, `security.auth.selectedType`, `model.name` | qwenlm.github.io/qwen-code-docs/en/users/configuration/auth |
| Droid | OpenAI chat, OpenAI Responses, Anthropic | `~/.factory/settings.json`: a `customModels` entry "enowX: <model>", to pick in `/model` | docs.factory.com/cli/byok |
| Goose (*moved*) | OpenAI chat, Anthropic | `~/.config/goose/custom_providers/enowx.json`, `GOOSE_PROVIDER`/`GOOSE_MODEL` of `~/.config/goose/config.yaml`; the key in `ENOWX_GOOSE_API_KEY` of its sessions (Goose ignores keys in its config) | block.github.io/goose/docs/guides/environment-variables |
| Aider | OpenAI chat | `~/.aider.conf.yml`: `openai-api-base`, `openai-api-key`, `model: openai/<model>` | aider.chat/docs/llms/openai-compat |
| Kimi (*moved*) | OpenAI chat, Anthropic | `~/.kimi-code/config.toml` (what enowX wrote in `~/.kimi/config.toml` before is still found): `[providers.enowx]`, `[models.enowx-<model>]`, `default_model` | kimi.com/code/docs/en/kimi-code-cli/configuration/config-files.html, env-vars.html |
| Mistral Vibe | OpenAI chat, OpenAI Responses, Anthropic | `~/.vibe/config.toml`: `[[providers]]` and `[[models]]` named `enowx`, `active_model`; the key in `~/.vibe/.env` | Vibe's configuration reference |
| GitHub Copilot | OpenAI chat, OpenAI Responses, Anthropic | its sessions' `COPILOT_PROVIDER_*`, `COPILOT_MODEL`, and `COPILOT_PROVIDERS_CONFIG` to an empty file, so the user's `providers.json` doesn't win over them | docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-byok-models |
| OpenClaude | OpenAI chat, Anthropic | its sessions' `CLAUDE_CODE_USE_OPENAI`, `OPENAI_*`, or `ANTHROPIC_*` | openclaude's advanced setup |
| Continue (*moved*) | OpenAI chat, Anthropic, Gemini | a config of enowX's, `~/.enowx-app/agentapi/continue.yaml`, given to its sessions (`--config`); the user's own untouched. A session's own holds `${{ secrets.ENOWX_CONTINUE_API_KEY }}`, not the key | docs.continue.dev/cli/configuration |
| Crush | OpenAI chat, Anthropic | its sessions' `CRUSH_GLOBAL_CONFIG` and `CRUSH_GLOBAL_DATA`, folders of the session's own: `crush.json` with `providers.enowx` (`type`, `base_url`, `api_key: $ENOWX_CRUSH_API_KEY`, `models`), `models.large`, `models.small` | github.com/charmbracelet/crush (README, `schema.json`, `internal/config/load.go`) |
| Pi | OpenAI chat, OpenAI Responses, Anthropic, Gemini | `~/.pi/agent/models.json` `providers.enowx`, `defaultProvider`/`defaultModel` of `~/.pi/agent/settings.json` | pi-mono's models docs |
| OMP | OpenAI chat, OpenAI Responses, Anthropic | `models.yml` in its agent folder (`PI_CODING_AGENT_DIR`): `providers.enowx` (`baseUrl`, `api`, `apiKey`, `models`), the model by `--model enowx/<model>` | github.com/can1357/oh-my-pi (`pi-utils/src/dirs.ts`, `pi-coding-agent/src/config/model-registry.ts`, `resolve-config-value.ts`) |

The others take no model API from enowX for the reason [configs.md](configs.md#every-agent) gives each; Settings → Agents says it on their page.

## Models and their options

An API can carry several models, the first the default, and for each what the agent takes of it: whether it reads images, its context window, the most it writes at once, its reasoning effort. The dialog, `enowx-app api show` and `agent_api_list` show what each agent takes; what it doesn't is left out. Unset values leave it to the agent.

| Agent | Several | Vision | Context | Output | Reasoning effort |
|---|---|---|---|---|---|
| Claude Code | `modelPicker.options` (/model) | — | `CLAUDE_CODE_MAX_CONTEXT_TOKENS`, one for all | `CLAUDE_CODE_MAX_OUTPUT_TOKENS`, one for all | per model, `modelSettings.<id>.effortLevel`: low, medium, high, xhigh |
| Codex | profiles `enowx-<model>` (`codex --profile`) | — | `model_context_window`, the default's | — | per model, `model_reasoning_effort`: none, minimal, low, medium, high, xhigh |
| OpenCode, Kilo Code | `models` map, `small_model` | `modalities`, `attachment` | `limit.context` | `limit.output` | per model, `options.reasoningEffort`: low, medium, high |
| Qwen Code | — | — | — | — | — |
| Droid | `customModels` | `noImageSupport` | `maxContextLimit` | `maxOutputTokens` | per model, `reasoningEffort`: off, low, medium, high |
| Goose | — | — | `GOOSE_CONTEXT_LIMIT` | `GOOSE_MAX_TOKENS` | — |
| Crush | — | — | `context_window` | `default_max_tokens` | — |
| Aider | the model and its weak model | — | `.aider.model.metadata.json` `max_input_tokens` | `max_output_tokens` | `reasoning-effort`, one for all: low, medium, high |
| Kimi | — | `KIMI_MODEL_CAPABILITIES` `image_in` | `KIMI_MODEL_MAX_CONTEXT_SIZE` | — | — |
| Mistral Vibe | `[[models]]` | `supports_images` | `max_context_length` | `max_output_tokens` | per model, `thinking`: off, low, medium, high, max |
| Pi | provider `models` | `input` | `contextWindow` | `maxTokens` | per model, `modelThinkingLevels`: off, minimal, low, medium, high, xhigh, max |
| OMP | `models` | `input` | `contextWindow` | `maxTokens` | — |
| Continue | `models` by role | `capabilities` `image_input` | `contextLength` | `maxTokens` | — |
| GitHub Copilot | — | — | `COPILOT_PROVIDER_MAX_PROMPT_TOKENS` | `COPILOT_PROVIDER_MAX_OUTPUT_TOKENS` | — |
| Gemini CLI, Antigravity, OpenClaude | — | — | — | — | — |

Antigravity's model is optional: with none, it uses its own choice.

Qwen Code, Goose and Kimi take one model per session, as their variables hold one: an API set in their files with several keeps the first when it becomes a config.

## Backups

Before each change, enowX copies the files it is about to change into `~/.enowx-app/agentapi/<agent>/<time>/`, noting the files that weren't there. The first backup, the files as the user had them, is kept for good; of the others, the last ten. **Restore** puts a backup's files back byte for byte and removes those that weren't there, after backing up the files as they are, so a restore can be undone too. **Back to its own API** removes only what enowX added. Comments in a JSON settings file are dropped when enowX writes it; the backup keeps them.

## Keys

A key goes from the app to the host's daemon, which writes it where the agent reads it, and never comes back whole: the daemon answers with its last four characters. Changing an API without typing a key keeps the key it has. On the command line, `--key-env VAR` or `--key-stdin` keep it out of the shell's history.

## Command line and protocol

`enowx-app api` lists the agents and what each is set to; `api show AGENT`; `api set AGENT --url URL [--key K|--key-env VAR|--key-stdin] [--model ID[,vision][,context=N][,output=N][,effort=E]]... [--small-model M] [--effort E] [--protocol P]` (several `--model`, the first the default); `api clear AGENT`; `api restore AGENT [BACKUP]`. MCP: `agent_api_list`, `agent_api_set`, `agent_api_clear`, `agent_api_restore`. Methods `agentapi.list`, `agentapi.set`, `agentapi.clear`, `agentapi.restore` (`proto/agentapi.go`).
