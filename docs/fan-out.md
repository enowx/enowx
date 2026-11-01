# Fan-out

One prompt, given to several agents at once, each on a branch and in a worktree of its own, to compare what they make and keep the best.

- **New session**, on a new branch: turn on "Also give it to" for more agents. Each session's branch is the one named with the agent after it (`fix-login-claude`, `fix-login-codex`; an agent picked twice gets `-2`).
- **Command line**: `enowx-app spawn --agents claude,codex --branch fix-login "Fix the login redirect"` prints each session's ID.

The sessions are grouped (`SessionInfo.Group`, `session.fanOut` in the protocol). **Compare**, opened after starting them and from the command palette on any of them, lists each with its state, note and what it changed. **Keep** discards the others, with their worktrees and branches, and leaves the one kept to review, commit and merge as any session.
