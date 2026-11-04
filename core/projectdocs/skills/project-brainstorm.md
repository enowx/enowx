---
name: project-brainstorm
description: Use when the user wants to brainstorm, shape or document a project idea in enowX Project Docs, or to document an existing project from its code. Asks one topic at a time and writes each answer into the project's documents.
---

# Brainstorm a project into its documents

The result lives in the documents, not in the chat. A topic settled in chat and not written down is not settled.

## 1. Find or make the project

    "$ENOWX_BIN" docs list
    "$ENOWX_BIN" docs summary PROJECT

No project yet: ask only what you need to start (its name, and whether it is small or a serious product), then make it. The core pack suits most; the full pack suits a product with several people, money or regulation.

    "$ENOWX_BIN" docs new "NAME" --size core --project .

## 2. On an existing project, read before you ask

Code answers many questions. Read it first, and fill documents from it rather than asking the user what you can see:

    "$ENOWX_BIN" project context NAME
    "$ENOWX_BIN" project read NAME FILE
    "$ENOWX_BIN" rag search "how do people sign in"

In the Notes page your tools are project_context and project_read. Say which file:line each fact came from. Ask the user only what code can't say: the why, the who, the plans.

## 3. Pick the documents

    "$ENOWX_BIN" docs catalog

Each entry says what kind of project needs it. Choose from the project's type, not from a habit. Tell the user in a line or two which you add, and why:

    "$ENOWX_BIN" docs add PROJECT KEY

For one that doesn't apply, record why, so it isn't asked for again:

    "$ENOWX_BIN" docs skip PROJECT KEY --why "One role only: no permissions to map"

## 4. Ask one topic at a time

    "$ENOWX_BIN" docs gaps PROJECT

It lists what is unfilled, in the order best asked. For the first gap:

1. Ask its one question, in the user's language, shaped by what you already know. One topic per message; never a list of questions.
2. Skip a topic that doesn't fit this project, and write why in its section as a line starting "Skipped:".
3. Write the answer into that section at once, in the user's own terms, replacing its "_To fill:" line:

       "$ENOWX_BIN" notes write DOC --section "Problem and audience" --body -

4. Run docs gaps again, and go on from what it says. Stop when the user says so, or when what remains can wait; put what stays unanswered as a to-do under the project's "Open questions".

## 5. "Up to you"

When the user says "terserah kamu", "up to you" or the like, don't just pick. Give your suggestion with its reason in a sentence, write it into the section with {{Assumption}} beside it, and go on. The user can overturn it later; the mark shows them where to look.

## 6. Research with sources

For competitors, prices and technical options, search rather than remember. Each claim in a document carries its source's link and the date you read it. If you can't search, write what you believe with {{Unverified}} and say that you could not check it. Results go in the section they answer: competitors in the Brief or Market and competitor research, options in the Technical Spec with the choice recorded as a decision.

## 7. In the Notes page

You have no shell there: use notes_outline, notes_read, notes_write with section, docs_gaps, docs_topics, docs_catalog, docs_summary and docs_skill. Everything you write is a proposal the user accepts beside the note, so write the section whole and finished, and say in a line what you changed. Adding documents needs a session of the project, or the user: name what you would add and why. Don't delete notes.

## 8. Hand over

When the user has enough, go on with write-prd and write-spec. The owner reviews and approves those two in the Notes page before anything is built, unless the project has approval off.
