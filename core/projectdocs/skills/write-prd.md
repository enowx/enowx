---
name: write-prd
description: Use when writing or revising a PRD in enowX Project Docs from what brainstorming settled. Turns the Brief and the user's answers into features, priorities and testable acceptance criteria.
---

# Write the PRD

## 1. See what is there

    "$ENOWX_BIN" docs summary PROJECT
    "$ENOWX_BIN" docs gaps PROJECT
    "$ENOWX_BIN" notes outline "PRD"
    "$ENOWX_BIN" notes outline "Brief"

Read the Brief's sections, and the PRD's sections you are about to fill. If the Brief's problem or goals are still empty, go back to project-brainstorm for those first: a PRD can't be derived from nothing.

## 2. Fill it a section at a time

    "$ENOWX_BIN" notes show "PRD" --section "Features and priorities"
    "$ENOWX_BIN" notes write "PRD" --section "Features and priorities" --body -

- **Features and priorities:** every feature names the user it serves and its release. Must, Should or Could; MVP or Later. A feature nobody asked for stays out, or is marked {{Assumption}}.
- **Roles, states, non-functional:** a concrete rule or number each. "Fast" is not a requirement; a guessed number with {{Assumption}} is better, since it can be argued with.
- **Out of scope:** what this version leaves out, and why.
- A section that doesn't apply gets a line starting "Skipped:" with the reason, in place of its "_To fill:" line.

Link what you build on, rather than repeating it: [[Brief#Problem and audience]].

## 3. Acceptance criteria

They go in User Stories and Acceptance Criteria, as to-dos, each one something a person or a test can check:

    - [ ] Paying with an expired card shows "Card expired" and charges nothing

Not "payment works well". Each Must feature has at least one. These to-dos are what review-against-prd checks later, so a criterion that can't be checked is a defect in the PRD.

## 4. Say what it's based on

    "$ENOWX_BIN" docs link "PRD" --to "Brief#Goals and success measures" --in "Problem and goals"

docs summary then reports the section as stale when its source changes.

## 5. Questions and review

Unanswered points go as to-dos under the PRD's "Open questions", not as guesses in the text. When it is complete, set it for review and tell the owner, naming the open questions and every {{Assumption}}:

    "$ENOWX_BIN" docs status "PRD" "In review"

The owner approves it in the Notes page. After approval, your further changes to it become proposals, each with a Change history line.
