# Claude Platform

Claude Platform is enowX's own coding agent: a Go loop over Anthropic's Messages API, part of enowX's program. It needs nothing installed (no Node, no Python, no other agent) on any host: macOS, Linux, Windows, a server. It runs on the user's API key from the vault, billed by Anthropic per token, and offers the tools Claude Code names: `Read`, `Write`, `Edit`, `Glob`, `Grep`, `Bash`, `AskUserQuestion`, `Task` and MCP's `mcp__server__tool`.

## How it runs

It is a program in a session's terminal like any other agent, so sessions, tabs, hibernation, restarts and other hosts work as they do for any: `<enowX's program> claude-platform [flags] [prompt]`. The preset (`core/agent/platform.go`, ID `claude-platform`, `Builtin`) always counts as installed, has nothing to install or sign in to, and resolves its program to the running enowX: `$ENOWX_BIN`, which the daemon sets to its launcher (or on Windows its own copy), else the executable itself.

| Flag | |
|---|---|
| `--model M`, `--effort E`, `--permission-mode P` | What it starts on (below). |
| `--dangerously-skip-permissions`, `--allow-dangerously-skip-permissions` | Start in bypass; offer bypass as a mode to switch to. |
| `--permission-mode-if-new P` | The mode when the conversation has none kept: what enowX starts a session again with. |
| `--continue`, `--resume ID`, `--session-dir DIR` | The newest conversation in the folder; one by its ID; the folder a session keeps them in. |
| `--append-system-prompt-file F`, `--system-prompt-file F` | Added after the prompt; in place of it. |
| `--mcp-config F` | MCP servers, as a file's path or its JSON. |
| `--max-budget D` | Dollars the session may spend before asking. |

## What enowX gives it

- **Guide and tools.** Its inject and guide in one system prompt (`--append-system-prompt-file`, a file of the session) and enowX's MCP server (`--mcp-config`), both in `agent-files/<session>/` so no command line holds them, as for Claude Code ([agents.md](agents.md)).
- **State and events.** It calls the daemon itself (`session.hook`), on Windows too, using Claude Code's tool names: state, `prompt.start`, `tool.before` (whose answer it acts on: refuse, or run unasked), `tool.after`, `turn.done`, and the session's start and end. It has no hooks in its command line (`HookArgs` is nil), and it counts as reporting itself everywhere that matters: it is never marked done by a quiet screen, and "skip what nobody hears" doesn't apply. A tool event's command and paths are read as every agent's are (`hooks.ToolEvent`).
- **Permission prompts** show on its screen and are answered by keys: `1` allow, `2` always, `3` or Esc deny. `Read`, `Glob` and `Grep` run unasked inside the session's folder, links followed; outside it they ask in every mode but bypass, plan mode included, for that call alone. An MCP tool always asks and plan mode refuses it, whatever its server's `readOnlyHint` says (the hint only lets its calls run side by side). "Always" on a `Write` or `Edit` keeps its name, which holds only inside the folder and outside guarded paths (`.git`, `.claude`, `.codex`, `.opencode`, `.mcp.json`, `.ssh`, `.config`, `.husky`, `.github`, `.gnupg`, `.aws`, every `.enowx*`); elsewhere it asks each time. For `Bash` it keeps a prefix only of a few plainly bounded commands (`git status`, `diff`, `log`, `show`; `go test`, `build`, `vet`; `npm test`; `make`; `ls`; `pwd`), and those not with flags that run other programs or write elsewhere (`go test -exec`, `git diff --output`, `make -f` and more); any other command is kept exactly. Conversations from before this keep their broader rules. A file is attached by naming it only in the user's own message, not in what the model or a hook wrote.
- **A hook can rewrite its call.** A user's hook that changes a command has the new one run, and a person asked to allow the call sees the new one. The call is judged by mode and rules as the model sent it.
- **GOD of Agents** runs on it enforced by `tool.before` alone: it has no flag limiting its tools, and needs none, as every call goes by the daemon's answer first. GOA's lines reach it typed in when it is idle ([goa.md](goa.md)).
- **Not offered:** rewinding from the chat and forking a session.

## The record

One JSON line per record, in `<session dir>/<id>.jsonl`: a session's own folder (`~/.enowx-app/agent-sessions/<session>/`) or, run by hand, `~/.enowx-app/claude-platform/sessions/` (package `core/platformagent/record`). The agent resumes from it, the chat reads it (`core/transcript/platform.go`: text and thoughts growing as they stream, each answer's final text in their place, tool calls, notices, cost and budget, a subagent's steps folded into its call), and token usage reads each answer's usage and cost (`core/tokens/adapt_platform.go`). No record holds an API key.

## The key

The key lives only in the vault, as a credential of platform `anthropic` (`Preset.VaultPlatform`). The daemon keeps it apart from what the session's other programs get, and the agent takes it from the daemon over the local socket as it starts (`vault.agentKey`, once per start): never in a file, a command line, a transcript, a log, a session's saved command, a sync, or the session's environment, which the wrapper shells around the agent, the login shell after it and its tabs would share.

**What keeps it from the agent's own commands.** The tools' programs start without it (`tools.CleanEnviron`). The agent then forgets it from its environment (`os.Unsetenv`; the SDK reads the environment only as its client is made) and holds it in memory, and on Linux makes itself non-dumpable (`PR_SET_DUMPABLE` 0), so a command of the same user can't read its `/proc/PID/environ` or memory, or attach to it. Text a tool brings back, an MCP server's too, has the literal key replaced before the model, the record and the screen see it.

**What that doesn't stop.**

- **`"$ENOWX_BIN" creds use` is open to any command.** The vault lends a credential to whatever program asks in the session, by its design: a credential allowed on this host, or allowed by the user when asked, can be read by a command the model runs, this key included. The protections above stop accidents and prying through the process; they are not a sandbox between the model's commands and the vault. `vault.agentKey` itself is one-shot: a program racing the agent to it leaves the agent with no key, which says so.
- **Linux's limit:** root and `CAP_SYS_PTRACE` still read the process. Non-dumpable lasts for the agent alone: a program it starts is dumpable again after its exec, which is why those start without the key. Enabling core dumps doesn't undo it.
- **macOS and Windows have no such call used here:** a program of the same user can read the agent's memory (with the usual debugging rights) and, for a key given by hand, its starting environment (`ps eww` on macOS).
- **A key given by hand or by a config:** run from a shell with `ANTHROPIC_API_KEY` set, or on a model API config, the key is in the environment the agent starts with, and so in that shell's or wrapper's environment. Unsetting changes only the agent's own view: `/proc/PID/environ` shows what it started with, which is what non-dumpable hides, for the agent and not for those others.
- **Encoded forms:** only the literal key is replaced in text; base64 of it isn't.

**Adding one.** Credentials → Add credential → Anthropic, or `enowx-app creds login anthropic`, which opens that form. Whether one is set: `vault.index` (`VaultIndex.Credentials`, any with `platform: "anthropic"`, no values there).

**At a session's start** the daemon picks the credential from the host's index (the one scoped to the session's project, else the platform's default, else the first; `vault.AgentKey`), and asks the app for it with the session's other credentials (`vault.request`, action `session`), the same lending as `creds env` ([vault.md](vault.md)): the app answers at once on a host the credential may be used on, and otherwise asks the user: **Allow once**, **Always on HOST**, **Refuse**, naming Claude Platform and the host. The daemon keeps the value in memory for the session, masked wherever a session's screen, conversation or traffic is read, and the session keeps only the credential's name.

**Tabs and other programs of the session** (Android runs, the Run menu's, a worktree's setup) get its other credentials as ever, but not this one, nor its name, so a restart doesn't lend it to them; a tab that is itself Claude Platform takes its own, asking the app for it as a new session does when the session it is in isn't Claude Platform. Any credential of platform `anthropic` in such a session counts, a named or an automatic one too. Running `claude-platform` by hand in the session's shell after the agent left finds no key waiting: the session's next start has one.

**Hibernating and waking** use what the daemon holds. **After a daemon restart** nothing is held and no app is connected yet, so a session that has a key waits, stopped, and continues by itself once an app serves the vault; waking one before that says it needs the app, rather than start with no key. A credential added since a session started is picked up the next time it starts.

**With no credential,** or one refused, the session still starts, and the agent says what is missing.

**A config** can run it on another API speaking Anthropic's protocol: a model API config with `--key-cred` gives `ANTHROPIC_BASE_URL` and `ANTHROPIC_API_KEY` in the session's environment and `--model` ([configs.md](configs.md)); that key is read as any config's is, and no credential is lent for it. It has no settings of its own an API could be written to, so `agent_api_set` refuses it: that would be a key in a file.

## Defaults

A new conversation starts on `claude-opus-5-5`, effort `high`, and a budget of $10 per session: past that it asks before going on (`record.Default*`; `/budget` or `--max-budget` to change it, 0 for no limit). A conversation continued or resumed keeps its own, unless `--max-budget` says, and one with nothing to continue gets the default. Spending counts what a failed, cut or interrupted stream said so far, and compactions; compacting comes after the check, so a refused budget spends nothing more. A compaction that fails is tried once in a turn, and not again until the context has grown by a tenth.

| Model | Window |
|---|---|
| Opus 5.5 (the default), Fable 5.1, Sonnet 5.5, Haiku 5.5, Opus 5, Sonnet 5 | 1M |
| Haiku 4.5 | 200K |

**While it works** its status line counts the seconds, so a long command or a slow answer doesn't read as a stopped session. **Modes** go by Shift+Tab, and its status line names the one on: `default` "ask first mode on", `acceptEdits` "accept edits on", `plan` "plan mode on", `bypassPermissions` "bypass permissions on". A conversation goes on in the mode it kept, so hibernating and waking change nothing, and a flag given at every start can't undo a switch to something safer; only an explicit `--permission-mode` overrides it. A kept bypass comes back only with `--dangerously-skip-permissions` or `--allow-dangerously-skip-permissions` given again, else it asks first and says so. GOD of Agents' allowing of `mcp__enowx__` tools lasts for that run alone, never on record. **Effort** is low, medium, high, xhigh or max (`/effort`), **model** `/model`.

**The system prompt** is, first found: `--system-prompt-file` for one session; `~/.enowx-app/claude-platform/system-prompt.md` for every session on the host; the built-in one. enowX's inject and guide go after whichever it is. A config's `args` can carry `--system-prompt-file`, so a config can have its own.

**Remaining credit isn't shown:** an ordinary API key can't read it. The Console shows it, and the Admin API's cost report needs an admin key. The session's cost in dollars, at list prices, shows beside its budget.

## Limits

Known and left as they are:

- **Any session can type into another.** `send` (enowX's `session.input`, the same for every agent) can put digits into this agent's terminal, so one session can answer another's permission prompt, or its own through a program it runs.
- **A program of the same user can replace the system prompt file** in `agent-files/<session>/`, or the host's `system-prompt.md`, which the next start reads. Nothing here defends against the user's own programs.
- **A link swapped at the last moment.** An edit is judged on where its path leads, again after hooks have run, and then written: a program racing to swap a link in the moments between that check and the write can still land the write through it. The files are not opened by a handle the check verified, which would need a different write path per platform.
- **Reads of a command.** The path checks are of `Read`, `Glob`, `Grep`, `Write` and `Edit`: an allowed `Bash` command reads and writes whatever the user can, and a kept prefix like `make` runs what the project's files say.
- **A process left over.** On Linux the agent's environment can't be read even by its daemon, so the reaping that finds a session's leftover processes by their environment doesn't find the agent itself; it ends with its terminal.

## Files

| Where | What |
|---|---|
| `core/platformagent` | The loop's API: `Runner`, `Front`, `Hooks`, `Options` |
| `core/platformagent/record` | The file format and its folders |
| `core/platformagent/terminal`, `tools`, `permission`, `mcpclient` | Its screen, tools, permission rules and MCP client |
| `core/agent/platform.go` | The preset's settings, commands, file forms |
| `daemon/agentkey.go` | The key at start, wake and restart |
| `daemon/platform_e2e*_test.go` | The built program as a session of its own daemon, against the stand-in API (`internal/standinapi`) |
