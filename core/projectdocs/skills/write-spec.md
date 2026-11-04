---
name: write-spec
description: Use when writing or revising a Technical Spec in enowX Project Docs from the PRD and the project's code. Each section says what it is based on, so a change upstream shows it as stale.
---

# Write the Technical Spec

## 1. Read the sources

    "$ENOWX_BIN" docs summary PROJECT
    "$ENOWX_BIN" notes outline "PRD"
    "$ENOWX_BIN" notes show "PRD" --section "Features and priorities"
    "$ENOWX_BIN" notes outline "Technical Spec"

On an existing project, read the code too, before you write anything about it:

    "$ENOWX_BIN" rag search "how are orders stored"
    "$ENOWX_BIN" project context NAME

A rag result is a lead: open the file before relying on it, and cite file:line. What the code does today is a fact; what it should do is a decision.

## 2. Write a section at a time

    "$ENOWX_BIN" notes write "Technical Spec" --section "Data model and migrations" --body -

- **Stack and reasons:** each choice with why, and what it was chosen over. On an existing project, the stack in use is a fact from its files.
- **Architecture, data, auth, integrations, security, observability:** what is built, not what could be. Unknown versions, limits and prices are looked up with a source, or marked {{Unverified}}.
- A section that doesn't apply gets a line starting "Skipped:" with the reason.
- Diagrams: follow draw-diagram. Don't describe from memory what a diagram should show from the schema.

Each real choice between options also goes in the Architecture Decision Records document: what, why, what was passed over.

## 3. Link every section to what it comes from

One Based on line per section, to the source's section where there is one:

    "$ENOWX_BIN" docs link "Technical Spec" --to "PRD#Features and priorities" --in "Architecture"
    "$ENOWX_BIN" docs link "Technical Spec" --to "PRD#Non-functional requirements" --in "Security"

The line records the source's content as of now. docs summary reports the section as stale once the source moves on.

## 4. Review, then confirm the links again

Set it for review, and tell the owner what is assumed or still open:

    "$ENOWX_BIN" docs status "Technical Spec" "In review"

After a review, re-read what changed in the source, change the section if it must change, and run the same docs link again for each section you checked. That is what says the section still agrees. Never run it to silence a stale mark without reading the source.

The owner approves in the Notes page. Log each later change in the document's Change history.
