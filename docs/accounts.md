# Agent accounts

An agent's other accounts are configs of it ([configs.md](configs.md)), as its model APIs are: a work and a personal Claude, say. Each is signed in once on a host, then picked per session, with no new sign-in to switch. The order, limits and fallback below are across all of an agent's configs, a model API's included.

Claude Code and Codex can have them; they keep their sign-in in a folder a variable names (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`). Claude Agent Teams uses Claude Code's.

- **Settings → Agents**: New config → Another account names one and opens a session signing it in. Remove forgets it, with its sign-in on that host. The configs sync to every host; a host where an account wasn't signed in yet signs it in from its menu.
- **New session** picks the config: Official is the agent's own.
- **Command line**: `enowx-app spawn --config Work …` (or `--account`).

Each account's home is `~/.enowx-app/accounts/<id>` on the host (`core/accounts`). It links to what is in the agent's own home (`~/.claude`, `~/.codex`): settings, skills, subagents, commands, plugins and conversations are the same whichever account a session uses, so the chat view, history and Settings see them all. Only the sign-in is the account's own (for Claude Code also `.claude.json`, which holds who is signed in, so MCP servers added with `claude mcp add --scope user` are per account). A session keeps its account when continued, after hibernation or a restart.

## API keys as accounts

A config of the kind **API key** is a credential of the vault (an Anthropic or OpenAI key, say): it has no home of its own. Sessions on it get the key as the vault lends credentials, its value never shown or logged.

## Order, limits and falling back

An agent's configs are used in an order (the official one is "" in it): the one first in it is the default for new sessions unless another is marked **Use for new sessions**, or a project has its own (`accounts default claude Work --project acme`). An account can be kept to some projects (**Only for some projects…**): it isn't offered, used or fallen back to elsewhere.

Each account shows its state and what it has left:
- **What it has left:** the plan and its limits, read from the provider with the account's own sign-in (`core/credits`). A Claude account keeps its sign-in in the keychain item `Claude Code-credentials-` followed by the start of its home's SHA-256, else in its `.credentials.json`; a Codex account in its `auth.json`.
- **Its state:** at its limit until a time, out of credits, or signed out, as a session's agent last said it.

The daemon reads the end of each session's screen while its agent isn't working, for what an agent says when it can't go on (`accounts.Classify`):
- **A usage limit:** "usage limit reached", "You've hit your usage limit", 429. When the agent says when the limit resets ("resets at 3pm", "try again in 2 hours 15 minutes"), that is the time kept.
- **Credits run out:** "credit balance is too low", 402.
- **A sign-in or key refused:** "Invalid API key", "Please run /login", 401.

What reads as a key or a token in that line is masked before it is kept, shown or logged.

**At a limit** sets what happens next:
- **Stop and tell me:** a sound and a banner.
- **Ask me first:** a dialog offers to go on as the next account.
- **Go on as the next:** the session switches by itself.

The next account is the one after it in the order that isn't spent (still at its limit, or one that doesn't say when it resets for an hour, credits and sign-ins for a day) and is allowed for the project. **Then** names another agent to go on with once every account is spent: it starts on a summary of the task (the user's prompts, what the other agent said last), since it can't take the other's conversation.

Switching (`account.switch`) stops the session's program and starts it again as the other account, on its conversation: the accounts share the agent's conversations. A fallback then types "Go on with what you were doing…" once the agent shows its input.

**Back to the first once its limit resets** has the session go back, between turns, once the first account isn't spent any more (looked at every minute).

A session's menu switches its account by hand. Picking an account takes it as working again, its limit forgotten.

Every start, switch, fallback, return and limit is logged (Settings → Agents → **History**, `account.log`, `~/.enowx-app/account-log.jsonl`): when, which session, which account, why.

The preferences (order, fallback, defaults, projects) are in the synced `settings/account-prefs.json`, by config name, so every host applies them to its configs of those names; renaming a config renames it there. Sign-ins stay on each host, credentials in the vault. A fallback is also a hook event (`account.fallback`, [hooks.md](hooks.md)).

The providers' terms are for the user's own accounts: no sharing one, and no several to get around limits a plan doesn't allow. The section says so, and enowX only switches between the accounts added here.

## Command line and MCP

- `enowx accounts` lists the accounts with their order, state and what they have left, and each agent's fallback.
- `accounts use NAME [--session S] [--agent A]` switches a session.
- `accounts priority AGENT NAME…` sets the order ("default" for the agent's own).
- `accounts fallback AGENT --on|--ask|--off [--to AGENT|none] [--return|--no-return]` sets what a limit does.
- `accounts default AGENT NAME [--project P]` sets the account new sessions use.
- `accounts log` shows the history.
- `spawn` without `--account` uses the preferences' default.

The MCP tools are `accounts_list`, `accounts_use`, `accounts_priority`, `accounts_fallback` and `accounts_log` (feature **Agent accounts**).
