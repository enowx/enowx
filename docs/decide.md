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
| A model per prompt | asked: New session's **Suggest a model for this prompt**, `enowx decide route`, MCP `decide_route` | easy, medium, hard | suggests the agent's model and effort to match (below); nothing changes until the user or the agent asking takes it |
| RAG: context or not | a prompt starts in a project whose RAG adds context ([rag.md](rag.md)) | relevant (with the parts kept), none | the prompt gets only the parts that bear on it, or none |
| RAG: memories to keep | a session ends in a project whose RAG offers memories | decision (with the decision, distilled), nothing | decision: the offer's text is the decision; nothing: no offer |
| Team chat urgency | a team member's message comes in while the chat isn't open | urgent, normal, quiet | urgent: the sound, card and banner even when the chat is set to mentions or off; quiet: only counted unread. A message naming or answering the user is never kept quiet |
| Flaky or real test failures | a check fails (the side panel's Checks) | flaky, real | shown on the failed check with why; a flaky one offers **Run again** |

**Approving commands and trimming** act through hooks, so the hook's conditions say which commands they cover ([hooks.md](hooks.md)). Settings → Decisions → Uses has **Add the hook…** for each:
- **Approve:** `tool.before`, action `judge`, Tool `^(Bash|shell|exec_command)$`.
- **Trim:** `tool.before`, action `trim`, Text matching test, build and install commands. The command then runs as `enowx decide trim -- <command>`. Output up to 150 lines and 12,000 characters passes through as it is; longer output is trimmed by the judge.

A refusal never overrides the user's own hooks: a hook that refuses still refuses, whatever the judge said.

### A model per prompt

The judge says how hard a prompt is; enowX maps that to the agent's own models (`core/decide/route.go`, from the lists in `core/agent/settings.go`):

| Agent | Easy | Medium | Hard |
|---|---|---|---|
| Claude Code | Haiku 5.5, low effort | Sonnet 5.5, medium | Opus 5.5, high |
| Codex | GPT-6-Luna, low | its own model, medium | its own model, high |
| Another agent with efforts | low | medium | high |

An agent set to another model API (Settings → Model APIs) gets that API's small model for easy prompts and its main model otherwise. With no answer, or the judge not sure enough, the agent keeps its own model and effort.

The answer is `{agent, difficulty, model, effort, why, switch, config}`: `switch` is what to type in a running session of the agent for that model and effort (its `/model`, `/effort`), and a new session starts on them (`enowx spawn --model M --effort E`, New session's **Use it**). A running session's model never changes on its own. `config` is kept for a session config of the user's to suggest later; it is empty for now.

## Decision models first

A **decision model** (a "System One" model) is made for this. It takes a state and typed questions:
- a choice among options;
- a yes/no (`noul`);
- a score.

It answers each with probabilities and a confidence, never text. It is fast (tens to hundreds of milliseconds) and cheap (cents per million tokens). enowX asks decision models first, and falls back to a provider's small chat model when none is set up or none answers.

Each use is asked as typed questions:
- the choice uses (a session's state, approving, a loop, a prompt's difficulty, a chat message, a flaky check) are one choice, whose options are the verdicts and their meanings;
- RAG context asks one yes/no per search result;
- a **memory** asks whether there is a decision, and which of the agent's sentences states it;
- a **trim** asks, of groups of the output's lines, whether the agent needs them, then cuts the kept groups again, down to single lines.

A decision model never writes. The memory and the trim are lines and sentences it **picks**, copied as they are, so it can't invent an error. Its reason is what the picked verdict means, with its probability.

### Providers

| Provider | Models | Address | Key |
|---|---|---|---|
| TypeSafe (Jev) | `jev-latest` | `https://api.typesafe.ai/v1/systemone` | vault platform **TypeSafe** |
| Cloudflare Workers AI (Clef) | `clef-flash`, `clef` | `…/accounts/{account}/ai/run/@cf/cloudflare/{model}` | vault platform **Cloudflare** (token and account ID) |
| Opper (a gateway: Jev, Clef, Perplexity Decider, Nimble…) | e.g. `cloudflare:global/clef-flash`, `typesafe/jev-1.14.0` | `https://api.opper.ai/v3/compat/v1/systemone` | a vault credential |
| OpenRouter (System One) | e.g. `typesafe/jev-latest`, `inception/mercury-decide` | the address its docs give | a vault credential |
| Ollama Cloud | `clef`, `clef-flash`, `nimble` | `https://ollama.com/v1/systemone` | a vault credential |
| OpenAI Decisions API (its own shape) | `gpt-6-luna` | `https://api.openai.com/v1/decisions` | vault platform **OpenAI** |
| On the host: Ollama, Kev, SGLang (Clef) | `nimble`, `clef`, `clef-flash`, `tev1`; `kev-latest`; the server's | `http://127.0.0.1:11434`, `:8009`, `:30000` + `/v1/systemone` | none |
| Your own | any | any System One address (or OpenAI's Decisions shape) | optional |

Found on their own:
- a TypeSafe, Cloudflare or OpenAI key in the vault;
- Ollama's decision models (Nimble, Clef, Clef Flash, Tev1, Kev);
- a Kev or SGLang server on the host.

Settings → Decisions → **Add a judge** → **A decision model** offers each provider above, its address and model filled in, and any other System One address.

Models of other shapes aren't asked directly: Perplexity Decider, Liquid d1, Microsoft-Decision-1, Celeris-1 and Baseten's mammoth-1 and elephant-1. Reach the System One ones through a gateway (Opper, OpenRouter), or add their address once they publish one.

### Policy

A decision model set up for this always judges, whatever the session's agent, unless the policy is **local models only**. That means one you added, Jev, Clef, or one on the host. OpenAI's Decisions API is reached with an OpenAI key that is yours for everything OpenAI, so it stays under the policy like OpenAI's chat models.

## Judges

The daemon finds what can judge on its host:

- **Agents** installed, through their own sign-in, with no key needed:
  - Claude Code runs `claude -p --safe-mode --strict-mcp-config --tools ""`;
  - Codex runs `codex exec --ephemeral -s read-only`;
  - Gemini CLI runs `gemini -p`.

  Each runs in an empty folder of its own, so it reads no project. Claude Code and Codex are offered with their cheapest models first (Haiku 5.5, then Sonnet 5.5; GPT-6-Luna, then GPT-5.6-Luna).
- **The model API an agent is set to in its own settings** (Settings → Agents, `core/agentapi`): the API judges directly, with the key kept in the agent's settings. While an agent is set to one, its own models aren't offered, and its one-shot command goes to that API too.
- **Model APIs** whose keys are in the vault: Anthropic, OpenAI, Gemini, OpenRouter. The key is read from the vault when the judge is asked, and never kept in settings.
- **Local servers:** Ollama (`127.0.0.1:11434`) and LM Studio (`127.0.0.1:1234`), each with its models.
- **Decision models** (above), asked before the others.
- **The user's own:** another OpenAI-compatible address, with a model and a vault credential, or a model server elsewhere on the host.

**Test** asks a judge one fixed question it should allow (`git status`) and keeps how it did: right or not, how long it took, and its cost when the service reports one.

**The order** of the list is the order judges are tried. When a judge fails, takes too long, or answers outside what was asked, the next judge is tried, and the one that failed is skipped for five minutes. A judge can be turned off.

## Who may judge

The policy decides which judges may be asked, for every use:

- **The session's agent's provider** (the default): a Claude Code session is judged by Anthropic's models, a Codex session by OpenAI's, a Gemini CLI session by Google's. A session on a model API config ([configs.md](configs.md)) is judged by that API's provider, taken from its address (OpenRouter, say), whatever the agent's other sessions run on; an agent set to a model API in its own settings likewise. An agent enowX doesn't know is judged by local models only.
- **One provider** the user picks.
- **Any provider.**
- **Local models only:** nothing leaves the host.

A team chat message belongs to no session or agent: under *the session's agent's provider*, only local models judge it. Pick one provider, or any, for another model to weigh messages.

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
| `decide route [--agent A] [--json] PROMPT` | `decide_route` | a model and effort of the agent for a prompt (this session's agent without `--agent`) |

## Measured

Measured on this Mac with `DECIDE_EVAL=1 go test ./core/decide -run TestEval`: 23 situations whose right verdicts are known (16 commands to approve, 5 screens, 2 runs of steps), and a 15,641-character test run with two failures to trim.

| Judge | Right | p50 | Cost | Trim |
|---|---|---|---|---|
| Claude Code · Haiku 5.5 | 23/23 | 3.2 s | $0.0033 for all 24 calls | 220 characters (1.4%), both failures kept |
| Codex · GPT-6-Luna | 23/23 | 4.0 s | its plan (about 17,000 tokens a call: Codex's own instructions) | 220 characters (1.4%), both failures kept |

Decision models, measured the same way on Cloudflare Workers AI. The vault's Cloudflare credential had no account ID, so the account was found from its token:

| Judge | Right | p50 | Cost of all 24 calls | Trim |
|---|---|---|---|---|
| Clef (`clef`) | 23/23 | 0.45 s | $0.0015 | 228 characters (1.5%), both failures kept, in 2 rounds |
| Clef Flash (`clef-flash`) | 23/23 | 0.88 s | $0.0002 | the same |

A decision model is asked in under a second, where a chat model takes three to four. An approval then costs the command little; a chat model as the judge adds a few seconds.

An approval adds a few seconds before each command it covers, so the hook's conditions should cover the commands worth it. The cache answers a repeated command at once.
