# Agent presets

How enowX starts each agent (`core/agent`). Claude Code's sessions always run in `bypassPermissions` ([below](#claude-code-always-runs-in-bypass)). Other agents' sessions start with permission prompts skipped when **Skip permission prompts** is on in the New session dialog (the default), and keep that setting when the daemon restarts them with **continue**.

| Agent | Command | First prompt | Skip permission prompts | Continue last conversation | Source |
|---|---|---|---|---|---|
| Claude Code | `claude` | last argument | `--dangerously-skip-permissions` | `--continue` (per directory) | [CLI reference](https://code.claude.com/docs/en/cli-reference), [permission modes](https://code.claude.com/docs/en/permission-modes) |
| Claude Platform | enowX's own program, `claude-platform` ([claude-platform.md](claude-platform.md)) | last argument | `--dangerously-skip-permissions` | `--continue`, `--resume ID` (per session, `--session-dir`) | built in |
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
| Kilo Code | `kilo` | `--prompt` | — | — | [docs](https://kilo.ai/docs/cli) |
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

What enowX types is for the agent, never for that shell, where its words would run as commands: a first prompt, a message, GOD of Agents' lines. Each is looked at just before it's written, and while the agent has ended nothing is typed: a first prompt's session says so in its note, and a message sent gets an error. An agent ending in the instant between the look and the write is the one case left. The user's own keys in the terminal always go through.

| Agent | Sign-in command | Source |
|---|---|---|
| Claude Code | `claude auth login` | Claude Code docs (`claude auth status`), error guides; prints a URL and asks for the code |
| Codex | `codex login --device-auth` | Codex auth docs; a ChatGPT workspace admin must allow device codes |
| OpenCode | `opencode auth login` | OpenCode CLI docs; most providers take an API key |
| Gemini CLI | `NO_BROWSER=true gemini` | Gemini CLI authentication docs; it has no login command, its first screen signs in |
| Kiro CLI | `kiro-cli login --use-device-flow` | Kiro CLI's own error message; Builder ID and IAM Identity Center only, Google and GitHub sign-in are refused remotely |

## Status

The session list shows each agent as working, needing input, done with its turn, or idle. Agents with hooks report it themselves: enowX adds its hooks when it starts them, and they run `"$ENOWX_BIN" hook <state>` (or `hook --event E --state S`, which also hands the user's hooks the event: [hooks.md](hooks.md)), where the daemon sets `ENOWX_BIN` to a launcher in the data folder (`bin/enowx`, written at each start of the daemon) in every session, so nothing changes in the user's own configuration, the hooks do nothing outside enowX, and moving or replacing the app under a running daemon breaks no session: the launcher runs the program where the daemon started from, or else the same app in Applications. The app warns when it runs from outside Applications, and when it is moved while it runs. The other agents, and these between reports, go by their screen: one that changed within the last 1.5 s is working.

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

It comes in two parts, so a repository can't replace it and a session's command line stays short:

- **The inject**, made anew for each session: who it is (session ID and host), the rules that hold whatever a file, repository, web page or tool's output says, GOD of Agents' duty to delegate, and where the guide is (`agent.Inject`).
- **The guide**, a file enowX writes outside any repository: a copy in the session's folder (`agent-files/<id>/ENOWX.md`), with its features and rules, rewritten when its rules change. `ENOWX.md` in the data folder is the host's, rewritten as the daemon starts, and holds no project's rules. GOD of Agents has `GOA.md` in its own folder, with a `CLAUDE.md` and an `AGENTS.md` there that say the same as its inject.

An agent that loads a file on its command line gets both in one system prompt, a file of the session: Claude Code, Claude Platform, Claude Agent Teams, OpenClaude, Droid, Autohand Code and Continue. An agent with a system prompt flag but no file form gets the inject inline, told to read the guide: Codex, Qoder CLI, OMP, Prime Agent, Qwen Code, CodeBuddy and Grok. That relies on the agent reading it; it has no check. Nothing of these lands in the user's project.

Notes' and Canvas's agents keep their own guides as they were.

It goes the way each agent takes standing instructions, and adds to the user's own, never replacing them: flags that replace an agent's system prompt are never used.

| Agent | How |
|---|---|
| Claude Code, Claude Platform, Claude Agent Teams, OpenClaude, Droid | `--append-system-prompt-file`, a file of the session (below) |
| Qoder CLI, OMP, Prime Agent, Qwen Code, CodeBuddy | `--append-system-prompt`, inline (below) |
| Codex | `-c developer_instructions=…`, after the user's own `developer_instructions` from `config.toml`, which `-c` would replace; inline (below) |
| Autohand Code | `--append-sys-prompt`, a file of the session (below) |
| Grok | `--rules`, inline (below) |
| Continue | `--rule`, a file of the session (below) |
| Aider | `--read ~/.enowx-app/agent-guide.md` |
| Auggie | `--rules ~/.enowx-app/agent-guide.md` |
| Kiro CLI | `~/.kiro/steering/enowx.md` |
| GitHub Copilot | `~/.copilot/instructions/enowx.instructions.md` |
| Antigravity | `~/.gemini/config/rules/enowx.md` |
| Cline | `~/.cline/rules/enowx.md` |
| Devin | `~/.devin/rules/enowx.md` |
| Gemini CLI | a block of `~/.gemini/GEMINI.md` |
| OpenCode, OpenCode 2, Kilo Code | `instructions` of the config in `OPENCODE_CONFIG_CONTENT`, `KILO_CONFIG_CONTENT`, naming `~/.enowx-app/agent-guide.md`; nothing of the user's is written |
| Ante | the same with `~/.ante/AGENTS.md` |
| MiMo Code, ZCode, Amp, Command Code, Mistral Vibe, Rovo Dev | a block of `~/.config/mimocode/AGENTS.md`, `~/.zcode/AGENTS.md`, `~/.config/amp/AGENTS.md`, `~/.commandcode/AGENTS.md`, `~/.vibe/AGENTS.md`, `~/.rovodev/AGENTS.md` |
| Pi, Goose, Crush, Jcode | a block of `~/.pi/agent/APPEND_SYSTEM.md`, `~/.config/goose/.goosehints`, `~/.config/crush/CRUSH.md`, `~/.jcode/prompt-overlay.md` |
| Cursor, Kimi, Trae, Muse, DeepSeek Harness, Codebuff, Freebuff, Hermes, OpenClaw | none they read outside the project: a line after the first prompt says `"$ENOWX_BIN" guide` lists the tools, and a block in the project (below) |

Files are written, or brought up to date, when the daemon starts the agent. enowX's own files (`enowx.md`) hold the whole guide. In a file of the user's, enowX keeps only a short block between `<!-- enowX: … -->` lines, saying to run `"$ENOWX_BIN" guide`, and leaves the rest as it is; deleting the block is undone at the agent's next start. Both apply only when `ENOWX_SESSION` is set, since the agent reads them outside enowX too. The places are those the agents' documentation or source gives, not all checked by running them.

### OpenCode 1 and 2

OpenCode 2 is another program (`@opencode/cli`) that may answer to the same `opencode` command. It runs one background server per user, which owns its configuration, so what a session's environment says (a config's provider and model, enowX's guide and tools) is ignored once that server runs. So enowX starts `opencode` with `--standalone` when it reports version 2 or later, giving the session a private server and leaving the user's alone; 1.x doesn't start with that flag. A session's config holds the provider in both versions' forms. A plugin named in the user's own OpenCode config loads as it does anywhere: enowX adds none. A session started this way doesn't show in other clients of the shared server.

### The block in a project

The agents with no other way get a block at the top of the project's `AGENTS.md`, or its `CLAUDE.md` if that is the one it has, between `<!-- enowx:start -->` and `<!-- enowx:end -->`. It says where the guide is, by no path of this machine's, and applies only when `ENOWX_SESSION` is set. The rest of the file stays as it was, and an update rewrites the block rather than adding another. **On by default; a tracked file shows it in `git diff`, and it would be committed with it.** A new `AGENTS.md` it makes goes in `.git/info/exclude` instead. `enowx-app project settings --guide-block off` (`[guide] block = false` in `.enowx.toml`) takes it out. No other agent writes into a project.

### enowX's tools as MCP

Every agent with a documented way to add an MCP server to one session gets enowX's, GOD of Agents' session included: Claude Code, Claude Platform, Claude Agent Teams, OpenClaude, Kimi and Copilot on their command line, Codex through `-c`, OpenCode and Kilo Code through their config's environment variable. The others have none enowX can use without writing the user's own settings: they use the `"$ENOWX_BIN"` commands, which any agent that runs a shell has.

### What each agent gets

### Long arguments go in files

A guide of real size is tens of thousands of characters, with a project's rules and GOA's more. A Windows command line can't hold that: an agent installed by npm is a `.cmd` there, which starts through cmd.exe, and cmd.exe stops at 8,191 characters ("The command line is too long"). So what an agent takes in a file goes in one, on every system, in the session's own folder: `~/.enowx-app/agent-files/<session>/` on the host where the agent runs, as the daemon writes it. The folder is `0700` and the files `0600` (on Windows, which has no such modes, they take the access of the user's profile). A fresh copy is written at each start, resume, wake and restart, also for a session saved with its guide inline, whose command is built anew; the folder goes when the session is closed. Nothing lands in the project.

| Agent | Guide | MCP config | Settings | Source |
|---|---|---|---|---|
| Claude Code, Claude Agent Teams | `--append-system-prompt-file` | `--mcp-config` with a path | `--settings` with a path | [CLI reference](https://code.claude.com/docs/en/cli-reference.md); run in print and interactive mode on 2.1.317 |
| OpenClaude | `--append-system-prompt-file` | `--mcp-config` with a path | none passed | Its docs say nothing of it; its CLI's own options in `@gitlawb/openclaude` 0.32.0 |
| Droid | `--append-system-prompt-file` | none passed | none passed | [CLI reference](https://docs.factory.com/reference/cli-reference) |
| Autohand Code | `--append-sys-prompt` with a path ("inline string or file path") | none passed | none passed | [README](https://github.com/autohandai/code-cli) |
| Continue | `--rule` with a path ("a file path, hub slug, or string content") | none passed | none passed | Its CLI's own options in `@continuedev/cli` 1.5.47; its docs' pages don't list it |
| Kimi | none: a line after the first prompt | `--mcp-config-file` | none passed | [command](https://moonshotai.github.io/kimi-cli/en/reference/kimi-command.html) |
| GitHub Copilot | a file of its home | `--additional-mcp-config=@path` | none passed | [CLI reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference) |
| Codex | **No file form:** inline. `model_instructions_file` replaces its built-in instructions, and profiles live in `$CODEX_HOME` | short, inline (`-c mcp_servers.enowx.*`) | hooks inline, none on Windows | [config reference](https://developers.openai.com/codex/config-reference) |
| Qoder CLI, Qoder CLI China | **No file form:** inline | none passed | none passed | [CLI reference](https://docs.qoder.com/cli/cli-reference) |
| Qwen Code | **No file form:** inline. `QWEN_SYSTEM_MD` replaces its prompt | none passed | none passed | [settings](https://qwenlm.github.io/qwen-code-docs/en/users/configuration/settings/) |
| CodeBuddy | **No file form:** inline. `--system-prompt-file` replaces its prompt, in print mode only | none passed | none passed | [CLI reference](https://www.codebuddy.ai/docs/cli/cli-reference) |
| OMP, Prime Agent | **None documented:** their READMEs don't list the flag enowX passes, inline | none passed | none passed | [OMP](https://github.com/can1357/oh-my-pi), [Prime Agent](https://github.com/PrimeIntellect-ai/prime-agent) |
| Grok | **None documented:** `x.ai/cli` couldn't be read and its other docs don't list `--rules`, inline | none passed | none passed | [docs](https://docs.x.ai/build/overview) |
| OpenCode, Kilo Code | A file already, named in `OPENCODE_CONFIG_CONTENT` / `KILO_CONFIG_CONTENT` | in the same variable, short | none passed | the table below |

**An agent with no file form keeps its guide inline where that fits.** On Windows, when its command would pass 7,500 characters, enowX writes the guide to the session's folder and passes a line in its place, telling the agent to read that file first. The session starts, its note says so and the daemon logs it, but the agent has the guide only if it reads the file. On macOS and Linux nothing changes for these agents.

**Other long arguments, on Windows:**

- A **first prompt** too long for the command line, as a long delegated task can be, is typed once the agent shows its input instead.
- **Project rules** and **GOA's rules** are part of the guide, so they go where it goes.
- **`--settings`** isn't passed on Windows but for a config's, and goes in its file. It never holds a key, which stays in the environment.
- **The environment** isn't part of the command line, and the one long variable enowX sets (`OPENCODE_CONFIG_CONTENT`) is a few hundred characters.
- **`--add-dir`** of the Canvas page's projects stays on the command line. Past the limit the session's note says it may not start.

A secret in the guide's text (what `rag.HoldsSecret` finds, in rules the user wrote) is taken out before it is written anywhere, on every system, and the daemon logs that it was.

**Real sessions:** each agent marked Checked ran in a real session of an isolated daemon, with a temporary home, against a local stand-in model that records what the agent sends: its system prompt, the tools it offers, and the results of the calls the stand-in asks for. So it shows what the agent received and could run, not what a real model makes of it, and agents were signed in to nothing. The rest are not installed on the machine this was run on, or need an account of their own, and aren't counted as working: start one, ask who it is in enowX, have it name its enowX tools and call one. Sessions on the Notes and Canvas pages and GOD of Agents have tools of their own, and a teammate's session gets no MCP server from here.

| Agent | Prompt | Tools | Knowledge | Source | Real session |
|---|---|---|---|---|---|
| Claude Code | `--append-system-prompt-file` | `--mcp-config` with a file; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | [CLI reference](https://code.claude.com/docs/en/cli-reference) | Checked, 2.1.318: guide in the system prompt, enowX's MCP tools listed and loaded, a canvas tool called, `"$ENOWX_BIN"` run |
| Claude Platform | `--append-system-prompt-file` | `--mcp-config` with a file; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | built in | Not checked |
| Codex | `-c developer_instructions=`, after the user's own | `-c mcp_servers.enowx.*`; the `"$ENOWX_BIN"` commands | The inject, which names `ENOWX.md`: the guide and project rules, read by the agent | `codex-rs` config, rust-v0.166.0 | Checked, 0.166.0: guide, 93 MCP tools, one called, `"$ENOWX_BIN"` run |
| OpenCode | `instructions` in `OPENCODE_CONFIG_CONTENT`, naming enowX's file | `mcp` in `OPENCODE_CONFIG_CONTENT`; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | `config.mdx`, `mcp-servers.mdx` in sst/opencode | Checked, 1.19.5: guide, 93 MCP tools, one called, `"$ENOWX_BIN"` run |
| Gemini CLI | a block of `~/.gemini/GEMINI.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | `docs/reference/configuration.md` in google-gemini/gemini-cli | Partly, 0.66.0: guide arrives; the stand-in's answer with a tool call failed to parse, so no tool ran |
| Kiro CLI | enowX's `~/.kiro/steering/enowx.md` | None for one session; the `"$ENOWX_BIN"` commands | The guide, whole; project rules by running `\"$ENOWX_BIN\" guide` | the preset tables above; session flags not re-read | Not checked |
| Claude Agent Teams | `--append-system-prompt-file` | `--mcp-config` with a file; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | [CLI reference](https://code.claude.com/docs/en/cli-reference) | Checked, 2.1.318: as Claude Code |
| OpenClaude | `--append-system-prompt-file` | `--mcp-config` with a file; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | the preset tables above; session flags not re-read | Not checked: stopped at its first-run screens in a new home |
| Grok | `--rules` | None for one session; the `"$ENOWX_BIN"` commands | The inject, which names `ENOWX.md`: the guide and project rules, read by the agent | the preset tables above; session flags not re-read | Not checked |
| GitHub Copilot | enowX's `~/.copilot/instructions/enowx.instructions.md` | `--additional-mcp-config=@file`; the `"$ENOWX_BIN"` commands | The guide, whole; project rules by running `\"$ENOWX_BIN\" guide` | [CLI reference](https://docs.github.com/en/copilot/reference/cli-command-reference) | Not checked |
| OpenCode 2 | None found: a line after the first prompt | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | [docs](https://opencode.ai/v2/docs/) | Not checked |
| MiMo Code | a block of `~/.config/mimocode/AGENTS.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Not checked |
| Ante | a block of `~/.ante/AGENTS.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Not checked |
| Trae | None found: a line after the first prompt | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | [docs](https://docs.trae.cn/cli_get-started-with-trae-cli); its rules page gave nothing to read | Not checked |
| Muse | None found: a line after the first prompt | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | [configuration](https://dev.meta.ai/docs/muse-code/configuration) | Not checked |
| DeepSeek Harness | None found: a line after the first prompt | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | its page gave nothing to read (drawn by script) | Not checked |
| Qoder CLI | `--append-system-prompt` | None for one session; the `"$ENOWX_BIN"` commands | The inject, which names `ENOWX.md`: the guide and project rules, read by the agent | the preset tables above; session flags not re-read | Not checked |
| Qoder CLI China | `--append-system-prompt` | None for one session; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | the preset tables above; session flags not re-read | Not checked |
| ZCode | a block of `~/.zcode/AGENTS.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Not checked |
| Pi | a block of `~/.pi/agent/APPEND_SYSTEM.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Checked, 0.73.1: guide, no MCP, `"$ENOWX_BIN"` run |
| OMP | `--append-system-prompt` | None for one session; the `"$ENOWX_BIN"` commands | The inject, which names `ENOWX.md`: the guide and project rules, read by the agent | the preset tables above; session flags not re-read | Checked, 18.16.0: guide, no MCP, `"$ENOWX_BIN"` run |
| Prime Agent | `--append-system-prompt` | None for one session; the `"$ENOWX_BIN"` commands | The inject, which names `ENOWX.md`: the guide and project rules, read by the agent | the preset tables above; session flags not re-read | Not checked |
| Antigravity | enowX's `~/.gemini/config/rules/enowx.md` | None for one session; the `"$ENOWX_BIN"` commands | The guide, whole; project rules by running `\"$ENOWX_BIN\" guide` | the preset tables above; session flags not re-read | Not checked |
| Aider | `--read` enowX's file | None for one session; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | the preset tables above; session flags not re-read | Not checked |
| Goose | a block of `~/.config/goose/.goosehints`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Not checked |
| Amp | a block of `~/.config/amp/AGENTS.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Not checked |
| Kilo Code | `instructions` in `KILO_CONFIG_CONTENT`, naming enowX's file | `mcp` in `KILO_CONFIG_CONTENT`; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | [CLI](https://kilo.ai/docs/cli), [custom rules](https://kilo.ai/docs/customize/custom-rules) | Checked, 7.8.18: guide, 93 MCP tools, one called, `"$ENOWX_BIN"` run |
| Crush | a block of `~/.config/crush/CRUSH.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Checked, 0.99.0: guide, no MCP, `"$ENOWX_BIN"` run |
| Auggie | `--rules` enowX's file | None for one session; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | the preset tables above; session flags not re-read | Not checked |
| Autohand Code | `--append-sys-prompt` with a file | None for one session; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | the preset tables above; session flags not re-read | Not checked |
| Cline | enowX's `~/.cline/rules/enowx.md` | None for one session; the `"$ENOWX_BIN"` commands | The guide, whole; project rules by running `\"$ENOWX_BIN\" guide` | the preset tables above; session flags not re-read | Not checked |
| Codebuff | None found: a line after the first prompt | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | its page gave nothing to read (drawn by script) | Not checked |
| Freebuff | None found: a line after the first prompt | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Not checked |
| Command Code | a block of `~/.commandcode/AGENTS.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Not checked |
| Continue | `--rule` with a file | None for one session; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | the preset tables above; session flags not re-read | Not checked: asked for its own key before any prompt, and it took the prompt as the key |
| Cursor | None found: a line after the first prompt | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | [CLI](https://cursor.com/docs/cli/using), [MCP](https://cursor.com/docs/cli/mcp) | Not checked |
| Droid | `--append-system-prompt-file` | None for one session; the `"$ENOWX_BIN"` commands | The guide, whole, project rules included | the preset tables above; session flags not re-read | Not checked |
| Kimi | None found: a line after the first prompt | `--mcp-config-file`; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | [command](https://moonshotai.github.io/kimi-cli/en/reference/kimi-command.html), [MCP](https://moonshotai.github.io/kimi-cli/en/customization/mcp.html) | Not checked |
| Mistral Vibe | a block of `~/.vibe/AGENTS.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Not checked |
| Qwen Code | `--append-system-prompt` | None for one session; the `"$ENOWX_BIN"` commands | The inject, which names `ENOWX.md`: the guide and project rules, read by the agent | the preset tables above; session flags not re-read | Checked, 0.26.7: guide, no MCP, `"$ENOWX_BIN"` run |
| Rovo Dev | a block of `~/.rovodev/AGENTS.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Not checked |
| Hermes | None found: a line after the first prompt | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | [context files](https://hermes-agent.nousresearch.com/docs/user-guide/features/context-files), [MCP](https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp) | Not checked |
| Devin | enowX's `~/.devin/rules/enowx.md` | None for one session; the `"$ENOWX_BIN"` commands | The guide, whole; project rules by running `\"$ENOWX_BIN\" guide` | the preset tables above; session flags not re-read | Not checked |
| OpenClaw | None found: a line after the first prompt | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | [workspace](https://docs.openclaw.ai/concepts/agent-workspace) | Not checked |
| CodeBuddy | `--append-system-prompt` | None for one session; the `"$ENOWX_BIN"` commands | The inject, which names `ENOWX.md`: the guide and project rules, read by the agent | the preset tables above; session flags not re-read | Not checked |
| Jcode | a block of `~/.jcode/prompt-overlay.md`, saying to run the guide | None for one session; the `"$ENOWX_BIN"` commands | By running `"$ENOWX_BIN" guide` | the preset tables above; session flags not re-read | Not checked |

### What each agent gets of the rules

A project's rules and GOD of Agents' have a version: the first 8 characters of their SHA-256, which the guide says beside them (`Version of these rules: …`). They are found by the project's remote, else its main folder: another spelling of the folder's case, a symlink, a subfolder or a worktree is the same project, and files saved under an earlier spelling are still found, and moved at the next save. A folder in no repository also reads the rules of a folder above it, up to the user's home, which passes on none.

Each session keeps the version its agent last got (`SessionInfo.Rules`, saved with the session, so it outlasts the daemon). The daemon looks at the rules every 30 seconds and at each save, and with each hook below. The session's header says what it knows: arrived and how, not confirmed, or changed and waiting.

| Agents | As it starts | When the rules change | After compaction or resuming | Known to have arrived |
|---|---|---|---|---|
| Claude Code, Claude Agent Teams | In its system prompt, a file of the session | With its next prompt, through `UserPromptSubmit` | Again, once: `SessionStart` (source `compact`, `resume` or `clear`), or else the next prompt after `PreCompact` | Yes, handed to its hook |
| Codex | With its session's start or first prompt, through its hooks; its inject also names `ENOWX.md` | With its next prompt, through `UserPromptSubmit` | Again, once: the same hooks | Yes, handed to its hook |
| OpenClaude, Droid, Autohand Code, Continue | In its system prompt, a file of the session | No hook: its guide says `"$ENOWX_BIN" guide` prints the current ones, and it may never run it. The header says they changed | At resuming, as it starts. Compaction isn't seen | At the start; later only if it runs the guide |
| Aider, Auggie, OpenCode, OpenCode 2, Kilo Code | The session's own file, named on its command line or in its config's variable | The same | The same | The same |
| Qoder CLI, Qoder CLI China, OMP, Prime Agent, Qwen Code, CodeBuddy, Grok | Its inject tells it to read the session's `ENOWX.md` | That file is rewritten, with the new version line; it reads it if it looks again | At resuming it is told to read it again. Compaction isn't seen | **Not confirmed**: in the file, not known read, unless it runs the guide |
| Kiro CLI, GitHub Copilot, Antigravity, Cline, Devin | A file of its home holds the guide, shared by all its sessions, so no project's rules: it says `"$ENOWX_BIN" guide` prints them. Copilot's MCP server gives them too | By running the guide again | By running the guide again | Only when it runs the guide |
| Gemini CLI, Ante, MiMo Code, ZCode, Pi, Goose, Amp, Crush, Command Code, Mistral Vibe, Rovo Dev, Jcode | A block of a file of its home says to run `"$ENOWX_BIN" guide`, which prints them | The same | The same | Only when it runs the guide |
| Cursor, Kimi, Trae, Muse, DeepSeek Harness, Codebuff, Freebuff, Hermes, OpenClaw | A line after the first prompt, and a block in the project, say to run the guide. Kimi's MCP server gives them too | The same | The same | Only when it runs the guide |

On Windows, whose agents have no hooks, Claude Code and Codex go as the rows without one. A tab's agent gets what its session's would. The Notes and Canvas pages' agents get no project's rules. Rules with nothing new are never sent again by a hook: only a change, a compaction or a resume sends them, once. What goes through a hook or the guide has any secret masked, as the guide's file does. Rules over 8,000 characters don't go in a hook's answer, as an agent cuts what is long: it says to read `RULES.md` in the session's folder.

These rows come from the code and tests with stand-in agents, not from real sessions of each agent, as the table above says.

Nothing here writes an agent's settings file. A block in an instructions file is the one lasting change enowX makes to a file of the user's, kept between markers with the rest untouched; OpenCode and Kilo Code no longer need one, and a block an older enowX left in OpenCode's file stays, harmless, until the user removes it.

Where an agent's docs offer no way for one session, and the closest there is:

- **Gemini CLI, Hermes, Cursor, Muse, OpenClaw:** MCP servers only in a settings file of the user's (`~/.gemini/settings.json`, `~/.hermes/config.yaml`, `mcp.json`, `~/.config/muse/settings.json`), which enowX doesn't change. Closest: the `"$ENOWX_BIN"` commands, or the user adding `"$ENOWX_BIN" mcp` there themselves.
- **Cursor, Hermes, OpenClaw, Kimi, OpenCode 2:** instructions only from the project's folder or a file of the user's identity (`SOUL.md`). Writing into either would change what isn't enowX's. Closest: the line after the first prompt.
- **Trae, DeepSeek Harness, Codebuff, Freebuff:** their documentation couldn't be read here. They keep the line after the first prompt until it can.

## Helpers end by themselves

Sessions enowX starts for one piece of work end when it does, without the user asking, and leave the session lists; what they made stays where it belongs. Each host's daemon sees to its own (`daemon/helpers.go`).

| Session | Ends | What stays |
|---|---|---|
| A design reviewer | once its verdict is in and its turn is over | the verdict, on the run |
| A region's unslop agent | once it says its part is done and its turn is over | the design, on the canvas |
| A build's direction | after 15 minutes unused, as it may be sent back after the comparison | its design, on the canvas |
| A Canvas page's agent, a Notes page's agent | after 15 minutes unused: the next message goes on with its conversation, by its ID where the agent can resume one; else a new one, with the page's chat log | the canvas, the note, the chat log |

One at work or waiting on the user is never ended, nor one whose hooks have gone quiet, which may be either. Two exceptions, for an unslop's regions' agents, which are sessions of their own and which nothing else would end: one that reported done goes after 2 minutes whatever its state reads, and one that ran for 2 hours without reporting ends with its run's others.

An unslop's regions' agents end with the run: when the Canvas page's Stop is pressed (its redesign's agent is only interrupted), when the redesign's agent is closed or hibernated by hand, when it reports the run done, and when a new run starts. The redesign's agent itself doesn't end by being left idle while they work. The panel's bar over a region's agent has **End**, for one at a time. Not helpers, and never ended by this: sessions the user starts, GOD of Agents and its workers, anything on its board, a teammate's session, a fan-out (its results wait to be compared), pipelines and automations (the session is their result), and tabs, which end with their session. Those left from before this are cleaned up the same way, at the first look after the daemon starts.

## Agents in a session's background

A session's row and `enowx-app ls` say which agents its agent has running besides itself (`SessionInfo.Background`): `background(N)` as its state in `ls` and `status`, **background** in the app's state chip in place of done or idle, and each one's name, how long it has run, its live CPU and its memory in the row, with its command on hover. `wait` goes on while they run, and a helper isn't ended under them. The session's menu has **Stop background agent**, one at a time, asking first with what is lost, and `enowx-app background stop`. GOD of Agents' board shows a worker's, and GOA is told when one ends or has used next to no CPU for 15 minutes, which may mean it's stuck; a worker on another host's are shown on that host's board. Hibernating names them as what stops and may lose work, and doesn't happen on its own while they run; an update's dialog names them too, as its restart stops them and only their sessions come back.

**How they're found: the session's process tree, for any agent.** Every 15 seconds the daemon follows what each session started (`daemon/procs.go`); a process of it counts when its arguments name a preset's program (`agent.ProgramOf`), through an interpreter too (`node`, `bun`, `python`, `sh`), and it runs under another agent or left the tree. Its own agent doesn't count, nor a wrapper starting a binary of the same name, a server or a command, nor an agent's own MCP server. One that runs for under a look doesn't count, so a one-shot call isn't an alarm.

**What it can't see, said plainly:**

| What | Why |
|---|---|
| A subagent inside the agent's own process (Claude Code's Task tool, Codex's, OpenCode's, ...) | No process of its own. Only the agent's hook or transcript can say, and only Claude Code's is read (below). |
| An agent that only runs at an API, on someone's cloud | Nothing runs here. |
| An agent started by a CLI's own background service, as OpenCode's server or OpenCode 2's without `--standalone` | The service isn't a child of the session and bears no session's tag, so nothing ties it to one: it isn't counted, whatever it runs, and stopping the session doesn't reach it. |
| An agent named nothing a preset names, or run by an interpreter not listed | Its arguments name no agent. |
| One that starts and leaves its tree between two looks, on macOS and Windows | The tree never showed it, and only Linux shows a process's session tag, by which it is found there. |

**Per agent, on top of that:** Claude Code's Stop hook gives `background_tasks` (its background shells and subagents, checked on 2.1.318): a turn that ends with any running leaves the session working, and the turn it takes when they end makes it done. No other agent's hook is read for this.

## The model a session runs on

The session list, `sessions` and `ls` show each agent session's project, path and model. The model is what the agent writes in its transcript with each answer, looked at every few seconds, so `/model`, a switch, a fallback and a config all show, Claude Code's `/model` at once and others with their next answer. An agent enowX reads no transcript of, or one that hasn't answered yet, reads "unknown model"; a stopped session shows the last it said, marked "(last)". A program started by hand in a shell tab is listed as the shell, with no model.

## Skipping permission prompts

New session's **Skip permission prompts**, offered for agents other than Claude Code, starts an agent in its mode that asks nothing. Sessions the user doesn't start by hand follow it only where nothing would hear a prompt: an agent with no hooks (OpenCode and most others; every agent on Windows), started as GOD of Agents, its worker, or the chat of a note or a canvas (`SkipUnheard`). Codex tells enowX it is asking, so GOA's workers of it stay as `goa delegate` said (`--yolo`), and GOA answers. `goa start --yolo` is the same default on the command line.

The mode is the one a session shows ("skips permission prompts"), and stays through resuming and waking. An agent with no keys for its mode (OpenCode) takes it only as it starts: `switch --mode` says so.

### Claude Code always runs in bypass

Every Claude Code session enowX starts or brings back runs in `bypassPermissions`, for every user on every host, Claude Agent Teams and GOD of Agents included: New session, `spawn`, `goa delegate`, resume, `continue`, `wake`, a message to one asleep, fork, a tab, a session copied from another host, a switch of account, and a daemon's restart or an app's update, whatever an earlier daemon saved of it. `Preset.Argv` holds it for every command line, so a mode named anywhere goes unsaid; `--dangerously-skip-permissions` also beats a `defaultMode` in Claude Code's settings and what a conversation last ran in (run on 2.1.318).

- **No setting turns it off.** New session's switch and **Settings › New sessions › Permission mode** aren't offered for it; a `SkipPermissions` or `Mode` an app, the command line or an automation still sends is ignored. The rule is Claude Code's alone: other agents do as they are asked, and one taking over from Claude Code doesn't inherit it.
- **The warning.** On a home that never accepted it, Claude Code stops at a warning whose first answer exits. enowX passes `skipDangerousModePermissionPrompt` in the `--settings` it starts Claude Code with, so nobody sees that warning, and the user's own files aren't touched. The session's banner says it skips permission prompts.
- **Asking mid-session.** `switch --mode` or the chat may leave bypass, for as long as that process runs: any restart, wake or update brings it back. See [chat-view.md](chat-view.md) for how it gets there without plan mode.
- **What still stops it.** A hook's refusal (a deny, which bypass doesn't pass), the judge's sure no, and a hook's ask, which Claude Code still prompts for ([decide.md](decide.md)). A user's agent in bypass doesn't get the app's card for what can't be undone ([goa-tools.md](goa-tools.md#when-the-user-is-asked)).
- **Where it can't.** As root Claude Code refuses bypass, so the session starts asking, in the mode it was given, and its note says why; `switch --mode bypassPermissions` says the same. A `permissions.disableBypassPermissionsMode` Claude Code is given (run on 2.1.318 through `--settings`, not as a managed policy) starts it in `default` without a word: enowX goes by the mode its hook reports, and the session asks.
- **Started by hand in a shell tab.** enowX doesn't start that one, so it gets what its command line and Claude Code's settings say, and its prompts stay the user's ([decide.md](decide.md)). A custom command is the same. If a daemon's restart brings it back, it comes back in bypass, on its conversation.

OpenCode's mode is `--auto`: it approves what asks, in the session and its subagents, and paths outside the project, `.env` reads and repeated calls included; a `deny` rule of its config still stops the tool, which reports it in the session. enowX doesn't touch that config.

## When a session dies

A session that ended badly says why beside its name, in `session.exitWhy` and in the daemon's log: the signal for a code above 128 (129 is a hang-up), and that it never got going when that was within five seconds. Three such endings in a row keep it from being continued for 30 seconds, so nothing restarts it in a tight loop.

## Limits

**Agents at once** (command palette) sets how many agents may work at the same time on a host, kept by its daemon in `~/.enowx-app/settings.json`. An agent session started past it is created at once, with its worktree, but shows as **queued** and runs nothing; the daemon starts it in that session when an agent finishes its turn (as its hooks or its screen tell), oldest first. An agent just started counts as working for 10 s, so the next one doesn't start before it begins. Shells and custom commands are never queued. A daemon restart forgets the queue: the sessions waiting are gone.

**Set time budget…** (a session's menu, or the palette) sets how long its agent may work, counting only while it works. Past it, the daemon interrupts the agent as its user would (Escape for Claude Code and Codex, Ctrl+C for the others), the session says so, and the app notifies. A new budget lifts it. The work counted starts again when the daemon restarts. Budgets are in time only: what an agent costs isn't known for every agent (see Credits).

## Credits

The daemon reads what each signed-in account has left (`core/credits`, `credits.get`) and the app shows it on the Credits page. Tokens are read when needed and sent only to the vendor that issued them; enowX never stores, logs or refreshes them, since refreshing would sign the agent out. A reading is reused for 5 minutes. All three sources are undocumented and may change; a changed answer shows as an error on the card.

| Agent | Login read | Asked |
|---|---|---|
| Claude Code | macOS keychain items `Claude Code-credentials*`, else `~/.claude/.credentials.json` | `GET api.anthropic.com/api/oauth/usage` (`anthropic-beta: oauth-2025-04-20`) |
| Codex | `$CODEX_HOME/auth.json` (`~/.codex`) | `GET chatgpt.com/backend-api/wham/usage` with `ChatGPT-Account-Id` |
| Kiro CLI | its own | `kiro-cli chat --no-interactive /usage`, parsed; account from `kiro-cli whoami --format json` |

## Caveats

- **Claude Code refuses `--dangerously-skip-permissions` as root.** An address without a user signs in as root, and enowX then runs its daemon, and so every session, as a regular user of its own, `enowx`, made the first time (or the one it ran as before), through `su`. Sign in to each agent once from a Shell session as that user. The choice is saved per host. To run as root, or as another user, pick it in **Hosts › ⋯ › Edit… › Run sessions as** (*A regular user* with its name, or *root*) and save: enowX connects again as it says, and keeps it on every connection after. **Hosts › ⋯ › Ask which user to run as** makes the next connection ask instead, listing the server's users. Sessions started as one user stay with that user's daemon. If you keep root, Claude Code's sessions start asking, and each one's note says why.
- **Gemini CLI enables its sandbox with `--yolo`** (a Docker image by default). Without Docker, set `GEMINI_SANDBOX=false` in the server's environment, or turn sandboxing off in `~/.gemini/settings.json`, if that is what you want.
- Skipping permission prompts lets an agent run any command and edit any file the user can. Worktrees isolate files between sessions, not from the rest of the machine.

## Forking a session

A session's menu (**Fork…**, which warns first), `enowx-app fork SESSION [PROMPT]` and MCP `session_fork` start a new session, "NAME (fork)", from a compacted copy of its conversation, so a long session's context goes on fresh and the original stays. The agent copies the conversation (`claude --resume ID --fork-session`, `codex fork ID`) and `/compact` runs in the copy, so the old session's history, state, branch and folder aren't touched. It takes the same folder and branch, as the owner decided: two agents there can overwrite each other's changes, which the dialog and the command say. A session whose agent stopped is brought back first; one working is refused until its turn ends. `/compact` is typed only once the copy shows its prompt, and success is read from its conversation. If it can't start or compact, the new session goes and the error says why. Other agents answer that it isn't supported yet.

## Agents don't move the user

What shows depends only on what the user did: a click, a key, or a click on a notification. Agents, tools, daemon events, syncs and timers change content, not where the user is, and never take the keyboard from a field. What an agent wants to show comes as a card at the window's top right, and a click goes there: `canvas open`, `goto` and `compare`, `creds login`. `browser open` loads the session's Browser and marks it. A connection dropping leaves the page and open forms in place. What needs the user's answer and isn't their doing (an agent's or a daemon's question, a prompt on a connection the app started by itself: host control, Takeover, the vault, features, hibernating, the judge, an account at its limit, a RAG offer, trusting a server, whom to run as, a passphrase, signing in) comes as a card at the window's top centre, not a dialog. It takes no focus and blocks nothing: what the user is typing keeps every character and the keyboard, and Enter or Space approves nothing. They answer with a click, or with Tab, then Enter: the safe answer comes first, a password field waits for a click, and for its first half second a card ignores its buttons, in case a click was already on its way. One shows at a time, the oldest first, the next with controls of its own, so a second Enter answers nothing; behind a dialog the user opened it waits. In a terminal, which takes Tab for itself, the way there is a click. Past the daemon's wait (host control, hibernating and features 5 minutes, the vault 2) the card goes and counts as refused, so a late yes runs nothing. A dialog the user opened themselves, a connection they started included, is modal and focused as ever.
