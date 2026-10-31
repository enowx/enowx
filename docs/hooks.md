# Hooks

A hook is something enowX does when something happens in a session: run a command, give the agent context, refuse or change what the agent is about to do, show a notification, or call a webhook. One hook works on every agent.

- **Agents with hooks of their own** (Claude Code, Codex) hand each event to enowX before acting on it. A hook can still refuse it, add context or change the command.
- **Other agents:** the daemon sees the event from outside, after the fact.

Settings → **Hooks** shows:
- the changes agents proposed, with Allow and Reject;
- projects whose hook files wait to be allowed;
- the hooks, grouped by where they are kept, each with its switch, Edit and Remove;
- **From a template** and **Add a hook**. The dialog edits a hook as a form or as its JSON, and has Test, which runs it now on a sample event.

The **Log** tab shows the latest 200 runs since the daemon started, with what each printed.

## Events

| Event | When | Claude Code, Codex | Other agents |
|---|---|---|---|
| `session.start` | the agent started or was continued | in the agent | from outside |
| `prompt.start` | a prompt is about to reach the agent | in the agent | typed and sent with Enter, seen after |
| `working` | the agent started working | from outside | from outside |
| `tool.before` | a tool is about to run | in the agent | — |
| `tool.after` | a tool ran (`failed` when it failed) | in the agent | — |
| `permission` | the agent asks for permission | in the agent | — |
| `permission.answer` | the user allowed (`allowed`) or denied it: the tool ran next, or the turn ended first | in the agent | — |
| `waiting` | the agent waits for the user | Claude Code | — |
| `compact` | the agent is about to compact its context | in the agent | — |
| `turn.done` | the agent finished its turn | in the agent | its screen quiet for 4 s |
| `file.changed` | the files a turn changed (git's view, changed since the turn began) | from outside | from outside |
| `git.before` | a commit, push or merge (`op`) is about to run: the agent's command, or the app's Git panel | in the agent | the Git panel only |
| `git.commit`, `git.merge` | a commit (a merge when it has two parents) in the session's folder | from outside, looked at every 5 s | from outside |
| `git.push` | the branch's upstream moved to it | from outside | from outside |
| `git.conflict` | files in conflict (`paths`) | from outside | from outside |
| `error` | its program ended with an exit code (`exit`) | from outside | from outside |
| `session.hibernate`, `session.wake` | the daemon stopped the idle agent; it was continued | from outside | from outside |
| `session.end` | its program ended | in the agent | from outside |
| `port.open` | a session started listening on `port` | the host | the host |
| `sync.done` | a sync of the user's data brought changes (`paths`) | the host | the host |
| `notify` | a notification was sent to the apps (`kind`, `text`), as `enowx notify sound` does | the host | the host |
| `schedule` | the hook's own Schedule came round ("hourly", "daily 09:00", "weekdays 18:30", "weekly mon 08:00"), in a project's folder for a project's hook | the host | the host |

`HookSupport` in `core/agent` keeps this table, and the dialog shows it under the event. Hooks are POSIX shell, so on Windows only the events seen from outside run.

## Conditions

An empty condition matches anything. By default, every condition a hook sets must hold. **Any of them** (`any`) runs the hook when one holds, and **is not** (`not`) turns a condition round.

- **Agents:** base IDs (`claude`, `codex`).
- **Projects:** a project's folder or its folder's name. The session's folder counts too, so a worktree matches its project.
- **Branches:** globs (`main`, `release/*`).
- **Hosts:** the host's name.
- **Tool or command:** a regular expression, case-insensitive, on the tool's name or its command (`^Bash$`, `git push.*\bmain\b`).
- **Files:** globs from the project's folder, as in project links (`*.go`, `src/**`), on the files changed or touched.
- **Prompt or command text:** a regular expression on the prompt, the command or a notification's text.

Limits: **once a session**, **at most N times a session**, **apart at least** some seconds, and **only within** some hours (`09-18`, `22:00-06:00`, which spans midnight).

## Actions

- **Run a command.** It runs with `/bin/sh` in the session's folder.
  - The event is passed as JSON on stdin (`proto.HookEvent`).
  - It also gets `ENOWX_HOOK_EVENT`, `ENOWX_HOOK_NAME`, `ENOWX_HOOK_SESSION`, `ENOWX_HOOK_AGENT`, `ENOWX_SESSION` and `ENOWX_BIN`.
  - Exiting 2 refuses the action, with stderr as the reason.
  - Before a tool, printing `{"command": "…"}` changes the command. Otherwise, what it prints is given to the agent as context, before a prompt or at a session's start.
  - It has a timeout (30 s unless set), and is killed together with what it started.
  - **In the background**, the agent doesn't wait for it.
- **Add context:** text the agent reads before a prompt or at a session's start.
- **Refuse:** a tool about to run, a prompt, or a commit, push or merge is refused, and Reason is told to the agent, or shown in the Git panel. On other events, the refusal is only logged.
- **Change the command:** Find (a regular expression) in a tool's command is replaced by Replace (`$1` for a group).
  - Claude Code asks the user about the new command, unless the session skips permissions.
  - Codex takes a changed command only as approved, so a Codex session that asks gets it as a refusal naming the command to run instead.
- **Ask the judge** (before a tool runs): enowX's judge allows the command without asking the user, leaves it to them, or refuses it ([decide.md](decide.md)).
- **Trim its long output** (before a tool runs): the command runs through `enowx decide trim`, and a long output reaches the agent cut by the judge to what matters ([decide.md](decide.md)).
- **Notify:** a sound (done, input, error) and a banner in the apps, saying Message. `{agent}`, `{project}`, `{branch}`, `{event}`, `{prompt}`, `{command}`, `{paths}`, `{commit}`, `{port}`, `{text}` and `{host}` are filled in.
- **Call a webhook:** POST to URL.
  - Discord's webhooks get Message as `content`, with no mentions, and Slack's as `text`.
  - Any other address gets the event as JSON, with Message as `message`.
  - The log keeps only the address's host, since a webhook's path is its token.
- **Run an enowX command:** enowX's command line with these arguments (`project autopush . --now`, `notes write …`), as a hook's command would run `"$ENOWX_BIN" …`.

Hooks run in this order: the host's, the project's, then the session's, each in its list's order. The first refusal stops the rest, contexts add up, and a changed command is what the next hooks see. On events nothing waits for, every hook runs in the background.

## Security

- Text from the agent (the prompt, a command) is masked of the vault's credentials lent to the session before a hook sees it.
- A hook's command runs with the daemon's environment, without the values of credentials lent to sessions. It reaches the vault only as the user's own commands do: `enowx creds use`, which asks the user.
- A project's own hooks run only once the user allowed the file as it is.
- What an agent adds, changes or removes is a proposal (`enowx hooks …` from a session, or the `hooks_*` MCP tools). It changes nothing until the user allows it in Settings → Hooks.

## How agents hand over events

enowX adds its own hooks when it starts Claude Code (`--settings`) or Codex (`-c hooks.<Event>=…`). They run `"$ENOWX_BIN" hook --event E [--state S]` with the agent's JSON on stdin (`daemon.Hook`).

That command calls `session.hook` on the daemon. The daemon sets the state and runs the matching hooks; a Git commit, push or merge in a tool's command also goes through the `git.before` hooks. The command then answers the agent as agents read their hooks:
- exit 2 with the reason on stderr, to refuse;
- the context on stdout;
- or, to change the command, `hookSpecificOutput` with `updatedInput` and `permissionDecision` `allow` (the session skips permissions) or `ask`.

Outside enowX (no `ENOWX_BIN`), these hooks do nothing.

Codex runs a hook only once the user trusted its exact definition, kept as a hash in `config.toml` (`[hooks.state."<source>:<event>:<group>:<handler>"] trusted_hash`).
- enowX's hooks are the same in every session, so the daemon records them as trusted itself (`agent.TrustCodexHooks`) before starting Codex.
- It adds only those tables, under the source `/<session-flags>/config.toml`, and leaves the rest of the file as it was. A hook the user turned off there stays off.
- The user's own hooks, and a project's `.codex/hooks.json`, still ask in Codex.

## Where hooks are kept

- **The host's own:** `~/.enowx-app/settings/hooks.json`, synced to the user's other hosts with their settings ([user-data-sync.md](user-data-sync.md)). The daemon reads the file again when it changes.
- **A project's own:** its `.enowx/hooks.json`, committed with it, as a list of the same hooks.
  - Saved from the app or `enowx hooks add --scope project`, the file is allowed as written.
  - Otherwise, its hooks don't run until the user allows that file as it is, and Settings → Hooks shows the projects waiting, with Review… and Allow them.
  - The approval is of the file's hash, kept per host in `~/.enowx-app/hooks-allowed.json`, so any change asks again.
- **A session's own:** kept by the daemon until the session ends (a hibernated one keeps them), not on disk.
- **Agents' proposals:** in `~/.enowx-app/hooks-proposed.json` until the user decides.

## Command line and MCP

`enowx hooks` lists the hooks with where they are kept, the projects waiting and the proposals. The other forms:
- `hooks events` lists the events.
- `hooks add` adds a hook, with flags for every field: `--event`, `--do run|context|refuse|rewrite|judge|trim|notify|webhook|enowx`, `--command`, `--tool`, `--files`, `--once`, `--schedule`, `--scope host|project|session` …
- `hooks edit ID` changes only the flags given.
- `hooks rm|on|off ID` removes a hook, or turns it on or off.
- `hooks test ID` runs it on a sample event.
- `hooks log [--last N]` shows the latest runs.

From a session, `add`, `edit`, `rm`, `on` and `off` are proposed. The MCP tools are `hooks_list`, `hooks_add`, `hooks_edit`, `hooks_remove`, `hooks_test` and `hooks_log` (feature **Hooks** in Settings → Features).

## Protocol

| Method | What |
|---|---|
| `hooks.list` (ProjectDir) | HooksInfo: the host's hooks, the projects' and sessions' own, files and proposals waiting, each agent's support |
| `hooks.save` ([]Hook) | the host's hooks, all of them |
| `hooks.saveProject` (ProjectHooks) | a project's hooks, written and allowed |
| `hooks.saveSession` (SessionHooks) | a session's own hooks |
| `hooks.approve` (HooksApproval) | allow or stop allowing a project's file |
| `hooks.propose` (HookProposal), `hooks.decide` (HookDecideParams) | an agent's proposed change, and the user's answer |
| `hooks.test` (HookTest) | run a hook now on a sample event |
| `hooks.log` | the latest runs |
| `session.hook` (SessionHookParams) | an agent's event: the state, then the hooks' decision |
| event `hooks.changed` | the hooks, files waiting or proposals changed |
