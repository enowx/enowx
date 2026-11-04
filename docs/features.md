# Features

**Settings → Features** decides which of enowX's features agents are told of and get MCP tools for. A feature turned off costs agents no tokens; the app itself keeps it. The page shows what each feature's part of the guide costs (about a token per four characters), and what the guide costs in all.

- **Scopes.** A switch is set everywhere (kept in the synced `settings/features.json`, the same on every host), on one host (`features-host.json`, that host's alone), for a project (by its Git origin, else its folder: the same project cloned on two hosts shares its switches), or for one session (kept with it). The narrower decides. A host with no display has Display off unless turned on there.
- **Core** features (notes, sync) are always on.
- **Presets**: Minimal, Web dev, Android, All, applied to the scope shown.
- **Lazy guides** (on by default): long features (Traffic, Android, the vault, project copies) are a line in the guide pointing at `enowx-app help NAME`, which prints the whole of it when the agent needs it.
- A feature that needs another (Canvas needs the Browser) says so when the other is off.
- Changes apply to sessions started after them; running sessions keep what they had until restarted. Agents reading their guide from a file in their home (not their command line) share the host's.

Agents: `enowx-app features` lists the switches for their session; `features on NAME --reason "…"` from a session asks the user in the app (Keep it off / For this session / Turn on everywhere) and waits for the answer: an agent never turns a feature on by itself. MCP: `features_list`, `features_request`, `feature_help`. The MCP server serves only the tools of the features on, and its instructions are the session's guide.

**Project Docs** (`projectdocs`, on by default, needs Notes) serves the `docs_*` tools; the Notes page's agent gets only the read-only ones: [project-docs.md](project-docs.md).

New features register in `core/agent.Features` (ID, group, default, guide part, MCP tools).
