# Agent presets

How enowX starts each agent (`core/agent`). Sessions start with permission prompts skipped when **Skip permission prompts** is on in the New session dialog (the default), and keep that setting when the daemon restarts them with **continue**.

| Agent | Command | First prompt | Skip permission prompts | Continue last conversation | Source |
|---|---|---|---|---|---|
| Claude Code | `claude` | last argument | `--dangerously-skip-permissions` | `--continue` (per directory) | [CLI reference](https://code.claude.com/docs/en/cli-reference), [permission modes](https://code.claude.com/docs/en/permission-modes) |
| Codex | `codex` | last argument | `--dangerously-bypass-approvals-and-sandbox` (alias `--yolo`) | `resume --last` (filtered to the working directory) | `codex-rs/utils/cli/src/shared_options.rs`, `codex-rs/cli/src/main.rs` in openai/codex |
| OpenCode | `opencode` | `--prompt` | `--auto` (explicit `deny` rules still apply) | `--continue` | `packages/web/src/content/docs/{cli,permissions}.mdx` in sst/opencode |
| Gemini CLI | `gemini` | `--prompt-interactive` | `--yolo` | `--resume latest` (per project root) | `docs/reference/configuration.md`, `docs/cli/session-management.md` in the npm package |
| Kiro CLI | `kiro-cli chat` | last argument | `--trust-all-tools` | `--resume` (per directory) | **Unverified**: Kiro's docs were unreachable; flags are those of its predecessor's open-source `chat` command |

More agents, started the same way; their flags are those their documentation gives, not all checked by running them. Agents that take no first prompt on their command line get it typed in, as a paste, once their screen settles. An agent found under its alias runs as that. A program that only shares an agent's name doesn't count as installed: the database tool `goose` (`goose --version` says `goose version: v…`) isn't the Goose agent.

| Agent | Command | First prompt | Skip permission prompts | Continue last conversation | Source |
|---|---|---|---|---|---|
| Claude Agent Teams | `claude` | last argument | `--dangerously-skip-permissions` · always `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` | `--continue` | [docs](https://code.claude.com/docs/en/agent-teams) |
| OpenClaude | `openclaude` | last argument | `--dangerously-skip-permissions` | `--continue` | [docs](https://openclaude.gitlawb.com/) |
| Grok | `grok` | `--` | `--permission-mode bypassPermissions` | — | [docs](https://x.ai/cli) |
| GitHub Copilot | `copilot` | `-i` | `--yolo` | — | [docs](https://docs.github.com/en/copilot/how-tos/set-up/install-copilot-cli) |
| OpenCode 2 | `opencode2 --standalone` | `--prompt` | — | — | [docs](https://opencode.ai/v2/docs/) |
| MiMo Code | `mimo` | `--prompt` | — | — | [docs](https://mimo.xiaomi.com/coder) |
| Ante | `ante` | typed in once it shows its input | `--yolo` | — | [docs](https://github.com/AntigmaLabs/ante-preview) |
| Trae | `traecli` | `--` | `--yolo` | — | [docs](https://docs.trae.cn/cli_get-started-with-trae-cli) |
| Muse | `muse --trust-workspace` | typed in once it shows its input | `--yolo` | — | [docs](https://dev.meta.ai/docs/muse-code) |
| DeepSeek Harness | `dsh-tui .` (also `dst`) | typed in once it shows its input | — | — | [docs](https://deepseek-harness.github.io/deepseek-harness/) |
| Qoder CLI | `qodercli` (also `qoder`) | `--prompt-interactive` | `--dangerously-skip-permissions` | — | [docs](https://docs.qoder.com/cli/overview) |
| Qoder CLI China | `qoderclicn` (also `qodercn`) | `--prompt-interactive` | `--dangerously-skip-permissions` | — | [docs](https://docs.qoder.cn/cli/overview) |
| ZCode | `zcode` | typed in once it shows its input | `--mode yolo` | — | [docs](https://zcode.z.ai/en/docs) |
| Pi | `pi` | last argument | — | — | [docs](https://pi.dev) |
| OMP | `omp` | last argument | `--auto-approve` | `--continue` | [docs](https://omp.sh) |
| Prime Agent | `prime-agent` | `--` | — | — | [docs](https://github.com/PrimeIntellect-ai/prime-agent) |
| Antigravity | `agy` | `--prompt-interactive` | `--dangerously-skip-permissions` | — | [docs](https://antigravity.google/docs/cli-overview) |
| Aider | `aider` | typed in once it shows its input | `--yes-always` | — | [docs](https://aider.chat/docs/) |
| Goose | `goose --version` | typed in once it shows its input | `GOOSE_MODE=auto` in its environment · always `GOOSE_MODE=auto` | — | [docs](https://block.github.io/goose/docs/quickstart/) |
| Amp | `amp` | typed in once it shows its input | `--dangerously-allow-all` | — | [docs](https://ampcode.com/manual#install) |
| Kilo Code | `kilo` | typed in once it shows its input | — | — | [docs](https://kilo.ai/docs/cli) |
| Crush | `crush` | typed in once it shows its input | `--yolo` | — | [docs](https://github.com/charmbracelet/crush) |
| Auggie | `auggie` | typed in once it shows its input | — | — | [docs](https://docs.augmentcode.com/cli/overview) |
| Autohand Code | `autohand` | typed in once it shows its input | `--unrestricted` | — | [docs](https://github.com/autohandai/code-cli) |
| Cline | `cline` | typed in once it shows its input | `--auto-approve true` | — | [docs](https://docs.cline.bot/cline-cli/overview) |
| Codebuff | `codebuff` | typed in once it shows its input | — | — | [docs](https://www.codebuff.com/docs/help/quick-start) |
| Freebuff | `freebuff` | typed in once it shows its input | — | — | [docs](https://freebuff.com/cli) |
| Command Code | `command-code --trust` | last argument | `--yolo` | — | [docs](https://commandcode.ai/docs/quickstart) |
| Continue | `cn` | typed in once it shows its input | `--allow *` | — | [docs](https://docs.continue.dev/guides/cli) |
| Cursor | `cursor-agent` | last argument | `--yolo` | — | [docs](https://cursor.com/cli) |
| Droid | `droid` | last argument | `--auto high` | — | [docs](https://docs.factory.ai/cli/getting-started/quickstart) |
| Kimi | `kimi` (also `kimi-code`) | typed in once it shows its input | `--yolo` | — | [docs](https://www.kimi.com/code/docs/en/kimi-code-cli/getting-started.html) |
| Mistral Vibe | `vibe` (also `mistral-vibe`) | typed in once it shows its input | `--agent auto-approve` | — | [docs](https://github.com/mistralai/mistral-vibe) |
| Qwen Code | `qwen` | typed in once it shows its input | `--approval-mode yolo` | — | [docs](https://github.com/QwenLM/qwen-code) |
| Rovo Dev | `acli rovodev run` | typed in once it shows its input | `--yolo` | — | [docs](https://support.atlassian.com/rovo/docs/install-and-run-rovo-dev-cli-on-your-device/) |
| Hermes | `hermes --tui` | `--query` | `--yolo` | — | [docs](https://hermes-agent.nousresearch.com/docs/) |
| Devin | `devin` | typed in once it shows its input | `--permission-mode bypass --respect-workspace-trust false` | — | [docs](https://devin.ai/cli) |
| OpenClaw | `openclaw` | typed in once it shows its input | — | — | [docs](https://github.com/openclaw/openclaw) |
| CodeBuddy | `codebuddy` (also `cbc`) | last argument | `--dangerously-skip-permissions` | — | [docs](https://www.codebuddy.ai/cli) |
| Jcode | `jcode` | typed in once it shows its input | — | — | [docs](https://github.com/1jehuang/jcode) |

### Trusting the folder

Many agents ask, the first time they start in a folder, whether to trust it, and take no prompt until answered. The folder is the one picked for the session in enowX, which is that decision made, so for its first 30 seconds the daemon watches a new agent's screen and answers once: a question with "trust" in it whose selected option (marked `>`, `❯`, `›` or `▶`) says yes or trust gets Enter (`agent.TrustAsked`). A first prompt typed in waits for that. Checked on CodeBuddy Code and Antigravity CLI.

## Installing

When an agent is missing, the New session dialog offers to install it: enowX shows the command, then runs it in a session on that host, as the user enowX runs as, so its output and questions stay visible. The daemon reads the login shell's PATH again when the agent list is asked for, so the new agent shows up without a restart.

| Agent | Install command | Source |
|---|---|---|
| Claude Code | `curl -fsSL https://claude.ai/install.sh \| bash` | README of anthropics/claude-code |
| Codex | `curl -fsSL https://chatgpt.com/codex/install.sh \| sh` | README of openai/codex |
| OpenCode | `curl -fsSL https://opencode.ai/install \| bash` | `docs/index.mdx` in sst/opencode |
| Gemini CLI | `npm install -g --prefix "$HOME/.local" @google/gemini-cli` | README of google-gemini/gemini-cli (`npm install -g`); the prefix avoids needing root |
| Kiro CLI | `curl -fsSL https://cli.kiro.dev/install \| bash` | Kiro's CLI install page, as given by the user (unreachable from enowX's build machine) |
| Aider, Amp, Goose, Cursor, Droid | their official install scripts (`core/agent/presets.go`) | each agent's install docs |
| GitHub Copilot, Qwen Code, Crush, Auggie, Cline, Codebuff, Kilo Code, Continue | `npm install -g --prefix "$HOME/.local" <package>` | each agent's npm package |

Agents with no install command enowX knows link to their documentation instead.

## Signing in

Agents run inside a small `/bin/sh` wrapper (PowerShell on Windows) that starts the login shell once the agent exits, so quitting it with Ctrl+C, or a failed sign-in, leaves a prompt in the same folder. The New session dialog's **Sign in** runs the command below in the project folder, which prints a link and a code to open on any device; this is what works on a server without a browser.

| Agent | Sign-in command | Source |
|---|---|---|
| Claude Code | `claude auth login` | Claude Code docs (`claude auth status`), error guides; prints a URL and asks for the code |
| Codex | `codex login --device-auth` | Codex auth docs; a ChatGPT workspace admin must allow device codes |
| OpenCode | `opencode auth login` | OpenCode CLI docs; most providers take an API key |
| Gemini CLI | `NO_BROWSER=true gemini` | Gemini CLI authentication docs; it has no login command, its first screen signs in |
| Kiro CLI | `kiro-cli login --use-device-flow` | Kiro CLI's own error message; Builder ID and IAM Identity Center only, Google and GitHub sign-in are refused remotely |

## Status

The session list shows each agent as working, needing input, done with its turn, or idle. Agents with hooks report it themselves: enowX adds its hooks when it starts them, and they run `"$ENOWX_BIN" hook <state>` (or `hook --event E --state S`, which also hands the user's hooks the event: [hooks.md](hooks.md)), where the daemon sets `ENOWX_BIN` to its own program in every session, so nothing changes in the user's own configuration and the hooks do nothing outside enowX. The other agents, and these between reports, go by their screen: one that changed within the last 1.5 s is working.

| Agent | How | Reports | Source |
|---|---|---|---|
| Claude Code | `--settings '{"hooks": …}'`, merged with the user's settings | `UserPromptSubmit`, `PostToolUse`, `PostToolUseFailure` → working; `Notification` matching `permission_prompt\|elicitation_dialog` → needs input; `Stop` → done | Claude Code hooks docs; checked on 2.1.314 |
| Codex | `-c notify=[…]`: a `/bin/sh` wrapper that reports done, then runs the user's own `notify` from `config.toml` with Codex's argument; `-c hooks.<Event>=[…]`, trusted by enowX in `config.toml` | end of each turn → done; `UserPromptSubmit`, `PostToolUse` → working; `PermissionRequest` → needs input | Codex `notify` config (`agent-turn-complete`), Codex hooks docs; checked on 0.164.0 |
| Every other agent | — | screen only | |

Typing into an agent that asked something, or pressing Enter after it finished, ends that state; a working agent whose screen stood still for 15 s (interrupted, say) goes back to its screen. Hooks are POSIX shell, so on Windows every agent goes by its screen.

When an agent comes to need input or finishes its turn in a session you aren't looking at, the app shows a desktop notification; clicking it opens the session.

### Unread

A session whose agent finishes its turn or asks something while the user doesn't see it (another session shown, another page, or the window not focused) turns unread: its name bold, with a dot, until it is shown. A tab's agent marks its session. **Mark as unread** in a session's menu keeps one to come back to.

### Hibernation

After 30 minutes by default. Settings → New sessions → **Hibernate idle agents** (never, 15 minutes, 30 minutes, 1 hour, 4 hours; `DaemonSettings.HibernateAfter`, per host) lets the daemon stop an agent that its hooks say is done and that has shown and been typed nothing for that long: its program ends, what it started too, and its session stays, marked hibernated, showing what it showed. Showing the session continues it (`session.continue`), the agent coming back on its conversation. Only agents enowX can continue (Claude Code, Codex, OpenCode, Gemini CLI, Kiro CLI, …) and not tabs; agents without hooks never count as done, so they never hibernate. Nor does an agent whose session still serves a port (a dev server it started) or runs a desktop app on a virtual display: they would stop with it.

#### By hand

A session's menu (right click in the session list) has **Hibernate** (⌘⇧H) and, for one hibernated, **Wake**; with several sessions selected (⌘-click or ⇧-click) it hibernates them all, and the project's menu has **Hibernate all idle**. Before stopping work, the app asks: an agent working (hibernate now, once it is done, or cancel), a port served, an app on a display, a program left running in the background, each listed with the memory it holds and what hibernating frees (`session.hibernate`, its `dryRun` plan). Agents' own helpers (their MCP servers, say) stop with them without asking. A session hibernated by hand is marked so in the list, with since when (`SessionInfo.HibernatedHow`); showing it, or **Wake** (`session.wake`), brings it back. It works on any host the app reaches, through its daemon.

**Don't hibernate on its own** in a session's menu keeps the daemon from hibernating it (`session.autoHibernate`, `SessionInfo.NoAutoHibernate`); Settings → New sessions sets it for new sessions, the same on every host (`settings/hibernate.json`, synced with the user's data).

From an agent: `enowx-app hibernate ID... [--idle] [--when-done] [--force] [--dry-run]`, `enowx-app wake ID`, `hibernate ID --auto off|on`, `--auto-default off|on` (MCP `session_hibernate`, `session_wake`); an agent asking has the user confirm in the app, and can't hibernate its own session. `ls` shows `hibernated` or `hibernated(by hand)`.

## What agents are told

Every agent enowX starts is told what the session gives it (`agent.Guide`, which `enowx-app guide` prints): dev servers show in the Browser tool by themselves and it can drive that Browser, a desktop app on a server needs `eval "$("$ENOWX_BIN" display)"` first (see [preview.md](preview.md)), the HTTP traffic of what it runs can be caught, read and mocked ([traffic.md](traffic.md)), Android devices can run and log its app ([android.md](android.md)), other agents can be started and followed, and `note` keeps the user posted. The same commands are MCP tools of `enowx-app mcp`, whose instructions are the guide.

It goes the way each agent takes standing instructions, and adds to the user's own, never replacing them: flags that replace an agent's system prompt are never used.

| Agent | How |
|---|---|
| Claude Code, Claude Agent Teams, OpenClaude, Qoder CLI, OMP, Prime Agent, Droid, Qwen Code, CodeBuddy | `--append-system-prompt` |
| Codex | `-c developer_instructions=…`, after the user's own `developer_instructions` from `config.toml`, which `-c` would replace |
| Autohand Code | `--append-sys-prompt` |
| Grok | `--rules` |
| Continue | `--rule` |
| Aider | `--read ~/.enowx-app/agent-guide.md` |
| Auggie | `--rules ~/.enowx-app/agent-guide.md` |
| Kiro CLI | `~/.kiro/steering/enowx.md` |
| GitHub Copilot | `~/.copilot/instructions/enowx.instructions.md` |
| Antigravity | `~/.gemini/config/rules/enowx.md` |
| Cline | `~/.cline/rules/enowx.md` |
| Devin | `~/.devin/rules/enowx.md` |
| Gemini CLI | a block of `~/.gemini/GEMINI.md` |
| OpenCode | a block of `~/.config/opencode/AGENTS.md`, or of `~/.claude/CLAUDE.md` when only that is there (OpenCode reads the first of them, so making the first would hide the user's) |
| Ante | the same with `~/.ante/AGENTS.md` |
| MiMo Code, ZCode, Amp, Command Code, Mistral Vibe, Rovo Dev | a block of `~/.config/mimocode/AGENTS.md`, `~/.zcode/AGENTS.md`, `~/.config/amp/AGENTS.md`, `~/.commandcode/AGENTS.md`, `~/.vibe/AGENTS.md`, `~/.rovodev/AGENTS.md` |
| Pi, Goose, Crush, Jcode | a block of `~/.pi/agent/APPEND_SYSTEM.md`, `~/.config/goose/.goosehints`, `~/.config/crush/CRUSH.md`, `~/.jcode/prompt-overlay.md` |
| Cursor, Kimi, Kilo Code, OpenCode 2, Trae, Muse, DeepSeek Harness, Codebuff, Freebuff, Hermes, OpenClaw | none they read outside the project: a line after the first prompt says `"$ENOWX_BIN" guide` lists the tools |

Files are written, or brought up to date, when the daemon starts the agent. enowX's own files (`enowx.md`) hold the whole guide. In a file of the user's, enowX keeps only a short block between `<!-- enowX: … -->` lines, saying to run `"$ENOWX_BIN" guide`, and leaves the rest as it is; deleting the block is undone at the agent's next start. Both apply only when `ENOWX_SESSION` is set, since the agent reads them outside enowX too. The places are those the agents' documentation or source gives, not all checked by running them.

## Limits

**Agents at once** (command palette) sets how many agents may work at the same time on a host, kept by its daemon in `~/.enowx-app/settings.json`. An agent session started past it is created at once, with its worktree, but shows as **queued** and runs nothing; the daemon starts it in that session when an agent finishes its turn (as its hooks or its screen tell), oldest first. An agent just started counts as working for 10 s, so the next one doesn't start before it begins. Shells and custom commands are never queued. A daemon restart forgets the queue: the sessions waiting are gone.

**Set time budget…** (a session's menu, or the palette) sets how long its agent may work, counting only while it works. Past it, the daemon interrupts the agent as its user would (Escape for Claude Code and Codex, Ctrl+C for the others), the session says so, and the app notifies. A new budget lifts it. The work counted starts again when the daemon restarts. Budgets are in time only: what an agent costs isn't known for every agent (see Credits).

## Credits

The daemon reads what each signed-in account has left (`core/credits`, `credits.get`) and the app shows it in the title bar chip, its popover and the Credits page. Tokens are read when needed and sent only to the vendor that issued them; enowX never stores, logs or refreshes them, since refreshing would sign the agent out. A reading is reused for 5 minutes. All three sources are undocumented and may change; a changed answer shows as an error on the card.

| Agent | Login read | Asked |
|---|---|---|
| Claude Code | macOS keychain items `Claude Code-credentials*`, else `~/.claude/.credentials.json` | `GET api.anthropic.com/api/oauth/usage` (`anthropic-beta: oauth-2025-04-20`) |
| Codex | `$CODEX_HOME/auth.json` (`~/.codex`) | `GET chatgpt.com/backend-api/wham/usage` with `ChatGPT-Account-Id` |
| Kiro CLI | its own | `kiro-cli chat --no-interactive /usage`, parsed; account from `kiro-cli whoami --format json` |

## Caveats

- **Claude Code refuses `--dangerously-skip-permissions` as root.** An address without a user signs in as root, and enowX then runs its daemon, and so every session, as a regular user of its own, `enowx`, made the first time (or the one it ran as before), through `su`. Sign in to each agent once from a Shell session as that user. The choice is saved per host. To run as root, or as another user, pick it in **Hosts › ⋯ › Edit… › Run sessions as** (*A regular user* with its name, or *root*) and save: enowX connects again as it says, and keeps it on every connection after. **Hosts › ⋯ › Ask which user to run as** makes the next connection ask instead, listing the server's users. Sessions started as one user stay with that user's daemon. If you keep root, enowX reports Claude Code's refusal to skip permission prompts instead of starting the session.
- **Gemini CLI enables its sandbox with `--yolo`** (a Docker image by default). Without Docker, set `GEMINI_SANDBOX=false` in the server's environment, or turn sandboxing off in `~/.gemini/settings.json`, if that is what you want.
- Skipping permission prompts lets an agent run any command and edit any file the user can. Worktrees isolate files between sessions, not from the rest of the machine.

## Forking a session

A session's menu (**Fork…**, which warns first), `enowx-app fork SESSION [PROMPT]` and MCP `session_fork` start a new session, "NAME (fork)", from a compacted copy of its conversation, so a long session's context goes on fresh and the original stays. The agent copies the conversation (`claude --resume ID --fork-session`, `codex fork ID`) and `/compact` runs in the copy, so the old session's history, state, branch and folder aren't touched. It takes the same folder and branch, as the owner decided: two agents there can overwrite each other's changes, which the dialog and the command say. A session whose agent stopped is brought back first; one working is refused until its turn ends. `/compact` is typed only once the copy shows its prompt, and success is read from its conversation. If it can't start or compact, the new session goes and the error says why. Other agents answer that it isn't supported yet.
