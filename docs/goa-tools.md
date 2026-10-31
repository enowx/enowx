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
| One prompt to several agents (fan-out) | | `spawn --agents` | `--host` | ✅ |
| Send a prompt or keys | | `send` (`session_send`) | `--host` | ✅ |
| Its screen | `read` (`session_read`) | | `--host` | ✅ |
| Its chat (transcript) | `chat` (`session_chat`), `peek` | | `--host`, own | ✅ |
| Wait for it | `wait` (`session_wait`) | | `--host` | ✅ |
| Interrupt it (Escape) | | `interrupt` (`session_interrupt`) | `--host` | ✅ |
| Stop it, continue it | | `stop`, `continue` (`session_stop`, `session_continue`) | `--host` | ✅ |
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
| RAG: consent, index | `rag status` (`rag_status`) | `rag index` | this host | ◐ consent and RAG's settings: the app's Settings → RAG only |
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
| Decisions (the judge): policy, judges, uses | `decide status\|log` (`decide_status`, `decide_log`) | `decide mark` | this host | ◐ the policy and judges: Settings → Decisions only |
| RAG: on, server, consent | `rag status` | | this host | ✗ Settings → RAG only |
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

## Gaps left, in order of use

1. Checks (`checks.list`, `checks.run`) and review comments: a reviewer GOA would use them.
2. Decisions' and RAG's settings (their policy, judges, server, consent): they name vault credentials, so they need care.
3. The agents' library (`env.*`): skills and MCP servers copied between homes.
4. A session's budget, privacy, slash commands; pipelines cancelled; projects arranged and removed on another host.
5. The daemon's log; setup and VPS creation; resource usage.
