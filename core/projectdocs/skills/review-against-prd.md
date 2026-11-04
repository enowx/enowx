---
name: review-against-prd
description: Use when checking finished work against the acceptance criteria of a project in enowX Project Docs. Ticks a criterion only on evidence, and names what stays unverified.
---

# Review against the PRD

## 1. Collect the criteria and the claim

    "$ENOWX_BIN" docs verify PROJECT --task ID
    "$ENOWX_BIN" notes show "User Stories and Acceptance Criteria" --section "Acceptance criteria"

docs verify sets a task's reported result beside its criteria. It ticks nothing: that is your job. No task on the board? Read the criteria of the feature you were asked to review. A worker's report says what to look at; it isn't evidence.

## 2. Check each criterion by itself

For each one, find evidence you saw yourself:

- a test you ran, with its name and result, or
- a file:line that does what it says, and for behavior, the behavior seen running.

A test existing proves nothing until it has run. Code that looks right is not evidence that behavior is right: when you can only read, say that you only read.

## 3. Tick only on evidence

    "$ENOWX_BIN" notes check "User Stories and Acceptance Criteria" --item "Paying with an expired card shows"

Each criterion comes out as one of:

- **Met:** ticked, with its evidence in your report.
- **Not met:** left open, with what happens instead.
- **Unverified:** left open and named, with what would verify it and who can do it. Never tick these to finish the list.

## 4. Look for what the criteria miss

Compare the result with the PRD's sections for the feature (empty, error and limit states; roles; non-functional). A gap the criteria didn't foresee goes to the owner as a new open question, not as a silently added criterion. A criterion you think is wrong is proposed for change, and stays open until then.

## 5. Report

A table of every criterion: met, not met or unverified, with evidence. Name what remains. Whether to accept the work is the owner's call: say what you found, and let them decide.
