# Token usage

enowX tracks the tokens every agent uses on each host: input, cache read, cache write, output and reasoning, per session, project, agent, config, model and host, with costs, budgets and alerts. It is **off** until turned on (Resource usage → Tokens, or `tokens.setSettings`); turning it on reads back the records already on disk (30 days by default).

Only counts are kept, never prompts or answers. Nothing leaves the host but the totals the app asks for, and each host's daily totals written to the synced settings (below). No telemetry.

## Layers

Each agent is tracked by the best layer it allows, and the app says which (Tokens → Coverage, `enowx usage coverage`):

1. **Its records.** The agent's own files on disk, read by an adapter each (`core/tokens/adapt_*.go`).
2. **Its traffic.** For agents that keep nothing: with **Meter agents that keep no records** on, a new session of such an agent starts with the host's interceptor as its proxy (a target of the session, shown in Traffic), and each model API answer is read for its `usage`, streamed or not: Anthropic's Messages (`message_start`, `message_delta`), OpenAI's chat completions (the last chunk with `usage`) and Responses (`response.completed`), Gemini's `usageMetadata`, OpenRouter and OpenAI-compatible servers (any host, by path). It reads what the interceptor records: while recording is paused, nothing is metered.
3. **Its screen.** With **Read the counts agents print** on, the screens of running agents that have no records and aren't metered are read for the lines agents print their counts in. Only Aider's line is known (`Tokens: 4.4k sent, … received`); no other agent here prints per-call counts.
4. **None.** Shown as not tracked, never guessed.

## Coverage

"Checked" means checked against a real install on the developer's machine (October 2026); the others against the agents' documentation and source (for closed-source agents, open-source usage trackers that parse them), with fixtures written from that format.

| Agent | Layer | What is counted | Where | Checked |
|---|---|---|---|---|
| Claude Code, Claude Agent Teams | records | each message: input, cache read, cache write, output, thinking; model (subagents too) | `~/.claude/projects/<folder>/*.jsonl` | yes |
| Codex | records | each call's `token_count` (`last_token_usage`): input, cached, cache write, output, reasoning; model; the plan | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | yes |
| OpenCode | records | each model call (a `step-finish` part; the message holds only its last step): input, output, reasoning, cache read, cache write, cost; provider, model | `~/.local/share/opencode/opencode.db`, before 1.x `storage/part/` | yes |
| CodeBuddy | records | each message: input, cache read, cache write, output, reasoning; model | `~/.codebuddy/projects/<folder>/*.jsonl` | yes |
| Kilo Code, MiMo Code, OpenCode 2 | records | as OpenCode (OpenCode 2's folder unconfirmed) | `~/.local/share/{kilo,mimocode,opencode2}/*.db` | no |
| Pi, OMP, Prime Agent | records | each assistant message: input, output, cache read, cache write, cost; provider, model | `~/.pi/agent/sessions`, `~/.omp/agent/sessions`, `~/.prime/agent/sessions` | no (Pi installed, no answers recorded yet) |
| OpenClaw | records | Pi's format, in SQLite (zstd rows read) or JSON lines | `~/.openclaw/agents/<a>/agent/openclaw-agent.sqlite` | no |
| OpenClaude, ZCode, Qoder CLI | records | Claude Code's shape (ZCode's and Qoder's usage fields unconfirmed) | `~/.openclaude/projects`, `~/.zcode/projects`, `~/.qoder/projects` | no |
| Gemini CLI | records | each message: input, cached, output, thoughts, tool; model | `~/.gemini/tmp/<project>/chats/session-*.jsonl` (older `.json`) | no |
| Qwen Code | records | each message's `usageMetadata`; model | `~/.qwen/projects/<folder>/chats/*.jsonl` | no |
| Goose | records | each call (`usage_ledger`): input, output, cache read, cache write, cost; model | `~/.local/share/goose/sessions/sessions.db` | no |
| GitHub Copilot CLI | records | each call (`assistant_usage_events`): input, output, cache read, cache write, reasoning | `~/.copilot/session-store.db` | no (installed, no store yet) |
| Devin CLI | records | each message's metrics; model | `~/.local/share/devin/cli/sessions.db` | no |
| Grok | records | Grok CLI: each call's input, output, cost; Grok Build: each inference's prompt, cached, completion, reasoning | `~/.grok/grok.db`, `~/.grok/logs/unified.jsonl` | no |
| Cline | records | each message's metrics (older: `api_req_started`) | `~/.cline/data/sessions/`, `~/.cline/data/tasks/` | no |
| Continue (`cn`) | records | each answer's prompt, completion, cached, cache write; no time (the session's last change) | `~/.continue/sessions/<id>.json` | no |
| Amp | records | older builds only (threads are now on Amp's servers) | `~/.local/share/amp/threads/T-*.json` | no |
| Auggie | records | each finished turn's last usage | `~/.augment/sessions/<id>.json` | no |
| Kiro CLI | records | each turn's counts, when Kiro fills them (often zeros: it meters credits) | `~/.kiro/sessions/cli/<id>.json` | installed: the files here held no counts |
| Jcode | records | each message's usage | `~/.jcode/sessions/session_*.json` | no |
| Kimi CLI | records | each answer's input, output, cache read, cache write; no model | `~/.kimi/sessions/<md5>/<id>/wire.jsonl` | no |
| Mistral Vibe | records | Vibe 3's `completion_succeeded` usage (Vibe 2 keeps only session totals) | `~/.vibe/logs/session/unified/<id>/journal/` | no |
| Muse | records | `model_completed` events, where its version writes them | `~/.local/share/muse/sessions/` | installed: its version wrote none |
| Command Code | records | each message: input, output, cache read, cache write, cost; model | `~/.commandcode/projects/<folder>/*.jsonl` | no |
| DeepSeek Harness | records | each answer's input, output, cache read; model (zstd read) | `~/.dsh/sessions/<folder>/<id>/session.jsonl(.zstd)` | no |
| Aider | records | each answer's `Tokens:` line, rounded as printed, and its cost | `<project>/.aider.chat.history.md` | no |
| Crush, Hermes, Droid, Autohand | traffic | their records keep only a session's totals (Crush: its last call) | | |
| Rovo Dev, Ante, Qoder CLI China | traffic | records undocumented or unknown | | |
| Cursor, Trae, Antigravity, Codebuff, Freebuff | none | they talk to their maker's own service, not a model API whose answers can be read; their usage is in their accounts | | |

Any agent added later without an adapter falls to the traffic layer.

What "input" means differs between agents (some count cached tokens in it, some don't); each adapter turns it into input without the cache, cache read, cache write, and output with reasoning in. Where an agent's records don't say (Cline, Continue, Jcode, Muse, DeepSeek Harness), a count smaller than the cache read is taken as without it.

## Recording

- **One record per model call** (`core/tokens.Call`): when; the project, session, agent, account, config, provider and model; the counts; the layer; for enowX's own use, which.
- **Tied to its session**: the same agent's session run in the call's folder, started before it (the latest). Calls made outside enowX get the project their folder is in. A session's account and config are recorded with it.
- **Kept by each host's daemon** in `~/.enowx-app/tokens/`, a JSON-lines file a day, **deduplicated by the call's ID** (the agent's message, response or step ID), so records read again count once. Files are read from where the last scan left off (`tokens/scan.json`); SQLite stores from the last row.
- **When**: every 20 seconds while tracking is on, and right after a session's turn ends.
- **Kept a year**; older days are rolled up into a day's totals per agent, model, project and config (`rollups.json`).
- **Backfill**: turning tracking on (or **Read back now**, `tokens.backfill`) reads every record back as far as the settings say (30 days by default, up to a year).
- **enowX's own use, apart**: the judge's tokens (each decision's, `core/decide`), RAG's embedder's (each index), never in the agents' totals or budgets. The judge's agent runs leave no records (`--no-session-persistence`, folders `enowx-judge-*` skipped). Canvas and notes agents are sessions, counted as any.

## Costs

A price table, US dollars a million tokens (input, cache read, cache write, output), ships with enowX (`core/tokens/prices.json`, the providers' October 2026 prices) and the user's prices go over it (Tokens → Prices): a model ID or a prefix ending in `*`, optionally a provider, or a config's own price (OpenRouter's, a local model's 0). Local models cost nothing. When an agent records the cost itself (OpenCode, Pi, Goose, Aider, …), that cost counts. A model without a price is counted as unpriced, never guessed.

Subscriptions: an agent signed in to a plan (Claude Code, Codex, Copilot, Cursor, Kiro, … without a model API of the user's set for it; Codex says its plan) has its tokens shown with the **API-equivalent** cost, marked as such. Its usage limits are in Settings → Agent accounts.

## Budgets

Per day, week or month, for everything, an agent, a config or a project, in tokens or dollars (both: the first reached). Synced: the same on every host, each counting its own use.

- **At 80%** a notification.
- **At 100%** the user's pick: **notify only**; **ask before new sessions** (a new agent session asks the user first; prompts typed into a running terminal can't be held, the app shows the alert); **stop new sessions** (new agent sessions are refused until the period resets).
- **GOA and the judge** read them: near a budget (80%) the judge asks its cheapest judges first, and `usage_summary` lists the budgets near their limit for agents to pick cheaper models.

## In the app

- **Resource usage → Tokens**: totals and a chart per day, input / cached / output side by side, split by agent, model, project, session, config or host, for today, 7, 30, 90 days or a year; enowX's own use apart; tabs for Budgets, Prices and Coverage.
- **Every host's numbers together**: the app asks each host it reaches (the one shown and the ones watched); a host not reached shows its last synced totals. Each host writes its daily totals of the last 90 days to `settings/tokens-hosts/<its ID>.json`, synced with the user's data, every 30 minutes while tracking.
- **Per session**: tokens and cost so far in the line under its header and beside the chat's context meter, and on hover in the session list.
- **Per project**: the project's menu → **Token usage** opens the tab filtered to it.

## Protocol

`tokens.settings`, `tokens.setSettings`, `tokens.summary` (`TokensQuery` → `TokensSummary`), `tokens.session`, `tokens.sessions`, `tokens.coverage`, `tokens.backfill`, `tokens.prices`, `tokens.hosts`, `tokens.budgets`; events `tokens.changed` (sessions whose use grew) and `tokens.budget` (a budget crossing 80% or 100%). `session.create` takes `tokenBudgetOK` once the user said yes to a budget asking first; its error starts with `proto.TokenBudgetAsks` or `proto.TokenBudgetStops`.

## Command line

```
enowx usage [--by agent|model|project|session|config|host] [--since 7d] [--host H] [--json]
enowx usage session [SESSION]     # a session's (self by default), by model
enowx usage budgets               # each budget and how much is used
enowx usage coverage              # each agent's layer
```

MCP: `usage_summary`, `usage_session` (feature **Token usage**, Settings → Features).
