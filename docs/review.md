# Review and merge

The **Git** tab of the side panel shows what the selected session changed and takes it on from there. The panel sits right of the workspace: the panel button in a session's header, or `⌘⇧D`, shows it, and the same button in its tab strip folds it. Its other tools, one button each in its top row, are described in [side-panel.md](side-panel.md).

The tab lists the changed files with git's status letters (`M`, `A`, `D`, `R`) and the lines each added and removed; clicking one opens its diff. The daemon runs `git` and `gh` on the session's host (`core/review`, `review.*`), so local and remote sessions work the same.

## What it compares

A session in its own worktree is compared with the branch the project folder has checked out, from where the two parted (`git merge-base`), so what that branch did since doesn't show as undone. A session in the project folder shows what isn't committed yet. The base menu picks another local branch, or the last commit. Committed, staged and unstaged changes all show, and untracked files as added; the view asks again every 4 seconds while the tab shows, since the agent keeps changing files. Very large diffs are cut (500 files, 4,000 lines a file, 30,000 in all) and say so.

## Actions

| Action | Shown when | What runs |
|---|---|---|
| Commit… | there are uncommitted changes | `git add --all`, `git commit -m` in the session's folder |
| Push | there is a remote, and the branch isn't pushed or is ahead of it | `git push --set-upstream <remote> <branch>` |
| Pull request… | `gh` is installed, there is a remote, and the branch has commits its base doesn't | a push, then `gh pr create --head <branch> --base <base>`, with `--fill` when the title is empty; the pull request opens in the browser |
| Merge into *base* | the session is in a worktree, its changes are committed, and the project folder has *base* checked out with nothing uncommitted | `git merge --no-ff` in the project folder; a merge that conflicts is undone (`git merge --abort`) |
| Discard the worktree… | the session is in a worktree enowX made | ends the sessions in it, `git worktree remove --force`, and `git branch -D` unless unticked |

Every action is also in the command palette (`⌘K`). Commands that talk to a remote never ask for credentials, which nobody would see: `GIT_TERMINAL_PROMPT=0`, `GH_PROMPT_DISABLED=1` and SSH in batch mode. Set up the remote's credentials (an SSH key, `gh auth login`) on the host once. They give up after 2 minutes, and run beside the daemon's other requests, so typing and the chat don't wait for them.

## Review comments

In a file's diff, clicking a line opens a comment on it; added and context lines are named by their line in the new file, removed ones by their line in the old one. Comments wait under their lines, and the file list counts them, until **Send to agent** types them into the session as one prompt (pasted, so its lines don't send it early), each with its file, line and the code it is on:

```
Review comments on your changes on feat/search (compared with main). Address each one:

1. core/search/rank.go:12 (`score := 0`)
   Start at 1: an empty query still matches.
```

**Discard** drops them. They are kept in the app, per session, until sent; the diff reading again doesn't move them.

## Conflict radar

While a project has more than one session, the app asks its daemon every 10 seconds which files two of its running sessions both change (`project.conflicts`): each session's changes against its own base, as the Git tab compares them, untracked files included. Sessions in one folder share their changes and are left out. A session that shares files with another is marked in the session list (amber when they change lines in common, which merging both would fight over), and its Git tab lists the files and the other session, which a click shows.

## From an issue

Under a git project's New session prompt, **From a GitHub issue…** lists the repository's open issues (`project.issues`: `gh issue list --state open`, on the project's host, newest 50, or those matching GitHub's search syntax typed in). Picking one fills the prompt with the issue's number, title, link and body, asking the agent to resolve it and run the tests, and a new branch named after it (`issue-12-login-form-forgets-the-email`). A pull request opened from that branch can then say `Fixes #12`.
