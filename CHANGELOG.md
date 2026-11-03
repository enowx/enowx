# Changelog

Each version's section is also what the app's update dialog shows.

## 0.6.1

Fixes sessions that wouldn't start on Windows, and keeps long content fast in more of the app. **On Windows, read "When you update" below.**

- **Windows: sessions start again.** An agent's guide and MCP settings went on its command line, which could pass the 8,191 characters cmd.exe allows, and the session failed with "The command line is too long". They now go in files of the session's own, on the host where the agent runs, at every start, resume, wake and restart. Agents with no way to take a file keep the guide on the command line; on Windows, past the limit, the guide goes in a file and a line tells the agent to read it, and the session's note says so. **How far this is tested:** in CI, through real PowerShell and cmd.exe with a stand-in agent. It hasn't run with a real agent on Windows. If a session still fails, please open an issue.
- **Windows, with no shell picked:** PowerShell stays open after the agent exits, and the session no longer ends with exit code 1.
- **Every system:** Claude Code, OpenClaude, Droid, Autohand Code and Continue now read their guide from a file. A version of one too old to take that won't start: update the agent. A secret found in your rules is masked in the guide.
- **Long content stays fast in more places:** Notes, unslop reports, GOD of Agents' panel, Activity, session lists and the palette, team chat, Token tables and canvas HTML views. Each now has a check with ten megabytes and a line of one. To hold that:
  - A note of over 2,000 blocks draws 300 at a time, with Show more.
  - A note block over 64 KB shows as it reads and doesn't open to type in. A table or layout block that long shows as raw text.
  - A note over 256 KB can be read and copied as Markdown, not edited there.
  - A canvas element over 64 KB can't be edited in the inspector.
  - A chat message shows its first 8,000 bytes and says how many more there are. Copy still takes it all.
- **Less work in the background:** the daemon no longer runs `git` for every project every 5 seconds to look at hooks. What lists projects keeps its answer per folder, and asks again when a checkout changes it, or after 5 minutes.

### When you update

- **On Windows, enowX can update itself even though sessions fail.** Updating doesn't need a session. Open the app and wait for the card at the bottom right, or use **Check for updates** in the command palette (Ctrl+K), then **Restart now**. If you're still in the first-run setup, finish it or leave it first: the card shows after it.
- **If that doesn't work on Windows:** close enowX, download `enowX.Setup.0.6.1.amd64.exe` from this release and run it. Keep its folder as it offers (`%LOCALAPPDATA%\Programs\enowX`): your data stays in `~/.enowx-app`.
- **Neither way has been tried on real Windows,** and updating an installed 0.6.0 hasn't been tried on any system. If an update fails, install from this release and tell us.
- **The first start restarts the daemon,** as with any update: agents at work stop and continue their conversation when it's back, and commands running in terminal tabs end. So finish what's running first. Servers do the same the first time you connect to them.

## 0.6.0

A new look, a first run that sets everything up, files you can edit, and a lot of work on staying fast. **Please read "When you update" below: going back to 0.5.1 isn't supported.**

- **Four UI styles:** Liquid Glass, Ledger, Tonal and Bento, in Settings → Appearance, saved per device, with Liquid Glass as the default. The whole app has been redrawn, and a background image, panel opacity and blur come with Liquid Glass.
- **Onboarding:** the app assembles itself while you set up agents, a project, a host, a style, GOD of Agents and a first task. Everything optional has Later, and nothing in it needs an agent or the network. If you already use enowX, it shows your settings to confirm instead. Run it again from Settings → App.
- **Edit files:** a text or code file opened in a tab can now be edited, on any host: undo, find and replace, go to line, comment out, and ⌘S or Ctrl+S to save. It writes only when you save. If an agent changes the file while you have changes, it asks: compare, keep yours or take theirs. Files over 16 MB, with a line over 8 KB, read-only or not UTF-8 stay read-only, and say why.
- **Long content stays fast:** Traffic, files, diffs, Checks output, logcat and code blocks draw only what's in view, so an 11 MB response or a one-line file of a megabyte no longer freezes the window.
- **Sessions keep what you gave them:** a session's model, effort and permission mode now survive restarts and wakes, instead of taking whatever the agent's global settings say by then. Settings → New sessions can set what new ones start on. Claude Code's `/model` and `/effort` still rewrite its own defaults, which is out of our hands: with this set, they don't reach new sessions.
- **GOD of Agents:** told about everything its workers do, without typing into your input box. Reports for every outcome of a turn, and for sessions you sent something to. Permission prompts answered by the judge or by GOA. Its sessions are ordered by what's at work.
- **Canvas:** versions no longer copy the whole canvas each time and stay on their host, which took some histories from gigabytes to megabytes. Moving designs export to MP4.
- **Settings → Clean cache:** see what an enowX folder can do without, and remove it. Nothing goes without asking.
- **Also:** a sent chat message shows at once, with Retry; an outline of the conversation; token counts rebuilt, with cache reads and estimates shown apart; Resource usage shows what sessions left running, with Stop; a report is left when the app stops answering; many fixes on Windows.

### When you update

- **Update every device and host together.** A 0.5.1 app connecting to a server puts its own 0.5.1 daemon back on it, and canvas versions already converted can't be undone to from 0.5.1 (nothing is lost: updating again brings them back). A 0.5.1 app can't sync with a host on 0.6.0, on purpose: it would delete canvas versions on the other host.
- **The first start restarts the daemon.** Agents at work stop and continue their conversation when it's back, and commands running in terminal tabs end. So finish what's running first. Servers do the same the first time you connect to them.
- **Canvas versions are converted once, in the background,** while you keep working: usually seconds, about ten seconds a gigabyte on a recent Mac, and longer on a small server. Each canvas's versions are kept as they were in `~/.enowx-app/canvas-history-backup/` first, and a canvas whose backup fails isn't converted. That folder keeps its disk space until you delete it by hand, which you can do once you're happy. To restore a canvas, see [the canvas docs](https://github.com/enowdev/enowx/blob/main/docs/canvas.md#where-they-are-kept).
- **Token counts are recounted once** from the agents' records, with a backup, so past figures may change.
- **Going back to 0.5.1 isn't supported.** It can run, but it won't read converted canvas versions, or show anything of these styles.
- **The repository moved** to [enowdev/enowx](https://github.com/enowdev/enowx). Updates keep working from older versions.

## 0.5.1

- **GOD of Agents knows what it is and what's going on:**
  - **Who it is:** its prompt says it, and it reads enowX's docs, now inside the app, and a built-in guide for setting up every setting, stopping for your keys and your agreement.
  - **Told of everything that stops a worker:** prompts no hook reports, startup dialogs, limits, sign-ins and your confirmations in the app, also for an agent run by hand in a shell tab, quoting what's asked and the time a prompt gives. A worker that isn't moving goes to "stalled", with limits you can change and a `session.stalled` hook.
  - **Keeps to a project's session:** a task goes to the session that knows the project, queued when it's busy, and a new one starts only if there is none or you ask. A session of yours in bypass isn't asked again for what can't be undone.
  - **Opens any session** in a tab beside it, and says why when it can't. Tasks remember what they asked and how they ended, with their project, path and the model they really ran on.
- **The model each session really uses:** in the session list and `sessions`, from what the agent reports, with its project and full path.
- **Canvas:**
  - **Prototypes:** a canvas's designs linked into pages sharing state, with a Reset, in Interact mode and on the shared page. Designs' scripts are now kept apart from the app and the page around them.
  - **Motion:** anime.js and three.js built in, so they work offline, with play, pause, restart and a scrubber, and agents check motion from sampled frames.
  - **Stays where you are:** the zoom holds between designs, Interact mode fits a design in a centered box, the grid hides with a button, and the unslop card scrolls inside itself.
- **Nothing moves you any more:** agents asking to show a canvas, a browser page or a login raise a card you can click. A dropped connection leaves your page and forms in place.
- **Asks no longer take your keyboard:** what an agent or the app asks without your doing arrives as a card you click or Tab to, so Enter or Space while typing approves nothing. A late answer no longer runs.
- **Settings → Agents lists all 45 agents,** with what each takes or why it can't, and search. Kilo Code, Qwen Code, Goose, Kimi, Continue and Crush take a model API per session, and Kimi, Mistral Vibe and Pi another account.
- **Fork a session:** a compacted copy goes on in a new session, for Claude Code and Codex.
- **Notifications say what they're about,** in cards that open the session.
- **Keep typed keys in the vault,** from RAG and Decisions, so other hosts can use them.
- **Fixed:**
  - OpenCode kept asking for permission in GOD of Agents, its workers and the chats of notes and canvases, which never got its `--auto`.
  - Adding rules with AI could drop your existing rules: it now only adds, and earlier versions are kept.
  - A daemon dying could leave a terminal stuck that hung every new shell on the Mac until it was cleared.
  - A decision use has no daily limit by default: `approve` stopped answering at 300 calls.
  - A session that dies says why, and isn't restarted in a tight loop.
  - A session in a folder behind a symlink showed no model or chat for Claude Code and Codex.
  - The missing confirmations and other bugs an inventory of the app found.

### Not tested for real yet

Built and tested with stand-ins or on this project's Mac; these wait for real use.

- **Sessions dying with code 129 on an Intel Mac (#12) is not fixed:** the cause isn't found. This release only says why a session died, and stops restarting it in a loop.
- **Scrolling in OpenCode:** the report couldn't be reproduced. Wheel and trackpad work here in OpenCode, Claude Code, Codex, OMP, Antigravity, CodeBuddy and Freebuff; not checked with long output in Kiro CLI, Command Code, Droid, Cursor or Muse, nor on a remote host. If it still fails for you, please say where.
- **In the real app:** typing while an ask card arrives, motion's bar in Interact mode, and opening a real remote host's session from GOD of Agents' list were tested in the test harness only.
- **Accounts and limits:** an account's fallback was tested on Claude Code and Codex with a mock API's limit error, not a real limit. Kimi's, Mistral Vibe's and Pi's other accounts and the new per-session APIs are read from their docs, not run. Keeping typed keys in the vault wasn't run on a real VPS.
- **Interruptions:** usage limits, signing out and running out of credits are recognized by their words, not seen on a real agent. A stalled worker was tested with stand-ins.
- **Motion** ran in a real web view on macOS only, not Linux or Windows.
- `switch --model` on Claude Code also changes its default for new sessions, as `/model` does.

## 0.5.0

- **GOD of Agents:** one session you drive that controls every session on every host. It splits what you ask into tasks and hands each to an existing session or a new one, on any host, agent and config; a board shows each task, its session, status and result. On Claude Code and Codex it is held to this by their hooks, on other agents watched, with each agent's level shown when you pick one.
  - **Its mode:** the session list folds away, Sessions and Tasks stay in the side panel, a strip on top reaches Browser, Notes, Canvas, Usage, Hosts and Settings. Leaving asks first.
  - **Model per task:** the judge picks each prompt's model and effort, or a cheaper provider's config, with why, cheaper near a budget. Change model overrides it.
  - **Its workers' permission prompts:** the judge and GOA answer them; what can't be undone, spends money, publishes or touches secrets comes to you with a sound and a banner.
  - **Rules:** tell it how to work, in the app, written by you or by an AI from your own words. Projects' rules can be written with AI too.
  - **Its own memory:** a RAG collection of tasks, outcomes, decisions and your preferences, and read-only search of projects. It reads files and writes what is small itself.
  - **Point at a session:** drag one from Sessions into a chat or terminal, for the agent to read it.
- **RAG, built in:** agents find a project's code, docs, attached notes and memories by meaning and by words (`rag search`, `rag remember`).
  - **Stored** in SQLite, or PostgreSQL with pgvector; embedded by Voyage, an OpenAI-compatible service or TEI. Keys from the vault or typed in Settings.
  - **Consent first:** before a project's first index, a dialog says which service gets its text, roughly how many tokens and what stays out. Secrets and ignored files never go.
  - **Kept up to date** as sessions change files, in the background, unchanged parts skipped.
  - **In the side panel:** status, Reindex, a search box whose results open the file at their lines, memories to edit, and turning it off, which deletes the index after asking.
  - **For prompts:** the parts that bear on a prompt added as context, and an offer to keep a session's decisions as memories.
- **Decision models:** a small model answers instead of guessing. enowX finds what can judge on the host: agents you have signed in (`claude -p`, `codex exec`), model APIs, local servers (Ollama, LM Studio), and decision models (Jev, Clef, Neuralwatt, OpenAI's). A failure never blocks an agent.
  - **Its uses:** a quiet session's state; approving a command, with the reason shown; trimming long output to what matters, the whole kept in a file; stopping a loop before more tokens go; a prompt's model; RAG's context and memories; a team message's urgency; a flaky or real test failure.
  - **Settings → Decisions:** the judges with a Test, the provider policy (the session's own, one provider, any, or local only), the fallback order, each use's switch, confidence and daily limit, per-project overrides, and a log to mark what was wrong.
- **On by default:** RAG, Decisions and their uses start on, and approving commands and trimming run without a hook, which costs a little time per command and some of the judge's quota. Turn any of them off in Settings → Decisions and Features. RAG still indexes nothing until you agree per project.
- **Hooks for every agent:** one format, installed into agents' own hook systems without touching your config, and detected from outside for agents without one; each agent's support is shown.
  - **Events:** sessions, prompts, tools, permissions, files, Git, status, ports, sync, notifications and schedules.
  - **Conditions:** agent, project, host, branch, tool, file, text, combined with AND, OR and NOT, with limits, hours and order.
  - **Actions:** run a command, add context, refuse, change the command, ask the judge, trim output, notify, a webhook, an enowX command.
  - **Settings → Hooks:** a builder and direct editing, templates, Test, a log; for the host, a project or a session. A project's hooks don't run until you allow them, and agents can only propose them.
- **Token usage:** a Tokens tab in Resource usage: each agent, model, project, session, config and host, per day, with input, cached and output, costs, prices you can edit, and budgets. Tracked from agents' own records, their traffic or their screen, with each agent's coverage shown, and never prompts or answers. Also `usage` on the command line.
- **Agents' configs:** other accounts and model APIs (Claude Code on a third-party model, say) saved apart from an agent's official config and picked per session at New session, shown as a badge, switched mid-session, and never written to the agent's own files. Model APIs set before move over, their files restored.
- **Accounts:** several per provider, in the order you set; at a limit, out of credits or signed out, a session goes on with the next, another provider's too, automatically or after asking. Their state, limits and a log of who was used and why.
- **Control from any session:** agents interrupt, stop, continue, rename, answer, switch model and review sessions on any host the app reaches, and change settings and project settings. What can't be undone asks you in the app, and a credential's value is never read.
- **Canvas:** a redesign's steps are kept, measured and played back, before and after, and exported as a video. Designs stay where you scrolled them while an agent works. Unslop also reads a page in the Browser for what makes it look AI-made.
- **Team chat:** a new message notifies with a sound, a banner and a card to answer from; unread counts; per team, all, mentions or nothing; quiet hours kept.
- **Delete a team:** its owner alone, typing its name; members' copies go at their next sync, and they are told.
- **Keys typed in Settings:** a judge's or RAG's key can stay on the host alone, without the vault.
- Fixed: a team chat showed only new messages until later, and said it was empty while reading; an agent with no conversation to continue failed to start; waiting on an SSH agent that hangs; Gemini CLI's sign-in with a config's key; a team's pull losing its place.

## 0.4.2

- **Agent chat, redone:** a conversation reads as a timeline, each step with its own icon (a pencil for an edit, a terminal for a command), a turn's work folded under one line. A bar over it says what the agent is at and for how long, searches the chat (⌘F) and opens or folds every step; each step says how long it took, an edit opens in Git in one click, and a button brings you back to the latest.
- **Answer the agent from the chat:** allow, always allow or deny a tool it asks to run, and pick an answer to its questions or write your own. Esc stops it, ↑ and ↓ bring your earlier messages back, and files attach with the paperclip or a drop; pictures you sent show under your message.
- **One sidebar:** the hosts, your sessions and the Notes or Canvas list share one sidebar with tabs, instead of three columns. Folded, it is one column of icons, each project a group of its sessions.
- **Rename a session** from its menu, or with a double click on it.
- **Canvas:** build a UI from a brief, with fresh agents writing concepts, a review scoring each design, and Compare to put designs or a running page side by side. Unslop finds more of what makes a design look AI-made, from the design as rendered, and a slider shows its before and after. Pages of your project in the Browser unslop too, on a branch of their own.
- **Full screen on macOS:** the green button takes the window full screen (Alt keeps zooming it).
- **Team chat:** send images, and a canvas link opens at the design it names.
- Fixed: designs rendered in a window that flashed over the app, and not at all while the app was in the background; Compare crashed with a shot per size; a build or a review stopped when its daemon restarted.

## 0.4.1

- **Team chat:** reply to a message (quoted in your reply, a click goes back to it) and react with emojis; see who of the team is online (a green dot, a count in the header) and who is typing, never what. The chat opens at its latest message, and its header switches between your teams.
- **Roles:** admins change a member's role from the team's Members tab, and contributors of a default team's GitHub repository join it as members. The members list stays current without reconnecting.
- **Switch space** from the Notes and Canvas pages: the line under the title opens your spaces, personal and teams'.
- **Terminal text size:** ⌘+ / ⌘− / ⌘0 (Ctrl on Windows and Linux), or the wheel with it held, size every terminal at once and say so; the size is kept, and Settings → Terminal changes it.
- **Security:** a full review of enowX Cloud and team sharing. Sign-in shows where a device asks from before you approve it, invites join only the team they showed, a team's sync takes each kind of change only from who may make it, keys are checked before use, and members working on a shared host get an environment and agent home of their own, only when you allow it.
- Fixed: an agent's canvas in a team's project went to your personal space, so it couldn't find it again; Kiro's sign-in page opened by itself (its usage is now read only when you refresh); a chat message changed width as its actions showed on hover.

## 0.4.0

- **enowX Cloud, for teams (opt-in):** sign in with GitHub and make a team, or join one from an invite link, in the app or right in the browser before you have it. Nothing reaches the cloud until you sign in, and what a team shares is end-to-end encrypted: the cloud keeps ciphertext only, never credentials.
- **A team's space:** its notes and canvases beside your own (switch in the account menu, move or copy between them), its projects as Git links, cloned in one click or found when you have them, its kit of skills, commands, subagents, pipelines, automations and templates, and each project's rules for the agents.
- **Shared hosts:** share a host with your team over Tailscale; members see its sessions, watch one live, ask to take it over (you let them and take it back), or start their own there with their own agent account, never yours. Keep a session private from its menu.
- **Team chat** in the side panel's Team group: `@` mentions members, notes, canvases, projects and files, Enter sends, messages edit (Up edits your last) and delete. The group also lists the team's projects, where each is and the sessions in it, and its members, who is on your host.
- **Team admin:** roles, invites, an activity log of who entered which host and started or took over which session, and a recovery key for the team's keys should every device be lost.
- **InstaBox:** create a VM there from Create a server, or connect to a machine you have, reached through its SSH gateway with a certificate signed for your key each time; paused machines wake as you connect. Its API key lives in the vault.
- **Discord:** what you work on shows under your name, the project, branch and agent with a Get enowX button; Settings → Discord turns it off or hides the names.
- Fixed: an event raised right as a client connected could be missed, so an agent asking to turn a feature on could wait for an answer that never came.

## 0.3.2

- **Terminal font:** Settings → Terminal picks the font terminals draw with, among the monospaced fonts of this device. Pick a Nerd Font and the icons programs like OMP print show as icons, where the system's monospaced font drew them as boxes. Terminals already open change at once.

## 0.3.1

- **Light and System appearance:** choose Dark, Light or System in Settings → Appearance; every palette has a light variant, your palette and accent are kept, and terminals stay dark. Until you pick one, web pages follow your device as before. Text on a custom accent now takes black or white, whichever reads better.
- **Images in Notes:** add a picture from a file or paste a screenshot, write beside it, mention it elsewhere on the page with `@`, rename or remove it, and undo or redo any edit (Ctrl/⌘+Z). Pictures nothing names any more are cleaned up after a week.
- **New session goes on with one you have:** pick a session of the project, running, hibernated or a past conversation, to open, wake or resume it, with an optional message.
- **OMP:** continue and resume its sessions, auto-approve, and its conversations in the chat view; sessions in one folder keep their own.
- **Files:** copy an opened file's source text from its toolbar, Markdown and HTML previews included.
- **Canvas:** zoomed out below 50%, a mockup shows as a picture of itself, so it shrinks whole instead of reflowing; click it to zoom in.
- **Sync with another host** shows each step while connecting, the Tailscale check's link to open, and can be cancelled; a server that stops answering while signing in gives up instead of leaving "Connecting…" forever.
- Fixed: after a project sync, a running agent goes on with the conversation brought instead of its old one; one file too large to sync no longer stops your data syncing (it is skipped and counted); large syncs no longer drop the connection; on Windows the app no longer restarts its daemon, and ends its sessions, at every connection; a session continued, woken or restored after a restart goes on with its own conversation, not the folder's latest.

## 0.3.0

- **Canvas:** design mockups, wireframes, diagrams and boards with agents on an endless board (scroll to move, pinch to zoom), watch them built live, compare and star variations, click a component to ask about it, change or redesign it, lay designs on a grid, share a read-only link on your tailnet, and turn a design into code. Agents start diagrams on a sheet of their own and keep designs within their frame.
- **Notes:** structured pages (tabs, toggles, columns, callouts, cards, an outline), version history, SVG icons in any color and size, an agent beside each page (Claude Code, Codex…) whose changes you accept, notes attached to projects on any host, and import from Notion (also on a schedule), Obsidian, Evernote, Keep and HTML, export to Markdown, HTML, PDF or a backup.
- **Credential vault:** your logins kept encrypted on this device and lent to sessions on any host when you allow it. Fill it from what a host's tools already keep, a token copied on the platform's page, or signing in to GitHub in the browser; git, gh, docker and ssh on servers use it.
- **Your data on every host:** notes, canvases, settings and the vault sync between this device and your servers, merged when both changed, conflicts kept to pick.
- **Projects across hosts:** clone from Git (also onto another host), keep a project's copies in sync on their own with conflicts announced, and auto push to a branch of its own.
- **Sounds:** a sound when an agent finishes, needs you or fails, your own sounds too, quiet hours and a count on the app's icon.
- **Features:** turn what agents are told about on or off, everywhere or per host, project or session.
- **Traffic:** pause, stop and resume recording. **Hibernate** a session yourself from its menu.
- **Floating control:** an optional radial menu over other apps (Cmd/Ctrl+Shift+Space): this PC or a VPS, a project, its sessions.
- Menus say a few words, the rest in a tooltip; Settings fills the window, and its sections no longer flicker between tabs and a menu.
- Fixed: the app's tests no longer stop a running daemon, Claude Code keeps its transcript when enowX was opened from an agent's terminal, a second daemon no longer takes the server's systemd service, the first auto push of a clean project works, and Windows passes agents their arguments whole in PowerShell 5 and 7.

## 0.2.2

- **Notes:** a Notes page in the rail with pages of notes on each host, nested like a notebook's. Write a block at a time: Markdown's shortcuts (`# `, `- `, `[] `, `> `) and `/` make headings, lists, to-dos, quotes, code, tables and dividers; drag blocks and pages to move them. Pages save themselves, and agents read and write them too (`enowx-app notes`).
- **Groups and order of projects:** drag projects and groups in the session list, or put projects in a group from their menu. The host keeps them, so every device shows the same list.
- **Pick your shell:** Settings → Terminal picks the shell terminals start with (zsh, bash, fish, PowerShell, Command Prompt, Git Bash, WSL…), and a session's **+** opens a tab of any of them.
- **Running as root:** a server's form now says whom sessions run as when you sign in as root, a regular user or root, and the choice sticks: connecting again no longer goes back to the regular user.
- Session rows stay two lines high; an agent's note shows on hovering its name.
- **Windows:** sessions start again (0.2.1's background daemon missed a library).
- Android: a device on the server connects through the server's adb, and the SDK is found by its adb.

## 0.2.1

- **The tray:** closing the window keeps enowX in the menu bar (macOS) or the notification area (Windows, Linux). Its icon counts the sessions that need you; a click lists those needing input, working or done, each opening the window on it. Settings → App can make closing quit again.
- **Android TV:** TVs are recognized; their screen fills the panel with a remote under it (D-pad, OK, Menu), Enter is OK, and agents press the remote's keys. **+ Device** connects a TV over the network, or makes a virtual one. Mi TVs' screenshots work.
- **Windows:** installing a new version while enowX runs no longer fails with "Error opening file for writing": the background daemon runs from a copy of the app. Updating from 0.2.0 may still need enowX ended once in Task Manager.

## 0.2.0

- **Traffic:** see the HTTP what a session runs sends and receives, HTTPS, WebSocket, SSE and gRPC included; mock, forward, change, fail or pause requests with rules; catch a terminal, a browser, an Electron app, Docker containers, the Browser tool or an Android device; export HAR. Its forms open as dialogs.
- **Android:** run the session's app on an emulator or a phone, of this machine or the server, read its log, and use its screen right in the side panel. Catching a device's traffic trusts the CA by itself on emulators and rooted phones.
- **Model APIs** (Settings): point Claude Code, Codex, OpenCode, Gemini CLI and 12 more agents at an API of yours (a gateway, OpenRouter, a local server), with several models and, where the agent takes them, vision, context window, output limit and reasoning effort. Each agent's settings are backed up first and restored in a click.
- **Every agent knows enowX's tools:** not only Claude Code and Codex. And agents can now do everything the side panel does: look at and tap an Android device, change traffic, run the project's commands, list its ports, set model APIs, from the command line or over MCP.
- The port cards can be turned off, in the Ports tool.
- Windows: no console window flashes for the programs enowX runs in the background.

## 0.1.1

- Windows: setting up a server no longer flashes a burst of console windows while this device's tools are scanned.
- The New session form fits small windows: its body scrolls, and Cancel and Start session stay in view.

Thanks to @Tiyouw for both fixes.

## 0.1.0

The first release.

- Run Claude Code, Codex and 43 other coding agents in parallel, each on its own branch in its own git worktree; fan out one prompt to several and keep the best; pipelines and automations.
- Watch them as terminals, or Claude Code and Codex as a chat; review each session's changes, comment lines back, commit, push, open a pull request and merge.
- Your own server over SSH or Tailscale: the daemon runs there as a service, sessions keep running with the laptop closed, one-click setup brings the server your tools, agents and skills. Desktop apps built there open on your screen.
- Copy a project and its sessions to another host, and sync them back both ways.
- Built-in browser for dev servers, ports forwarded, a files panel, memory and CPU of every session, and the `enowx-app` command line.
