# GOD of Agents

GOD of Agents (GOA) is one session the user drives that controls every enowX session on every host: this device, servers, teammates' shared hosts. It doesn't do the coding itself: it hands each task to a session, follows it, checks what came back and reports. It may read files for a project's context, and write what is small itself (a plan, a note, a doc), an edit going by its session's permission mode. Starting, stopping and switching sessions, opening projects, reviewing and merging, changing settings is control, and GOA's to do.

## In the app

The title bar's **GOD of Agents** chip (or ⌘K, "Enter or leave GOD of Agents") enters the mode. With no GOA on the host shown, it asks which agent to run GOA on, each with how far it holds GOA to delegating (below). In the mode:

- GOA's session is the main view, alone; the session list is folded away.
- A strip on top reaches GOD of Agents, **Browser** (GOA session's browser in the main view), Notes, Canvas, Usage, Hosts and Settings, and **Leave** goes back (GOA keeps running; the chip, now "Leave GOD of Agents", does the same).
- Notes and Canvas show their own lists beside the page, with the choice of the user's own or a team's space, as the session list that holds them elsewhere is folded away.
- GOA's session opens on its **chat** when its agent has one (Claude Code, Codex…), once; Terminal, Chat and Traffic stay in its header (its browser is the strip's), and the view picked is kept.
- The side panel stays, its **GOA** group first:
  - **Sessions**: every session on every host the app reaches, its state and the task GOA gave it; a session that waits on the user says so. Clicking one shows its task, and opens it (leaving the mode) when it is on the host shown.
    Dragging one into a chat or terminal puts a pointer at it where the prompt is typed, `[session ID: name, agent on host]`, to edit and send; the guide tells agents what it means.
  - **Tasks**: the board, newest first: each task's session and host, the model and effort its prompt runs on and why, its status and result, what it asks. **Change model** sets another model and effort: switched in its session at once (its `/model`, `/effort`) and kept on the board as chosen by the user; GOA is told. Below, what a watched GOA did itself, files to revert (the user confirms).

GOA's mode shows the host the app shows; the app tells that host of GOA's workers on other hosts as they change (`goa.worker`), as only it sees them. With the app closed, GOA keeps controlling its own host; other hosts wait until the app is back, whose first look at each host tells GOA's host what its workers there did meanwhile (and every look after, for a change it missed).

Leaving the mode asks first, whichever way: the title bar's chip, **Leave**, the palette, or **Open the session** in Tasks. GOD of Agents keeps running either way.

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

1. **Its session**: one named (`--session`, `--host`), or a new one of `--agent` in `--project` on `--host`, the project added there first, `--branch` a worktree of its own. A new one is GOA's worker (`SessionInfo.GOAWorker`). Everything goes through `host.call`, so control's checks, the user's yes and the log apply ([cli.md](cli.md#control)).
2. **Its model and effort**: the judge's `decide route` ([decide.md](decide.md)) for the prompt: easy to a cheap model on low effort, hard to the strongest on high, and a step cheaper when a token budget is at 80% or more ([tokens.md](tokens.md)). A new session starts on them (`SessionCreateParams.Model`, `Effort`); a running one gets its `/model` and `/effort` typed first. `--model`, `--effort`, `--config` (another provider, [configs.md](configs.md)) choose by hand; `--no-route` keeps the agent's own. The choice and why are printed, kept on the board, and shown in Tasks, where the user can change them.
3. **The board** (`goa.board`, kept in `goa-board.json` beside the daemon's settings, the last 500): the task, its session and host, routing, status, result.

**GOA is told, never polls.** The board follows each worker: this host's through its session updates (its agent's `turn.done` hook, or its screen going quiet), other hosts' through the app. When a task's session finishes a turn, asks something, fails or ends, the board changes and a line starting `[enowX GOA]` is typed into GOA's conversation once it is idle: the result (the worker's last message, else its screen's end), what it asks and whose it is to answer. GOA then checks the work (`chat`, `changes`) and settles the task (`goa task ID --status done --result …`).

There is no limit on workers; rate limits fall to accounts' fallback ([accounts.md](accounts.md)) and budgets.

## Tasks from Project Docs

`goa delegate --doc NOTE[#SECTION]` (`doc` on `goa_delegate`) hands over a task of a [Project Docs](project-docs.md) project. It reads the project first and refuses while its gate is closed, naming each document in the way; the owner approves in the Notes page. Otherwise it prepends to the prompt what the task needs of the documents, up to 8000 bytes, saying what was cut and how to read it: the section, the PRD's and Technical Spec's sections it is based on, the acceptance criteria not met yet; without such links, the document's outline. A line of that context, `Project Docs: <project ID> <doc ID>#<section>`, ties the task to its project: the board and the protocol don't change, so a daemon of before serves all of it.

The gate holds only with `--doc`: `goa delegate` without it hands over whatever it is given, and the guide tells GOA to stop while the gate is closed.

- `docs plan PROJECT` (`docs_plan`): the Work Breakdown's tasks, written `- [ ] T-1: verb phrase (stage, depends on T-0)`, their dependencies, the board tasks whose title names them, what is ready and the exact command for each. It delegates nothing.
- `docs progress PROJECT` (`docs_progress`): documents by status, the gate, tasks planned and done, the project's tasks on the board, acceptance criteria done.
- `docs verify PROJECT --task ID` (`docs_verify`): the task's result beside the criteria that apply to it, the document's or section's own, else User Stories and Acceptance Criteria. It ticks nothing: the checker ticks with `notes check`, only with evidence, and names what remains.

## Workers' permission prompts

On the worker's host, before each tool call of a GOA worker (its agent's hooks):

1. **Urgent** calls are the user's: what can't be undone (`rm -rf`, `git reset --hard`, `DROP TABLE`…), money (Stripe, payments, creating servers), publishing (`git push`, `npm publish`, `gh pr merge`, deploys), secrets (`.env`, keys, `printenv`, `creds env`) (`core/goa.Urgent`). Their prompt is left to the user, with a sound and a banner; `session.ask` marks it `urgent`, and GOA's `answer` is refused ("the user's to answer"), on that host and in the app's relay. The judge's sure refusal still stands for them.
2. Otherwise, with the judge's `approve` use on: its sure yes runs the call unasked, its sure no refuses it; unsure is urgent too.
3. Edits inside the worker's own folder are its work: allowed.
4. What is left asks as usual, and GOA, told, answers it (`answer ID allow|deny --host H`).

## Its RAG collection

GOA's collection is its folder's (`rag.ProjectID` of GOA's folder), apart from every project's. `goa remember --kind task|delegation|outcome|decision|preference TEXT` (and `rag remember` from GOA's session) keep there, never in a project. `goa recall QUERY` searches it by meaning once the user turns RAG on for it, else by words, nothing sent anywhere. `--project P [--host H]` searches a project's collection too, read-only: GOA is the one session that may read a project not its own (`rag.search` from it names any project of its host; another host's goes through the app, `rag.search` being a read of control). It can't write to one.

## Its rules

**Rules** in GOA's strip opens the user's rules for it: Markdown on how it delegates, what it leaves to the user, what it tells its workers. They live in the synced settings (`settings/goa-rules.md`), so every host's GOA follows them. GOA reads them in its guide when it starts, and, on an agent with a start-of-prompt hook (Claude Code, Codex), again with the next prompt after they change; `enowx-app goa rules` prints them. Only the app changes them: an agent reads them, it doesn't rewrite them. Rules that look like they hold a secret are refused.

**Write with AI**, there and in a project's rules, takes what the user wants in their own words and has an AI write it as clear rules, keeping the current ones (`rules.draft`). The first of the host's judges that writes is asked: agents' one-shot commands, then model APIs, then local models, never a decision model, which only picks; the policy *local only* keeps it on the host. The result lands in the editor, to read and undo; nothing is saved until the user saves. Text that looks like it holds a secret isn't sent.

## Project Docs

A project documented as notes hands GOA its tasks, with the approval gate before them: [project-docs.md](project-docs.md#god-of-agents).

## Commands and tools

`enowx goa board|delegate|task|remember|recall|rules|agents|flags|start` ([cli.md](cli.md#god-of-agents)); MCP `goa_board`, `goa_delegate`, `goa_task`, `goa_remember`, `goa_recall`, `goa_agents`, `docs_plan`, `docs_progress`, `docs_verify` (feature `goa`, always on in GOA's session). Everything else GOA reaches is the rest of enowX's tools, on any host: [goa-tools.md](goa-tools.md).

## Protocol

`goa.agents`, `goa.board`, `goa.delegate`, `goa.task`, `goa.override`, `goa.worker`, `goa.flags`, `goa.revert`, `goa.remember`, `goa.recall`, `goa.rules`, `goa.rules.save`, `rules.draft`; events `goa.changed` (the board) and `goa.flagged`; `SessionCreateParams.GOA` and `GOAWorker`, `SessionInfo.GOA` and `GOAWorker`, `SessionAsk.Urgent`. Older daemons ignore the fields and lack the methods: the app's chip then says the host's enowX is too old.

## What it can't do yet

- The app shows GOA of the host it shows; a GOA on a host the app only watches isn't shown in the mode.
- A watched GOA's changes are found by looking (every 5 s), not prevented.
- Workers' permission prompts of agents without hooks aren't judged: GOA reads their screen and answers.
