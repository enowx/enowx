# Claude Platform

Claude Platform is enowX's own coding agent: a Go loop over Anthropic's Messages API, part of enowX's program. It needs nothing installed (no Node, no Python, no other agent) on any host: macOS, Linux, Windows, a server. It runs on the user's API key from the vault, billed by Anthropic per token, and offers the tools Claude Code names: `Read`, `Write`, `Edit`, `Glob`, `Grep`, `Bash`, `AskUserQuestion`, `Task` and MCP's `mcp__server__tool`.

## How it runs

It is a program in a session's terminal like any other agent, so sessions, tabs, hibernation, restarts and other hosts work as they do for any: `<enowX's program> claude-platform [flags] [prompt]`. The preset (`core/agent/platform.go`, ID `claude-platform`, `Builtin`) always counts as installed, has nothing to install or sign in to, and resolves its program to the running enowX: `$ENOWX_BIN`, which the daemon sets to its launcher (or on Windows its own copy), else the executable itself.

| Flag | |
|---|---|
| `--model M`, `--effort E`, `--permission-mode P` | What it starts on (below). |
| `--dangerously-skip-permissions`, `--allow-dangerously-skip-permissions` | Start in bypass; offer bypass as a mode to switch to. |
| `--continue`, `--resume ID`, `--session-dir DIR` | The newest conversation in the folder; one by its ID; the folder a session keeps them in. |
| `--append-system-prompt-file F`, `--system-prompt-file F` | Added after the prompt; in place of it. |
| `--mcp-config F` | MCP servers, as a file's path or its JSON. |
| `--max-budget D` | Dollars the session may spend before asking. |

## What enowX gives it

- **Guide and tools.** Its inject and guide in one system prompt (`--append-system-prompt-file`, a file of the session) and enowX's MCP server (`--mcp-config`), both in `agent-files/<session>/` so no command line holds them, as for Claude Code ([agents.md](agents.md)).
- **State and events.** It calls the daemon itself (`session.hook`), on Windows too, using Claude Code's tool names: state, `prompt.start`, `tool.before` (whose answer it acts on: refuse, or run unasked), `tool.after`, `turn.done`, and the session's start and end. It has no hooks in its command line (`HookArgs` is nil), and it counts as reporting itself everywhere that matters: it is never marked done by a quiet screen, and "skip what nobody hears" doesn't apply. A tool event's command and paths are read as every agent's are (`hooks.ToolEvent`).
- **Permission prompts** show on its screen and are answered by keys: `1` allow, `2` always, `3` or Esc deny.
- **A hook can rewrite its call.** A user's hook that changes a command has the changed command run, and asked about if it asks.
- **GOD of Agents** runs on it enforced by `tool.before` alone: it has no flag limiting its tools, and needs none, as every call goes by the daemon's answer first. GOA's lines reach it typed in when it is idle ([goa.md](goa.md)).
- **Not offered:** rewinding from the chat and forking a session.

## The record

One JSON line per record, in `<session dir>/<id>.jsonl`: a session's own folder (`~/.enowx-app/agent-sessions/<session>/`) or, run by hand, `~/.enowx-app/claude-platform/sessions/` (package `core/platformagent/record`). The agent resumes from it, the chat reads it (`core/transcript/platform.go`: text and thoughts growing as they stream, each answer's final text in their place, tool calls, notices, cost and budget, a subagent's steps folded into its call), and token usage reads each answer's usage and cost (`core/tokens/adapt_platform.go`). No record holds an API key.

## The key

The key lives only in the vault, as a credential of platform `anthropic` (`Preset.VaultPlatform`). It reaches the agent only as `ANTHROPIC_API_KEY` in its session's process environment: never in a file, a command line, a transcript, a log, a session's saved command or a sync.

**Adding one.** Credentials → Add credential → Anthropic, or `enowx-app creds login anthropic`, which opens that form. Whether one is set: `vault.index` (`VaultIndex.Credentials`, any with `platform: "anthropic"`, no values there).

**At a session's start** the daemon picks the credential from the host's index (the one scoped to the session's project, else the platform's default, else the first; `vault.AgentKey`), and asks the app for it with the session's other credentials (`vault.request`, action `session`), the same lending as `creds env` ([vault.md](vault.md)): the app answers at once on a host the credential may be used on, and otherwise asks the user: **Allow once**, **Always on HOST**, **Refuse**, naming Claude Platform and the host. The daemon keeps the value in memory for the session, masked wherever a session's screen, conversation or traffic is read, and the session keeps only the credential's name.

**Hibernating and waking** use what the daemon holds. **After a daemon restart** nothing is held and no app is connected yet, so a session that has a key waits, stopped, and continues by itself once an app serves the vault; waking one before that says it needs the app, rather than start with no key. A credential added since a session started is picked up the next time it starts.

**With no credential,** or one refused, the session still starts, and the agent says what is missing.

**A config** can run it on another API speaking Anthropic's protocol: a model API config with `--key-cred` gives `ANTHROPIC_BASE_URL` and `ANTHROPIC_API_KEY` in the session's environment and `--model` ([configs.md](configs.md)); that key is read as any config's is, and no credential is lent for it. It has no settings of its own an API could be written to, so `agent_api_set` refuses it: that would be a key in a file.

## Defaults

A new conversation starts on `claude-opus-5-5`, effort `high`, and a budget of $10 per session: past that it asks before going on (`record.Default*`; `/budget` or `--max-budget` to change it).

| Model | Window |
|---|---|
| Opus 5.5 (the default), Fable 5.1, Sonnet 5.5, Haiku 5.5, Opus 5, Sonnet 5 | 1M |
| Haiku 4.5 | 200K |

**While it works** its status line counts the seconds, so a long command or a slow answer doesn't read as a stopped session. **Modes** go by Shift+Tab, and its status line names the one on: `default` "ask first mode on", `acceptEdits` "accept edits on", `plan` "plan mode on", `bypassPermissions` "bypass permissions on". **Effort** is low, medium, high, xhigh or max (`/effort`), **model** `/model`.

**The system prompt** is, first found: `--system-prompt-file` for one session; `~/.enowx-app/claude-platform/system-prompt.md` for every session on the host; the built-in one. enowX's inject and guide go after whichever it is. A config's `args` can carry `--system-prompt-file`, so a config can have its own.

**Remaining credit isn't shown:** an ordinary API key can't read it. The Console shows it, and the Admin API's cost report needs an admin key. The session's cost in dollars, at list prices, shows beside its budget.

## Files

| Where | What |
|---|---|
| `core/platformagent` | The loop's API: `Runner`, `Front`, `Hooks`, `Options` |
| `core/platformagent/record` | The file format and its folders |
| `core/platformagent/terminal`, `tools`, `permission`, `mcpclient` | Its screen, tools, permission rules and MCP client |
| `core/agent/platform.go` | The preset's settings, commands, file forms |
| `daemon/agentkey.go` | The key at start, wake and restart |
| `daemon/platform_e2e*_test.go` | The built program as a session of its own daemon, against the stand-in API (`internal/standinapi`) |
