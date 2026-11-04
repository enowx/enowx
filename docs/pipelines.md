# Pipelines

A pipeline runs coding agents one after another on one task: plan, then implement, then review, by default, each stage starting from what the one before ended with (`daemon/pipeline.go`, `proto/pipeline.go`). It is started from a project's menu or the command palette (**New pipeline…**, git projects only), or with `enowx-app pipeline` (see [cli.md](cli.md)).

## How it runs

- The pipeline gets a branch of its own, made from the base branch (the project's current one unless picked), in a worktree. Every stage runs there, so each sees what the ones before changed.
- Each stage is a session of the agent picked for it, named after the stage and the task, listed with the project's sessions like any other: the user can watch it, type into it, or answer what it asks.
- A stage's prompt is a template: `{task}` is the task, `{previous}` what the stage before ended with, `{base}` the base branch. The defaults (`proto.DefaultPipeline`) ask the first agent for a numbered plan and to change nothing, the second to carry the plan out and run the checks, the third to review the branch against the base and fix what it finds. Stages can be renamed, given other agents and prompts, added (up to 8) and removed in the dialog.
- A stage is done when its agent's hooks say so (Claude Code, Codex), or, for agents without hooks, once its screen has been quiet for 15 seconds after its first 15. An agent asking the user something (`waiting`) holds the pipeline until it is answered in its session. A session deleted, or a program ending with an error, fails the pipeline.
- What a stage ended with is its agent's last answer, from its conversation where enowX reads it (Claude Code, Codex), else the end of its screen; up to 8,000 bytes go to the next stage.
- Skip permission prompts follows the New session dialog's setting; Claude Code always skips them.

The daemon keeps its pipelines in `~/.enowx-app/pipelines.json`: one restarted goes on following the stage that was running, whose session it restores. Cancelling stops the pipeline and leaves its sessions as they are; a pipeline that ended can be removed from the list.

## In the app

A project's pipelines show over its sessions: the task, then a chip per stage (pending, running, waiting, done, failed), each opening its session. The ⋯ menu cancels a running pipeline or removes one that ended.

## Protocol

`pipeline.start` (`PipelineStartParams` → `Pipeline`), `pipeline.list`, `pipeline.cancel`, `pipeline.remove`, and the `pipeline.updated` event. Older daemons don't know them: the app shows no pipelines there.
