# A project's settings

A project can keep settings for enowX in `.enowx.toml` at its root, committed with it, so everyone working on it gets them (`core/project/config.go`). Today they say what new worktrees get:

```toml
[worktree]
# Files or folders git ignores that a new worktree gets from the project
# folder: paths or globs, inside the project.
copy = [".env", ".env.local", "config/*.local.json"]
# A script that readies a new worktree, run in a tab of its session named
# Setup, which keeps a shell once it ends.
setup = "npm install"
```

- **copy** is done whenever a session's new branch or existing branch gets a worktree made for it, before its agent starts; what the worktree has already stays, links are left out, and paths can't leave the project.
- **setup** runs only when the user asks: the New session dialog shows the script with **Run the project's setup** (on by default when there is one), so a repository's script never runs unseen. It runs once, when the worktree is new, in its folder, while the agent starts beside it.

```toml
[guide]
# Keep enowX's block out of this project's AGENTS.md or CLAUDE.md. It goes there only
# for agents with no other way to enowX's guide (docs/agents.md). It is on by default,
# and turning it off takes it out, the rest of the file as it was.
block = false
```

The New session dialog says what a new worktree gets. A project without the file gets nothing extra.
