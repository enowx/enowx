# Decisions: enowX's judge

The judge is a small, cheap model that enowX asks instead of guessing. It is given one situation and answers with a verdict, how sure it is, and why, in one line. enowX's own code then acts on that answer (`core/decide`, through the host's daemon). A session on a server is judged on that server.

Every use is **off** until the user turns it on. When a use is off, no judge answers, or the judge isn't sure enough, enowX does what it does without the judge. A failure never blocks an agent.

## Uses

| Use | When | The judge says | What enowX does |
|---|---|---|---|
| A quiet session's state | an agent without hooks of its own (not Claude Code or Codex) goes quiet | done, waiting, stuck, failed | waiting: marks the session waiting and plays the input sound; stuck or failed: plays the error sound with why |
| Approving commands | a hook **Ask the judge** before a tool runs | allow, ask, refuse | allow: the agent runs it without asking the user (Claude Code); refuse: the agent is told why; ask: the agent's own permission prompt |
| Trimming long output | a hook **Trim its long output** before a tool runs | trimmed (with the lines kept), keep | the agent reads only the lines kept, with a note naming the file that holds the whole output; the exit code is unchanged |
| Stopping loops | three of an agent's last eight commands failed, two of them the same | looping, progress | looping: presses Escape to stop the agent's turn, and asks the user whether to let it go on |

**Approving commands and trimming** act through hooks, so the hook's conditions say which commands they cover ([hooks.md](hooks.md)). Settings → Decisions → Uses has **Add the hook…** for each:
- **Approve:** `tool.before`, action `judge`, Tool `^(Bash|shell|exec_command)$`.
- **Trim:** `tool.before`, action `trim`, Text matching test, build and install commands. The command then runs as `enowx decide trim -- <command>`. Output up to 150 lines and 12,000 characters passes through as it is; longer output is trimmed by the judge.

A refusal never overrides the user's own hooks: a hook that refuses still refuses, whatever the judge said.

## Judges

The daemon finds what can judge on its host:

- **Agents** installed, through their own sign-in, with no key needed:
  - Claude Code runs `claude -p --safe-mode --strict-mcp-config --tools ""`;
  - Codex runs `codex exec --ephemeral -s read-only`;
  - Gemini CLI runs `gemini -p`.

  Each runs in an empty folder of its own, so it reads no project. Claude Code and Codex are offered with their cheapest models first (Haiku 5.5, then Sonnet 5.5; GPT-6-Luna, then GPT-5.6-Luna).
- **The model API an agent is set to** (Settings → Model APIs, `core/agentapi`): the API judges directly, with the key kept in the agent's settings. While an agent is set to one, its own models aren't offered, and its one-shot command goes to that API too.
- **Model APIs** whose keys are in the vault: Anthropic, OpenAI, Gemini, OpenRouter. The key is read from the vault when the judge is asked, and never kept in settings.
- **Local servers:** Ollama (`127.0.0.1:11434`) and LM Studio (`127.0.0.1:1234`), each with its models.
- **The user's own:** another OpenAI-compatible address, with a model and a vault credential, or a model server elsewhere on the host.

**Test** asks a judge one fixed question it should allow (`git status`) and keeps how it did: right or not, how long it took, and its cost when the service reports one.

**The order** of the list is the order judges are tried. When a judge fails, takes too long, or answers outside what was asked, the next judge is tried, and the one that failed is skipped for five minutes. A judge can be turned off.

## Who may judge

The policy decides which judges may be asked, for every use:

- **The session's agent's provider** (the default): a Claude Code session is judged by Anthropic's models, a Codex session by OpenAI's, a Gemini CLI session by Google's. An agent set to another model API is judged by that API's provider, taken from its address (OpenRouter, say). An agent enowX doesn't know is judged by local models only.
- **One provider** the user picks.
- **Any provider.**
- **Local models only:** nothing leaves the host.

Local models are always allowed. A project can have its own policy (Settings → Decisions → Uses, for the project open), so a client's project can keep to local models.

## Per use

- **On or off.**
- **How sure** the judge must be for enowX to act: 75% by default, 90% for approving commands.
- **The judge:** the order, or one judge alone.
- **Calls a day:** 300 by default. Every judge asked counts, including those that failed; an answer from the cache doesn't.

The same question asked again within ten minutes gets the same answer without asking a judge.

## The log

Every decision is kept on the host (`decide-log.json`, the latest 1,000), with:
- what the judge was given, cut short, with the host's lent secrets and anything that looks like a key masked;
- its verdict, how sure it was and why;
- which judge answered, how long it took, its tokens and cost, and the judges that failed first;
- what enowX did, and how many characters a trim kept out of the agent's context.

Settings → Decisions → Log filters by use and marks a decision **wrong** (or right after all). The Uses tab totals the week: tokens kept out of agents' context and loops stopped.

When the judge stops a loop or refuses a command, the app asks the user. **Let it go on** (or **Let it run it**) marks the decision wrong and tells the agent to go on.

## For agents

The feature `decide` (Settings → Features) tells agents what a refusal by the judge, a stopped turn and a trimmed output mean:

| CLI | MCP | |
|---|---|---|
| `decide status` | `decide_status` | the policy, the uses on, the judges in order |
| `decide log [--use USE] [--last N]` | `decide_log` | the latest decisions and why |
| `decide test [JUDGE]` | | test one judge, or all |
| `decide mark ID [--right] [--note TEXT]` | | mark a decision wrong (or right) |
| `decide ask USE --command CMD` | | ask now (state and trim read stdin) |
| `decide trim -- COMMAND` | | run a command, its long output trimmed, its exit code kept |

## Measured

Measured on this Mac with `DECIDE_EVAL=1 go test ./core/decide -run TestEval`: 23 situations whose right verdicts are known (16 commands to approve, 5 screens, 2 runs of steps), and a 15,641-character test run with two failures to trim.

| Judge | Right | p50 | Cost | Trim |
|---|---|---|---|---|
| Claude Code · Haiku 5.5 | 23/23 | 3.2 s | $0.0033 for all 24 calls | 220 characters (1.4%), both failures kept |
| Codex · GPT-6-Luna | 23/23 | 4.0 s | its plan (about 17,000 tokens a call: Codex's own instructions) | 220 characters (1.4%), both failures kept |

An approval adds a few seconds before each command it covers, so the hook's conditions should cover the commands worth it. The cache answers a repeated command at once.
