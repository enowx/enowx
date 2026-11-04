---
name: build-from-spec
description: Use when building a feature or task of a project documented in enowX Project Docs. Checks the approval gate first, builds to the acceptance criteria and keeps the documents true in the same turn as each commit.
---

# Build from the spec

## 1. The gate comes first

    "$ENOWX_BIN" docs list
    "$ENOWX_BIN" notes --project .
    "$ENOWX_BIN" docs gate PROJECT

If it blocks, stop before writing any code. Tell the owner exactly what it names and that they approve in the Notes page. Don't approve anything yourself, don't work around it, and don't start "just the easy part".

## 2. Read what applies, and whether it is current

    "$ENOWX_BIN" docs summary PROJECT
    "$ENOWX_BIN" notes outline "Technical Spec"
    "$ENOWX_BIN" notes show "Technical Spec" --section "Data model and migrations"

Read only the sections your task touches. A prompt from GOA already carries them: read further where it points. Sections docs summary reports as stale: read their source again before relying on them, and if the two disagree, ask the owner which holds.

## 3. Build to the criteria

The acceptance criteria in User Stories and Acceptance Criteria (or the document your task names) are what done means. Build what they say, and no more. Where the spec is silent or wrong:

- A small choice inside the task: make it, and record it in the Architecture Decision Records document.
- Anything changing what was approved: stop and ask. Don't quietly build something else.

## 4. In the same turn as each commit

Commit, then in that same turn:

- Tick what the commit finished, only when it is done and you have checked it:

      "$ENOWX_BIN" notes check "Work Breakdown and Backlog" --item "T-1: Add the orders table and its migration"
      "$ENOWX_BIN" notes check "User Stories and Acceptance Criteria" --item "Paying with an expired card shows"

- Update the sections the code changed, and add a Change history line. On an approved document this becomes a proposal: mention it in your report, and go on.
- If the schema changed, bring its diagram into step (draw-diagram).

What only the owner can test goes in a separate to-do of its own, and doesn't keep the build's to-do open.

## 5. Report

Say what was built, what was ticked and on what evidence, what is proposed and waiting for the owner, and what you could not check. Then review-against-prd can check the rest.
