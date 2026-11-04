---
name: plan-work
description: Use when turning an enowX Project Docs PRD and Technical Spec into milestones and tasks with dependencies, ready to hand to agents through GOD of Agents.
---

# Plan the work

## 1. Read what is to be built

    "$ENOWX_BIN" docs summary PROJECT
    "$ENOWX_BIN" notes outline "PRD"
    "$ENOWX_BIN" notes outline "Technical Spec"
    "$ENOWX_BIN" notes outline "User Stories and Acceptance Criteria"

Plan from the documents as they are. If PRD and Technical Spec are unfinished, say so: planning on sand wastes the tasks. Stale sections (docs summary) are re-read first.

## 2. Milestones

In Roadmap and Milestones, under "Milestones": stages, each ending in something that can be shown or tested. No invented dates: a date comes from the owner, or carries {{Assumption}}.

    "$ENOWX_BIN" notes write "Roadmap and Milestones" --section "Milestones" --body -

## 3. Tasks

In Work Breakdown and Backlog, under "Tasks". One line each, in exactly this form, since docs plan reads it:

    - [ ] T-1: Add the orders table and its migration (MVP)
    - [ ] T-2: Build the order form against the API spec (MVP, depends on T-1)
    - [ ] T-3: Send the confirmation email (MVP, depends on T-1, T-2)

That is `- [ ] T-<n>: verb phrase (stage, depends on T-<m>)`. Each task starts with a verb, is small enough for one session, and finishes something checkable. Only real dependencies count: what can go on at once should.

    "$ENOWX_BIN" notes write "Work Breakdown and Backlog" --section "Tasks" --body -

Every Must feature of the PRD is covered by some task. Name what isn't, rather than leaving it to be noticed.

## 4. See the plan as enowX reads it

    "$ENOWX_BIN" docs plan PROJECT

It lists the tasks, their dependencies, what is ready and the exact command for each. It delegates nothing. A line it can't read means the form above isn't kept: fix the line.

## 5. Hand tasks over

Only with the owner's say-so, and only once the gate is open:

    "$ENOWX_BIN" docs gate PROJECT

While it blocks, stop: tell the owner what to approve. Once open, delegate what is ready, one task at a time, its section carried in the worker's prompt:

    "$ENOWX_BIN" goa delegate --doc "PRD#Features and priorities" --title "T-1" "T-1: Add the orders table and its migration"

Follow the work with docs progress PROJECT. Keep the plan true: a task added, split or dropped is written here, with a Change history line.
