# GOD of Agents

GOD of Agents (GOA) is one session the user drives that controls every enowX session on every host: this device, servers, teammates' shared hosts. It doesn't do the coding itself: it hands each task to a session, follows it, checks what came back and reports. It may read files for a project's context, and write what is small itself (a plan, a note, a doc), an edit going by its session's permission mode. Starting, stopping and switching sessions, opening projects, reviewing and merging, changing settings is control, and GOA's to do.

## In the app

**GOD of Agents** in the title bar's pages (or ⌘K, "GOD of Agents") enters the mode. With no GOA on the host shown, it asks which agent to run GOA on, each with how far it holds GOA to delegating (below). In the mode:

- GOA's session is the main view, as a session's: the same header, tab strip, tools dock and panel, with the session list folded away.
- The title bar reaches the pages as anywhere. GOA's header has **Rules** and **Leave**, which goes back (GOA keeps running), and its More menu **Stall limits**.
- Notes and Canvas show their own lists beside the page, with the choice of the user's own or a team's space, as the session list that holds them elsewhere is folded away.
- GOA's session opens on its **chat** when its agent has one (Claude Code, Codex…), once; Chat and Terminal stay in its header, the tools open in center tabs as in a session, and the view picked is kept.
- The side panel stays, its **GOA** group first:
  - **Sessions**: every session on every host the app reaches, its state and the task GOA gave it; a session that waits on the user says so. Clicking one opens it in a tab of the strip: its terminal, live, on any host the app reaches, GOA and its conversation staying as they are; a stopped one is brought back first. Closing the tab closes the view alone, and the session runs on. Its details button shows its task.
    Dragging one into a chat or terminal puts a pointer at it where the prompt is typed, `[session ID: name, agent on host]`, to edit and send; the guide tells agents what it means.
  - **Tasks**: the board, newest first: each task's session and host, the model and effort its prompt runs on and why, its status and result, what it asks. **Change model** sets another model and effort: switched in its session at once (its `/model`, `/effort`) and kept on the board as chosen by the user; GOA is told. Below, what a watched GOA did itself, files to revert (the user confirms).

GOA's mode shows the host the app shows; the app tells that host of GOA's workers on other hosts as they change (`goa.worker`), as only it sees them. With the app closed, GOA keeps controlling its own host; other hosts wait until the app is back, whose first look at each host tells GOA's host what its workers there did meanwhile (and every look after, for a change it missed).

Leaving the mode asks first, whichever way: the title bar's **Sessions**, **Leave**, or the palette. GOD of Agents keeps running either way.

## Its session

`session.create` with `goa: true` (the app's start, or `enowx goa start --agent A`): one GOA per host at a time.

- **A folder of its own**, outside every project (`<enowX's folder>/goa`), with no branch.
- **Every feature on**, its guide `agent.GOAGuide` first: delegate, don't work; don't poll; who answers which permission; keep the board and its collection.
- **Its permissions asked**: enowX's own tools are allowed (Claude Code's `--allowedTools mcp__enowx`, Codex's MCP server approved) and reading is allowed by its hook, unless the path looks like a secret; an edit goes by the session's permission mode: Claude Code asks unless it is in auto mode or the like; Codex writes in GOA's folder and the system's temporary folders unasked, its sandbox stopping the rest.

### Held to delegating, as far as each agent allows

| Level | Agents | How |
|---|---|---|
| enforced | Claude Code (and its teams), Codex | Their own hooks hand every tool call to the daemon first (`tool.before`); every tool but these is refused before it runs: enowX's MCP tools (`mcp__enowx__*`), its tools for reading and editing files, or one `"$ENOWX_BIN" …` or reading command (`cat`, `head`, `tail`, `ls`, `wc`, `grep`, `rg`, `sed -n 'Np'`, no secret named) with nothing chained, piped, redirected or substituted (`core/goa.Allowed`). Claude Code also starts with only its tools for files (`--tools Read,Grep,Glob,Edit,Write,NotebookEdit`, enowX's MCP server only, `--strict-mcp-config`, `ENABLE_TOOL_SEARCH=false` so enowX's tools load); Codex runs its commands read-only (`--sandbox read-only`). |
| limited | agents whose tools can be restricted at start but have no hooks | Started with enowX's tools only. (No agent enowX knows is limited and not enforced today.) |
| watched | every other agent | Only told. The programs GOA's session runs itself (found by the session's tag in their environment) and the files that change in the host's git projects while no other session works there are flagged to the user, with a banner; a file unchanged, or changed only in tracked files, when GOA started can be put back as it was (`git stash create` keeps that state without touching the folder). Best effort: a change GOA makes in a folder a worker works in passes as the worker's. |

The app shows each agent's level when picking GOA's; `enowx goa agents` lists them.

No enowX command that runs a program of GOA's choosing works in its session, or in its MCP server: `creds use … -- COMMAND`, `decide trim -- COMMAND`, and `run` of a project command (listing them still works). Those would be a way around the tools it lacks.

Checked with Codex itself, started as GOA: enowX's tools reached it over MCP as `mcp__enowx__…`, as its hooks name them; it read a project's file and wrote one in its own folder, and handed over a write outside it and a `go version`. Checked with Claude Code itself, started as GOA: it read a project's file and wrote one (in auto mode, unasked), and handed `go version` to a session, having no Bash.

## Tasks

`enowx goa delegate` (MCP `goa_delegate`, `goa.delegate`) hands a task over:

1. **Its session**: one named (`--session`, `--host`); with `--project` and no `--branch`, the one the project already has (below); else a new one of `--agent` in `--project` on `--host`, the project added there first, `--branch` a worktree of its own. A new one is GOA's worker (`SessionInfo.GOAWorker`). A session whose agent stopped (hibernated or ended) is brought back on its conversation, and the prompt typed once it shows again and goes quiet. If it can't come back, delegate fails and nothing goes on the board. Everything goes through `host.call`, on any host the app reaches, so control's checks, the user's yes and the log apply ([cli.md](cli.md#control)).
2. **Its model and effort**: the judge's `decide route` ([decide.md](decide.md)) for the prompt: easy to a cheap model on low effort, hard to the strongest on high, and a step cheaper when a token budget is at 80% or more ([tokens.md](tokens.md)). A new session starts on them (`SessionCreateParams.Model`, `Effort`); a running one gets its `/model` and `/effort` typed first. `--model`, `--effort`, `--config` (another provider, [configs.md](configs.md)) choose by hand; `--no-route` keeps the agent's own. The choice and why are printed, kept on the board, and shown in Tasks, where the user can change them.
3. **The board** (`goa.board`, kept in `goa-board.json` beside the daemon's settings, the last 500): the task, its session and host, routing, status, result.

**GOA is told, never polls.** The board follows each worker: this host's through its session updates (its agent's `turn.done` hook, or its screen going quiet), other hosts' through the app. A turn's end sets a task `turn-ended`, with what its agent said in `said`: it may be mid-work, so it is no result, and only `goa task --status done|failed` settles it, or its agent ending. When a task's session finishes a turn, asks something, fails or ends, the board changes and a line starting `[enowX GOA]` is typed into GOA's conversation once it is idle: the result (the worker's last message, else its screen's end), what it asks and whose it is to answer. GOA then checks the work (`chat`, `changes`) and settles the task (`goa task ID --status done --result …`).

There is no limit on workers; rate limits fall to accounts' fallback ([accounts.md](accounts.md)) and budgets.

**A task's to-dos.** `goa delegate --note NOTE --items "one\nper line"` shows the worker the to-dos its task finishes, with the rule to tick each in the same turn as its commit. Settling the task done doesn't tick them: it says which aren't ticked (`Open` on the board), as a to-do done but not committed is the worker's to say. GOA never tells a worker to leave a to-do open.

**Which session a project's task goes to** (`goa.PickSession`): an agent's session of its own in the project's folder, not in a worktree, whose branch is another task's; the user's, not a teammate's; of `--agent` when given, with the same `--yolo`, and the same `--config` when given. Not one working, asking the user, or with an open task on the board, and not a running one of an agent without hooks, as it may be working unseen. Of those, a running one first, then a hibernated one, then one that ended; among equals, the newest. With none, a new session. A running session keeps its own config when the judge's route names another provider's. An agent that quit to the shell in its terminal counts as running, as the daemon can't tell it from one at rest.

## Every session reports to GOA

Every session GOA controls reaches it when a turn ends, when it asks something, and when it fails, with its reply in the line (up to 2,000 characters; `chat ID` has the rest), whatever came of it. A session counts as controlled when it has a task on the board, or when GOA sent it something (`session send`, with no task): the board's `Told`.

Routes, in order:

1. The agent's turn-end hook.
2. `goa report --outcome done|partly|nothing|blocked|refused|failed "what happened"`, which every task's prompt asks for at the end of each turn. The report stays on the session, on its own host, and reaches GOA as the session's changes do, through the app for another host's. It goes on the task as `Outcome`.
3. For an agent with no hooks, one started by hand in a shell tab, and one whose hooks fail: the screen. A session no hook speaks for, still for 20 seconds, using under 20% of a core (an agent at rest isn't at 0), showing a turn's end (the time it took, or a prompt mark alone on its line) raises a report, not "isn't moving". Only what follows that end can say otherwise: the turn's own words above it and a footer's under it don't. A screen only quiet, with no such sign, stays a stall line. A session GOA sent something to with no task is watched the same way, on any host: it reports its turn's end, says once if it goes quiet for the idle limit with no sign why, and says if it is gone.

What the session doesn't say, enowX says for it: its process exiting or crashing, with the exit code and its last words, marked `NoReport` on the board and never shown as at work; a turn interrupted; a limit its agent showed; a lost sign-in, as what it asks; going silent, as a stall line; a host that can't be reached, once, after the idle limit.

A line stays until it has gone in: kept in `goa-tell.json` while GOA is busy, hibernated or stopped and over a daemon's restart, put back if typing it fails, the newest 200 kept. Only a stall line of a task since settled is dropped, as moot; a report isn't. With no GOA on the host at all there is no one to tell.

Sent something again, a task whose turn had ended is at work again, so a screen with no hook can report the next turn too.


A report belongs to the task under way when it was sent. A task given to a session starts from that session's last report, so it never takes one sent before it; a task only queued takes none. A turn's end told after a task was typed in, with the session no longer done, is the turn before's (on another host, one within 3 seconds of the task); if it really was this task's, the screen says so about 20 seconds later.

Lines about a task under way (its report, its turn's end, what it asks) go with its ID, and are dropped if it is settled before GOA takes them. A settled task raises nothing asking to settle it. What its session reports after it was settled arrives once, marked a correction.

A session's tab in GOA's strip attaches again by itself when the session's program starts again, as a hibernate and wake does (`SessionInfo.Run`, a value for each run). While the session isn't running, the tab says that what it shows is its screen from when it stopped.

The Sessions list in GOA's side panel has what is at work on top (asking the user, working, queued), then what finished, the latest first (`SessionInfo.FinishedAt`: a turn's end, a program's end, or a screen stopping where no hook says).

### How lines reach GOA

Workers' reports never go through GOA's input box while anything of the user's could be in it: a line typed there took the user's unfinished sentence along with it. By what each agent documents, in this order (`daemon/goa_inject.go`):

| When | Claude Code | Codex | An agent with no hooks |
|---|---|---|---|
| In a turn | `additionalContext` at the next prompt or tool's end | The same | None |
| At a turn's end | Its `Stop` hook's `additionalContext` keeps the turn going | `decision: "block"` with the lines as the reason | None |
| Idle | A background hook (`asyncRewake`, 24 hours at most, armed at each start and turn's end) wakes it on exit code 2, the lines as a system reminder | No way in documented: below | Below |

Idle with no such route, or that hook gone: the line waits, and is typed only into a composer sure to be empty, with no key pressed for 5 seconds and nothing at work or asked. A prompt the screen can't be sure of counts as a draft, so the line goes on waiting, shown in GOA's Tasks panel under Waiting for GOA. It never sends with a draft in the prompt. A line waiting goes with the next turn, however that starts. Claude Code's channels also push into an idle session, but they are a research preview, a custom one needs a development flag at each start, and they need Anthropic sign-in, so they aren't used.
## Workers' permission prompts

On the worker's host, before each tool call of a GOA worker (its agent's hooks):

1. **Urgent** calls are the user's: what can't be undone (`rm -rf`, `git reset --hard`, `DROP TABLE`…), money (Stripe, payments, creating servers), publishing (`git push`, `npm publish`, `gh pr merge`, deploys), secrets (`.env`, keys, `printenv`, `creds env`) (`core/goa.Urgent`). Their prompt is left to the user, with a sound and a banner once the agent really shows it (its permission-request hook; an agent without that hook, at the call); `session.ask` marks it `urgent`, and GOA's `answer` is refused ("the user's to answer"), on that host and in the app's relay. The judge's sure refusal still stands for them.
2. Otherwise, with the judge's `approve` use on: its sure yes runs the call unasked, its sure no refuses it; unsure is urgent too. A worker in bypass (by the mode its hook reports, or started so when it reports none) never prompts, so neither urgent nor unsure raises a banner there: the call runs, as bypass promises, and is logged. The judge's sure refusal still stops it.
3. Edits inside the worker's own folder are its work: allowed.
4. What is left asks as usual, and GOA, told, answers it (`answer ID allow|deny --host H`).

What can't be undone, asked by GOA or a worker in bypass mode, runs without the app's asking on the user's own hosts, and is logged ([goa-tools.md](goa-tools.md#when-the-user-is-asked)); a worker's urgent prompt stays the user's whatever GOA's mode.

## Its RAG collection

GOA's collection is its folder's (`rag.ProjectID` of GOA's folder), apart from every project's. What GOA hands out goes in by itself: at delegation the title, the whole prompt, project, host, session, agent and date; when a task truly ends, its status, final report, and the commits and pull requests it names. Worker conversations don't. A prompt or report holding a secret, by the check project indexing uses, is kept as a line saying so, and what the vault lent is masked. Entries stay until deleted (`rag memories`, `rag forget ID` from GOA's session). Each host's GOA has its own collection. `goa remember --kind task|delegation|outcome|decision|preference TEXT` (and `rag remember` from GOA's session) keep there, never in a project. `goa recall QUERY` searches it by meaning once the user turns RAG on for it, else by words, nothing sent anywhere. `--project P [--host H]` searches a project's collection too, read-only: GOA is the one session that may read a project not its own (`rag.search` from it names any project of its host; another host's goes through the app, `rag.search` being a read of control). It can't write to one.

## Its rules

**Rules** in GOA's header opens the user's rules for it: Markdown on how it delegates, what it leaves to the user, what it tells its workers. They live in the synced settings (`settings/goa-rules.md`), so every host's GOA follows them. GOA reads them in its guide when it starts, and, on an agent with a start-of-prompt hook (Claude Code, Codex), again with the next prompt after they change; `enowx-app goa rules` prints them. Only the app changes them: an agent reads them, it doesn't rewrite them. Rules that look like they hold a secret are refused.

**Write with AI**, there and in a project's rules, takes what the user wants in their own words and has an AI write it as clear rules, added after those there, which it sees but never gets to change (`rules.draft`): the app does the adding, exactly as they are, and a new rule that goes against one is shown to the user, not settled. Changing or removing a rule is by hand. It's refused past 32 KB, cutting nothing. Rules that couldn't be read can't be edited or saved over; saving them empty asks first. **Earlier versions** brings back the last 20 before a save (`rules.history`), kept on that host. The first of the host's judges that writes is asked: agents' one-shot commands, then model APIs, then local models, never a decision model, which only picks; the policy *local only* keeps it on the host. The result lands in the editor, to read and undo; nothing is saved until the user saves. Text that looks like it holds a secret isn't sent.

## Commands and tools

`enowx goa board|delegate|task|remember|recall|rules|agents|flags|start` ([cli.md](cli.md#god-of-agents)); MCP `goa_board`, `goa_delegate`, `goa_task`, `goa_remember`, `goa_recall`, `goa_agents` (feature `goa`, always on in GOA's session). Everything else GOA reaches is the rest of enowX's tools, on any host: [goa-tools.md](goa-tools.md).

## Protocol

`goa.agents`, `goa.board`, `goa.delegate`, `goa.task`, `goa.override`, `goa.worker`, `goa.flags`, `goa.revert`, `goa.remember`, `goa.recall`, `goa.rules`, `goa.rules.save`, `rules.draft`; events `goa.changed` (the board) and `goa.flagged`; `SessionCreateParams.GOA` and `GOAWorker`, `SessionInfo.GOA` and `GOAWorker`, `SessionAsk.Urgent`. Older daemons ignore the fields and lack the methods: the app's chip then says the host's enowX is too old.

## What it can't do yet

- The app shows GOA of the host it shows; a GOA on a host the app only watches isn't shown in the mode.
- A watched GOA's changes are found by looking (every 5 s), not prevented.
- Workers' permission prompts of agents without hooks aren't judged: GOA reads their screen and answers.

A task shows its project's name, path and host, kept from when it was delegated so they outlive its session ("no project" for a session outside any, "unknown" for a task older than this), its session's name beside its ID, and the model it really ran on: what its agent says in its transcript, every change kept (`GOATask.Ran`). What was only asked for reads "(asked)"; nothing known reads "unknown". On another host a task's model is read from the live session list, in the app only.

A session is also read for what stops it, every few seconds, whatever started it, a shell tab with an agent run by hand included (`core/agent/prompts.go`): prompts in bypass mode, startup dialogs, usage limits, signing in, updates, listed by their words; and, for what nobody listed, a screen still for 20 seconds, not saying it works, that ends on a question or a menu. It goes to waiting, GOA's line quotes what's asked and any time it gives, and `session_ask` says the same; when a timed prompt goes, GOA is told to read what happened.

A task for a project goes to that project's own session, even busy, asking, hibernated or started with other permissions (`goa delegate --project`, `core/goa.PickSession`). A busy one has it queued, shown so on the board, delivered at its turn's end, oldest first; if the session ends first the task fails with that said. A new session starts only when the project has none, or on `--branch` or a named `--config`, what the user asked for, and delegate says so. Worktrees and shell tabs don't take a project's tasks. A queued task goes in when its session's turn ends, or, its turn's end unheard as after an interrupt, once its prompt is really free: hooks gone stale, its screen still for 20 seconds, nothing running, and it neither works nor asks by what it shows.

A task that isn't moving goes to `stalled` on the board until its screen moves, and GOA gets one line naming how, for how long and what the screen shows; a further one comes only after three times the limit, and again each time that passes while it stays stalled. The hook event `session.stalled` fires once per stall, with no hook needed for GOA to be told.

| Case | What it is | Limit, by default |
|---|---|---|
| Never started | its screen is still blank after the task | `blank`, 1 minute |
| Idle | its session is hibernated or gone, or its agent isn't at work (no state, or done with a turn before this task) with its screen still | `idle`, 2 minutes |
| Stuck | its agent last said it works, but its screen has stood still | `stuck`, 10 minutes |

A session's processes using the CPU (over 1% of a core, its whole process tree) keep a task from counting as never started, idle or stuck, so a long build or test run isn't taken for one; a slow model's reply shows the agent's own timer, which moves the screen. That timer also hides an agent hung with its spinner still turning: nothing here reads its transcript. Where a host can't measure processes (Windows, or an older app or daemon on the way), the line says so, as it may be a long build. A session waiting for its turn to start (at the host's limit of agents) doesn't count.

The limits are each host's, for its own GOA, from a second to a day: **Stall limits** in GOA's More menu, Settings → New sessions, or `enowx-app goa stall --blank 90s --idle 5m --stuck 30m` (`goa_stall`), `0` putting one back to its default. They are kept in `goa-stall.json` in the host's data folder, and apply at once.

Tasks on other hosts are watched the same, through the app, which is what reaches them: every few seconds GOA's host reads their sessions, screens and CPU. With no app connected, or the host out of reach, their tasks stay as they are until it can look. Those reads aren't in `control log`, as no session made them. The hook runs on GOA's host, not the worker's.

Across a daemon restart, a stalled task stays stalled and GOA, whose waiting lines went with the daemon, hears again; the hook doesn't fire again for the same stall. A session that ended while the daemon was down settles its task as failed or stopped.

GOA's prompt is 5.2 KB, and goes into every turn; `goa skill setup` (4 KB), `goa skill features` (4 KB) and the docs (`goa doc`: a 10 KB list, each doc when asked for) are read when needed. With the prompt and those, a small model named a real command for each of the 29 features, and said who it is from the prompt alone.

Asked one question per doc (39 on 2026-11-02, Haiku, the prompt and `goa doc` its only tools, no code), it got 36 right at first. The three misses taught the prompt that a question means enowX unless the user says otherwise, that `goa doc cli` has every command, and to search before saying enowX can't; after those, all 39 hold the fact and name commands that exist. The questions check one fact each, not a whole answer.

A session waiting for the user's yes in the app, on what can't be undone, shows as waiting too, its line saying it's the user's to answer.
