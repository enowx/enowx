# Automations

An automation is a prompt an agent gets on a schedule, each run in a session of its own, kept and run by the host's daemon whether or not an app is connected (`core/automation`, `daemon/automation.go`). Weekday triage of new issues, a nightly dependency update, a weekly summary of what changed.

They are added, changed, turned on and off, run now and removed in Settings → **Automations** (for the connected host), or with the command line ([cli.md](cli.md)):

```sh
enowx-app automation add --name "Weekday triage" --schedule "weekdays 09:00" --agent codex --branch "Triage the issues opened since yesterday"
enowx-app automations
enowx-app automation run "Weekday triage"
```

| Field | Meaning |
|---|---|
| Schedule | `hourly` (or `hourly :15`), `daily 09:00`, `weekdays 18:30`, `weekly mon 08:00`, in the host's time. |
| Project, agent, prompt | What runs, where. |
| Own branch | `automation/<name>-<date>-<time>`, in a worktree of its own, so runs don't touch each other or the user's work; otherwise the project folder. |
| Skip permission prompts | Nobody may be there to answer them. Claude Code always skips them. |

The daemon keeps them in `~/.enowx-app/automations.json` and looks for those due every 20 seconds; one it was down for runs once when it is back. Each records its last run, its session or why it failed, and its next run.
