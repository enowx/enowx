# What a session reaches: every feature and setting against its tool

GOD of Agents (one session controlling every session, on any host; PLAN §10) reaches enowX through the command line and its MCP tools alone. This table is the audit of that: each feature and setting of enowX, the command (and MCP tool) that reads it and the one that changes it, whether it works on another host, and what is missing. Keep it up to date when a feature or a setting comes.

**Marks:** ✅ read and changed by a tool; ◐ partly (what is missing is said); ✗ no tool yet; — not meant for a tool (the app's own look and feel, a page to look at).
**Other host:** `--host H` (MCP: `host`) runs the command on host H through the app ([cli.md](cli.md#control)); "own" means the command has a `--host` of its own; "synced" means the data is the same on every host already ([user-data-sync.md](user-data-sync.md)); "app" means the app answers wherever it is asked from.

## Sessions

| Feature | Reads | Changes | Other host | |
|---|---|---|---|---|
| Sessions of a host | `ls`, `status` (`sessions_list`) | | `--host` | ✅ |
| Sessions of every host | `sessions` (`sessions_everywhere`) | | all hosts | ✅ |
| Start a session (agent, project, branch mode, prompt, model, effort, account, skip permissions) | | `spawn` (`session_spawn`) | `--host` (with `--project`) | ✅ |
| An agent's configs (custom sessions: provider, model, account) | `configs` (`configs_list`) | `configs …`, `spawn --config` (`session_spawn` config) | `--host` (spawn) | ✅ |
| The model and effort a prompt needs (the judge) | `decide route` (`decide_route`) | `spawn --model --effort`, `switch --model --effort` | `--host` (spawn, switch) | ✅ |
| One prompt to several agents (fan-out) | | `spawn --agents` | `--host` | ✅ |
| Send a prompt or keys | | `send` (`session_send`) | `--host` | ✅ |
| Its screen | `read` (`session_read`) | | `--host` | ✅ |
| Its chat (transcript) | `chat` (`session_chat`), `peek` | | `--host`, own | ✅ |
| Wait for it | `wait` (`session_wait`) | | `--host` | ✅ |
| Interrupt it (Escape) | | `interrupt` (`session_interrupt`) | `--host` | ✅ |
| Stop it, continue it | | `stop`, `continue` (`session_stop`, `session_continue`) | `--host` | ✅ |
| Fork it (a compacted copy goes on in a new session) | | `fork` (`session_fork`) | `--host` | ✅ Claude Code and Codex |
| Hibernate, wake, never hibernate on its own | | `hibernate`, `wake`, `hibernate --auto` (`session_hibernate`, `session_wake`) | `--host` | ✅ |
| Rename it | | `rename` (`session_rename`) | `--host` | ✅ |
| Its note (status line) | `ls` | `note` (`session_note`) | this host | ◐ another session's note on another host: not offered (a session writes its own) |
| Close it | | `close --yes` (`session_close`, yes; the user confirms) | `--host` | ✅ |
| Its permission prompt | `answer` (`session_ask`) | `answer ID allow\|always\|deny\|N` (`session_answer`) | `--host` | ◐ the keys are known for Claude Code only; other agents: read the screen and `send` |
| Model, effort, permission mode, account mid-session | the chat's header: `chat` | `switch` (`session_switch`), `accounts use --session` | `--host` | ◐ the current model and effort aren't printed by a tool yet (they are in `session.transcript`) |
| Its time budget | | | | ✗ `session.budget` has no command |
| Private from the host's teams | | | | ✗ `session.private` has no command |
| Rewind (edit a past message) | | | | ✗ `session.rewind`, the chat's alone |
| Its slash commands | | | | ✗ `session.commands` has no command (`send "/…"` types one) |
| Attach to its terminal | | `attach` | this host | ✅ |
| Its tabs, ports, Run menu | `ports`, `run` | `run N`, `run --save` | this host | ◐ no `--host` |
| A virtual display (desktop apps on Linux) | | `display` | this host | ✅ |
| Its Browser | `browser snapshot\|screenshot\|eval` | `browser open\|click\|fill` | this host | ✅ |
| Pipelines | `pipelines` | `pipeline` | this host | ◐ cancelling or removing one: no command |
| Automations | `automations` | `automation add\|run\|enable\|disable\|remove` | this host | ✅ |
| Take over a teammate's session | | | | — the app's, between people |

## When the user is asked

What can't be undone (the `confirm` methods in `core/hostcontrol`: close, push, pull request, merge, discard, removing a project, a host's environment variable) and hibernating or stopping sessions, asked by an agent, follow the asking session's permission mode:

- **In bypass, on the user's own hosts:** done without asking, and logged: `control log` says `not asked: the agent is in bypassPermissions`.
- **Always allowed, hibernating and closing only:** the card for these two offers **Always allow**, for the session asking. From then on, on the user's own hosts, that session hibernates and closes sessions without a card, each logged: `control log` says `not asked: the user always allows this session, and nothing would be lost`. It still asks for an agent at work, something served, an app shown, a program left running, what can't come back on its conversation (a shell, where it can't tell what runs), its own session, and a teammate's session; and for push, pull request, merge, discard, removing a project and a host's environment variables, whatever it was allowed. A teammate's session never gets the choice, and a request to a teammate's shared host always asks. The asking session's daemon holds the rule, in a file that outlives a restart (`control-allow.json`); the app checks another of the user's hosts' sessions, which only it reaches. Settings → Agents → Always allowed lists the sessions and revokes the choice. It isn't reachable through `host.call`, but like the rest here it is no boundary against an agent with the host's socket.
- **Anything else:** a card for the user's yes ([agents.md](agents.md), Agents don't move the user), which doesn't take the keyboard. That includes a mode not known, `auto`, `acceptEdits` and the like, a teammate's session on this host, and any request to a teammate's shared host, which also keeps checking the role.

The mode comes from the daemon, never from what the session sends (`HostCallParams.Bypass` is overwritten): what the agent's own hook last reported as `permission_mode`, which it does just before each tool call, so a shift+tab counts by the next call. A report counts for two minutes, and is forgotten when the agent restarts and when the daemon does. How a session was started counts for nothing, since the mode can change after.

| Agent | What enowX knows | Without asking |
|---|---|---|
| Claude Code, Claude Agent Teams, OpenClaude | `permission_mode` in its hooks, `bypassPermissions` when it asks nothing | Yes |
| Codex | the same field; `bypassPermissions` when it never asks for approval | Yes |
| Every other agent, and any agent on Windows | no report: `--yolo` at the start says nothing of now | No, always the card |

This follows the agent's mode; it isn't a boundary against the agent. A command the agent runs could call `enowx hook` with a mode of its choosing or name another session in `From`, as it could already leave `From` out; agents' other asks (features, the vault, moving project copies) are unchanged.

## Review and merge

| Feature | Reads | Changes | Other host | |
|---|---|---|---|---|
| A session's branch against its base | `changes ID` (`session_changes`) | | `--host` | ✅ |
| Its diff | `changes ID diff [--files]` (`session_changes` diff) | | `--host` | ✅ |
| Commit | | `changes ID commit -m` (`session_commit`) | `--host` | ✅ |
| Push, pull request, merge, discard | | `changes ID push\|pr\|merge\|discard --yes` (`session_push`, `session_pr`, `session_merge`, `session_discard`; yes; the user confirms) | `--host` | ✅ |
| Conflict radar | `project conflicts` (`project_conflicts`) | | this host | ✅ |
| Review comments | | | | ✗ the app's Review page only |
| Starting from a GitHub issue | | | | ✗ `project.issues` has no command |
| Checks (tests, linters, build) of a session | | | | ✗ `checks.list`, `checks.run` have no command |

## Projects

| Feature | Reads | Changes | Other host | |
|---|---|---|---|---|
| A host's projects, their order and groups | `projects`, `project list` (`projects_list`) | `project add`, `projects group\|move\|move-group\|rename-group` (`project_add`) | `--host` (list, add) | ◐ arranging: this host only; removing a project: no command (`project.remove` is reached through `host.call` only) |
| Clone a repository | `project status` (`project_clone_status`) | `project clone`, `project cancel` (`project_clone`) | `--host` | ✅ |
| A project's rules | `project rules` (`project_rules`) | `project rules --set` (`project_rules_set`) | `--host` | ✅ |
| A project's settings (`.enowx.toml`: worktree copy, setup) | `project settings` (`project_settings`) | `project settings --copy --setup` (`project_settings`) | `--host` | ✅ |
| Its context and files, read-only | `project context`, `project read` (`project_context`, `project_read`) | | own | ✅ |
| Copies on other hosts, their syncs, auto push | `project links\|origin\|history\|conflicts` | `project copy\|sync\|autopush` (`project_copy`, `project_sync`, `project_autopush`) | all hosts | ✅ |
| Notes attached to it | `notes --project` (`notes_for_project`) | `notes attach\|detach` | own | ✅ |
| Its hooks (`.enowx/hooks.json`) | `hooks list` | `hooks add\|edit\|rm` (proposed, the user allows) | this host | ✅ |
| RAG: consent, index | `rag status` (`rag_status`) | `rag index`, `rag project on agreed\|off` | this host | ✅ on another host: not yet |
| Moving or packing a project with its sessions | | | | ✗ `transfer.*`, the app's |

## The host's settings

| Setting | Reads | Changes | Other host | |
|---|---|---|---|---|
| Agents at once (`max-agents`) | `settings` (`host_settings`) | `settings set max-agents` | `--host` | ✅ |
| Hibernate idle agents after | `settings` | `settings set hibernate-after` | `--host` | ✅ |
| New sessions hibernate on their own | `settings` | `settings set auto-hibernate`, `hibernate --auto-default` | `--host` | ✅ |
| Terminal: the shell | `settings`, `shells` | `settings set shell`, `shells use` | `--host` (settings) | ✅ |
| Environment: variables every session gets | `settings env` (`host_env`; masked) | `settings env set\|unset` (`host_env`; the user confirms a value set from an agent) | `--host` | ✅ |
| Features agents get | `features` (`features_list`) | `features on\|off\|lazy\|preset` (asks the user from an agent) | own scope (`--host` is a scope there) | ✅ |
| Agent accounts | `accounts list\|status\|log` (`accounts_*`) | `accounts use\|priority\|fallback\|default` | this host | ◐ adding or removing an account: the app's (it signs in) |
| Model APIs | `api` (`agent_api_list`) | `api set\|clear\|restore` (`agent_api_set`, …) | this host | ✅ |
| Hooks | `hooks list\|log\|events` (`hooks_*`) | `hooks add\|edit\|rm\|on\|off` (proposed, the user allows) | this host | ✅ |
| Automations | `automations` | `automation …` | this host | ✅ |
| Decisions (the judge): policy, judges, uses | `decide status\|log\|settings` (`decide_status`, `decide_log`) | `decide mark\|use` | this host | ◐ adding a judge and its key: Settings → Decisions only |
| RAG: on, server, consent | `rag status\|settings\|plan` | `rag set`, `rag project on agreed\|off` | this host | ◐ typed keys: Settings → RAG only |
| Token usage: its switch, budgets, prices | `usage`, `usage budgets\|coverage` (`usage_summary`, `usage_session`) | | this host, `--host` to read | ◐ changing them: Settings only |
| Agents' library (skills, MCP servers, subagents, commands, settings) | | | | ✗ `env.*` has no command (Settings → Agents' library, Sync page) |
| The daemon: log, restart | | | | ✗ `daemon.log` has no command; restarting stays the user's |
| Credentials (the vault) | `creds list\|audit` (`creds_list`, `creds_audit`), never a value | `creds add\|rm\|login` (the user confirms) | this host | ✅ (by design, a value is never read) |
| Cloud sign-in, teams | `cloud status`, `team list\|members` | `cloud login\|logout`, `team …` | this host | ✅ (signing out and removing ask first in the app's flows) |
| Sharing the host with a team | `team hosts` | `team share` | this host | ✅ |
| Team kit | `team kit` | `team kit add\|rm` | this host | ✅ |
| Setting a server up (`enowx.toml`, setup steps) | | | | ✗ `setup.scan` has no command |
| Creating servers (VPS) | | | | ✗ the app's (a provider token) |
| Resource usage | | | | ✗ `stats` has no command |

## The app's settings

| Setting | Reads | Changes | Other host | |
|---|---|---|---|---|
| New session: agent, worktree or project folder, skip permissions, time budget | `settings app` (`app_settings`) | `settings app set default-agent\|new-session-folder\|skip-permissions\|time-budget` | app | ✅ |
| Terminal font and size | `settings app` | `settings app set terminal-font\|terminal-font-size` | app | ✅ |
| Appearance: theme, mode, accent | `settings app` | `settings app set theme\|theme-mode\|accent` | app | ✅ |
| Desktop banners, port cards, stopping agents on quit, the editor | `settings app` | `settings app set desktop-notifications\|port-cards\|stop-agents-on-quit\|editor` | app | ✅ |
| Notification sounds, quiet hours, mutes | `notify status` (`notify_status`) | `notify set\|mute\|unmute` (`notify_set`) | synced | ✅ |
| Keeping the app in the tray, looking for updates | | | | ✗ not in `app.settings` yet |
| Floating control, Discord, the sidebar | | | | — the app's own look and feel |

## Content

| Feature | Reads | Changes | Other host | |
|---|---|---|---|---|
| Notes, groups, versions, sections, to-dos | `notes`, `notes show\|outline\|history` (`notes_*`) | `notes new\|write\|move\|rm\|check\|restore` | synced, teams' spaces | ✅ |
| A note's agent | `notes chat` (`notes_chat`) | `notes ask\|agent` (`notes_ask`, `notes_agent`) | own | ✅ |
| Proposals | `notes proposals` | `notes accept\|reject` | synced | ✅ |
| Import and export | `notes import-status` | `notes import\|export` | this host | ✅ |
| Canvases, designs, comments, versions, exports | `canvas list\|show\|comments\|history\|screenshot` (`canvas_*`) | `canvas new\|write\|patch\|shape\|…` | synced | ✅ |
| Design brief, unslop, review, build | `canvas brief`, `browser unslop` | `canvas build`, `review` | this host | ✅ |
| Community library (opt-in) | `library search\|show\|prompt` (`library_*`), other people's data, labelled | `library publish` only shows the user a card to the form; sending is the user's, in the app | the host's cloud | ✅ |
| Icons | `icons search` (`icons_search`) | | needs no host | ✅ |
| Team chat | `team chat` (`team_chat_read`) | `team chat --send` (`team_chat_send`) | cloud | ✅ |
| Your data's sync | `sync status\|conflicts\|trash` (`sync_*`) | `sync now\|resolve\|restore` | all hosts | ✅ |

## Previews and debugging

| Feature | Reads | Changes | Other host | |
|---|---|---|---|---|
| HTTP traffic: exchanges, rules, settings, CA | `traffic list\|show\|rules\|settings\|status` (`traffic_*`) | `traffic mock\|fail\|forward\|breakpoint\|settings set…` | this host | ✅ |
| Android: devices, run, screen, input | `android status\|screenshot\|controls\|logcat` (`android_*`) | `android start\|run\|tap\|type\|key\|catch` | this host (a device of another host through the app) | ✅ |
| Files of a session's folder | the agent's own tools | | | — |

## Control itself

| Feature | Reads | Changes | Other host | |
|---|---|---|---|---|
| What sessions did through control | `control log` (`control_log`) | | `--host` | ✅ |

## GOA itself

| Feature | Reads | Changes | Other host | |
|---|---|---|---|---|
| Handing a task to a session, routed ([goa.md](goa.md)) | | `goa delegate` (`goa_delegate`) | `--host` | ✅ |
| The task board | `goa board` (`goa_board`) | `goa task` (`goa_task`) | its own host's | ✅ |
| Its own RAG collection, projects' read-only | `goa recall` (`goa_recall`) | `goa remember` (`goa_remember`) | projects' through the app | ✅ |
| When a task counts as stalled | `goa stall` (`goa_stall`) | `goa stall --blank\|--idle\|--stuck` (`goa_stall`) | this host | ✅ |
| The agents it runs on, their level | `goa agents` (`goa_agents`) | | this host | ✅ |
| What a watched GOA did itself | `goa flags` | reverting: the user's, in the app | this host | — the user's |

## Built-in skills

`goa skill` (`goa_skill`) prints guides that ship in the program, on every host and for any agent, which GOA's prompt points at: `setup`, doing every setting with the user and stopping for what needs them, and `features`, every feature from the registry (`core/agent.Features`), so it can't fall behind. A test checks every command they and the prompt name exists. Decisions' and RAG's settings are reachable: `rag settings`, `rag set`, `rag project on agreed|off`, `rag plan`, `decide settings`, `decide use`, on this host; keys stay typed in the app or named from the vault.

`goa doc` (`goa_doc`) prints these docs, which ship in the program too (package `docs`, every top-level doc but the roadmap, `PLAN.md`), so a host with no repository has them: the list with a line on each, one doc, a `--section`, or `--search WORDS`. It is what GOA answers questions about a feature from, rather than the code.

## What GOA can't do yet

Everything below GOA says it can't do, and where the user finds it; it doesn't work around it. In order of use.

| What | Where it is today | Follow-up |
|---|---|---|
| A session's checks: find and run tests, linters, build | Side panel, `checks.list`, `checks.run` | `checks` command and MCP tools; until then, delegate running them |
| Review comments on a session's diff | The Git tab | Command to add and send them; until then, `send` the comments as a prompt |
| Adding, connecting, removing a host | Hosts page; the list is `hosts.json`, the password in the keychain | App command through `host.call`, a password never in the arguments |
| Removing a project; arranging projects on another host | Project's menu (`project.remove` through `host.call` only); `projects group\|move` here only | `project remove --yes`; `--host` on `projects` |
| Agents' library: skills, MCP servers, subagents, commands, agents' settings | Settings → Agents' library, Sync environment (`env.*`) | `env list\|show\|copy` |
| Signing in to another agent account, removing one | Settings → Agents (a sign-in in a terminal) | Command that opens the sign-in for the user; `accounts rm` |
| Adding a judge, and typed keys for judges, RAG and PostgreSQL | Settings → Decisions, Settings → RAG | `decide add` taking a vault credential's name; a typed key stays the app's |
| RAG's and Decisions' settings on another host | This host's commands only | `--host` on `rag` and `decide` |
| Token usage: its switch, budgets, prices | Settings (`tokens.setSettings`, `tokens.prices`) | `usage set`, `usage budget`, `usage price` |
| A session's time budget | The chat, New session (`session.budget`); the default is `settings app set time-budget` | `switch --budget` |
| A session kept private from the host's teams | Its menu (`session.private`) | `switch --private` |
| Rewinding to a past message | The chat (`session.rewind`) | `rewind ID N`, for Claude Code and Codex |
| Listing a session's slash commands | The chat (`session.commands`); `send "/…"` types one | `commands ID` |
| The model and effort a session runs on, read directly | The chat's header; `chat` shows them | A line in `status` |
| Permission prompts on agents other than Claude Code | `answer` knows Claude Code's keys only; read the screen and `send` | Keys per agent in its adapter |
| Tabs, ports and the Run menu of another host's session | `ports`, `run` here only | `--host` on both |
| Cancelling or removing a pipeline | A project's pipelines, ⋯ menu | `pipeline cancel\|rm` |
| Moving or packing a project with its sessions | Copy to host… (`transfer.*`); `project copy` copies the files | `project copy --sessions` |
| Starting a session from a GitHub issue | New session (`project.issues`) | `spawn --issue N` |
| Setting a server up like this device | Set up… (`setup.scan`) | `setup scan\|plan`, running in a session |
| Creating a server at a provider | Hosts → Create a server… (the provider's token) | `vps create`, the token named from the vault |
| The daemon's log | Settings (`daemon.log`) | `daemon log`, masked |
| Resource usage of a host | The rail's Resource usage (`daemon.stats`) | `stats` |
| Keeping the app in the tray, looking for updates | Settings → App | Fields in `app.settings` |

**Stays the user's, with no follow-up:** restarting a daemon; the vault's master password, adding a key and typing one (never in the chat or a command); agreeing to a project's files going to an embedder (`rag project on agreed` records what they said); the rules for GOA (`goa rules` reads them); taking over a teammate's session; reverting what a watched GOA did.

**The app's own look and feel, with no command planned:** the floating control, Discord, the sidebar, the keyboard shortcuts. Theme, accent and terminal font are commands: `settings app set`, with the app open.
