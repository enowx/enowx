# Project Docs

Project Docs documents a whole project (PRD, Technical Spec, ERD and the rest) as ordinary notes under one project note, for agents to fill in with the user, build from, and keep up to date. There's no new kind of note: sync, teams, history, attach, RAG and a note's agent work on them as on any other. Notes are the source; the repository gets a copy only when asked (`docs export`).

The flow: an idea, a brainstorm with an agent, the PRD, the Technical Spec and diagrams, a work plan, tasks through GOD of Agents, results checked against the acceptance criteria, the documents updated.

## What makes a note a document

Its body's first line, in `core/projectdocs/meta.go`:

```
Project Docs: Project · {{Core pack}} · Approval: required
Project Docs: PRD · {{Approved}} · v2 · @9f8e7d6c
```

A document's line has its kind, status, version and, once approved, the hash of its content as approved. A section says what it's built on in a line under its heading (`links.go`):

```
Based on: [[PRD#Features and priorities]] @1a2b3c4d, [[Brief]] @5e6f7a8b
```

**Why in the body, not the note's header.** A daemon of an earlier version rewrites a header without the keys it doesn't know, whereas a body survives it, sync, teams, exports and proposals. In an app that doesn't know Project Docs, these lines read as plain text, with `{{Approved}}` a badge in versions that have badges, and the notes are ordinary pages under one parent.

Hashes leave out these record lines and count every to-do as unticked: ticking an acceptance criterion doesn't change a document, and neither does updating its status.

## Sizes and the catalog

`core/projectdocs/catalog.go` lists every document with a template, by area, with **When**, the kind of project that needs it. `docs catalog` prints it.

- **Core pack:** the entries marked Core, as documents directly under the project note.
- **Full pack:** every entry, under a page per area.

`docs new --with KEY,KEY` adds entries, `docs add` adds one later, and `docs skip PROJECT KEY --why TEXT` records under **Skipped documents** why one isn't needed, so it isn't asked for again. A user's own template is a note kept under a top-level note called "Project Docs templates" (`docs template save`).

A template's sections start with a line `_To fill: …`. `docs gaps` lists the sections that still have one, in the order of `topics.go`, skipping absent documents and sections with a `Skipped:` line. That's the order an agent asks in.

## Statuses, approval and proposals

Draft, In review, Approved, Outdated. Approval and whether a project requires it are the owner's alone:

- `docs approve` and `docs approval` are refused in a session. There's no MCP tool for either. The owner approves in the Notes page.
- `docs gate PROJECT` says what stops the project being built: a gated document (PRD and Technical Spec) that's missing, not approved, or changed since. It exits non-zero while it blocks. A project with approval off is never blocked.
- An agent's write to an approved document, or one that would approve a document or change a project's approval, becomes a note proposal the owner accepts (`AgentWrite`, in `guard.go`). Accepting is the approval, at the next version. Ticking to-dos stays direct.
- An agent can't remove or move a project note or an approved document (`AgentMayRemove`).
- A write from an app or daemon that doesn't know Project Docs isn't guarded. The hash shows it: the document reads as changed since approval, and the gate blocks until the owner approves it again.

## Staleness

`docs link DOC --to SOURCE[#SECTION] [--in SECTION]` writes a Based on line with the source's current hash, and running it again re-confirms it after a review. When the source's content moves on, `docs summary` reports the document as stale; when the source is gone, as gone. Nothing changes by itself: an agent re-reads the source, and only then links again.

## Attach, RAG and diagrams

`docs new --project P` and `docs attach PROJECT --project P` attach the project note and every document, on any host ([notes.md](notes.md#projects)). Each attachment is what makes a document reach agents: `notes --project .` lists it, and RAG indexes it.

RAG indexes each note attached to the project on its own ([rag.md](rag.md)), so a note under an attached project that isn't attached itself isn't indexed: use `docs attach` rather than `notes attach` on the project note. What's embedded leaves out the first line, Based on lines and `_To fill:` lines (`IndexText`): a template's prompts would come back from a search as if they were facts about the project, and a status change would embed the document again for nothing. The note itself keeps them all.

`docs diagram DOC --canvas ID [--section S] [--from TEXT]` puts a canvas in a document as an embed, which is how the owner finds it, with what it was drawn from and the date.

## Export

`docs export PROJECT --to DIR [--force] [--dry-run]` writes the project into a repository as Markdown: the project note as `README.md`, area folders for a full pack, links between documents made relative, each file starting with a comment naming its note and saying Notes are the source. It refuses to overwrite a file without that comment unless `--force`, never deletes, and refuses when it finds a secret. `--format md|zip|html|pdf|json` goes through the ordinary note export ([notes.md](notes.md#import-and-export)).

## Skills

Seven guides ship in the program (`core/projectdocs/skills/`): `project-brainstorm`, `write-prd`, `write-spec`, `plan-work`, `build-from-spec`, `review-against-prd`, `draw-diagram`. `docs skill` lists them and `docs skill NAME` (MCP `docs_skill`) prints one, so any agent has them without anything installed.

`docs skills install [--agent claude|codex|shared|opencode] [--force]` writes each as `<name>/SKILL.md` in the agent's user skills folder, the folders of `core/envsync`, and says what it wrote. It does so only when run: enowX never installs them by itself. Each file ends with a hash of its text, so a later install updates a file as enowX wrote it and keeps one the user changed, unless `--force`. Only `SKILL.md` is written; what the user put beside it stays.

## What agents are told

The feature `projectdocs` (Settings → Features) adds its part to the guide and serves the `docs_*` tools. The rules: before changing a project read its documents, run `docs gate` before building, update documents in the same turn as the commit, record decisions in the ADR document, never invent facts, mark what's assumed `{{Assumption}}` and what's unchecked `{{Unverified}}`.

The Notes page's agent ([notes.md](notes.md#asking-an-agent)) gets the read-only tools alone, `docs_summary`, `docs_gaps`, `docs_topics`, `docs_catalog` and `docs_skill`, and brainstorms by `project-brainstorm` with its writes as proposals.

## GOD of Agents

`docs plan PROJECT` reads the tasks of Work Breakdown and Backlog, written `- [ ] T-1: verb phrase (stage, depends on T-0)`, and says what's ready; it delegates nothing. `goa delegate --doc NOTE[#SECTION]` refuses while the gate blocks, and otherwise starts the worker's prompt with that section, what it's based on and the criteria that apply. `docs progress PROJECT` shows documents, gate, tasks and criteria. `docs verify PROJECT --task ID` sets a task's result beside its criteria; whoever checks it ticks them with `notes check`, only on evidence. See [goa.md](goa.md).

## Commands

| Command | MCP | What it does |
|---|---|---|
| `docs new TITLE [--size core\|full] [--approval required\|off] [--with KEY,KEY] [--template NAME] [--group G] [--team T] [--project P [--host H]]` | `docs_new` | Makes a project |
| `docs list` | `docs_list` | The projects |
| `docs summary PROJECT [--json]` | `docs_summary` | Statuses, progress, open questions, stale, unfilled, gate |
| `docs catalog [--json]` | `docs_catalog` | Every document there's a template for |
| `docs add PROJECT KEY`, `docs skip PROJECT KEY --why TEXT` | `docs_add`, `docs_skip` | Adds a document, or says why not |
| `docs status DOC [STATUS]` | `docs_status` | Reads or sets Draft, In review, Outdated |
| `docs approve DOC`, `docs approval PROJECT required\|off` | none | The owner's, refused in a session |
| `docs link DOC --to SOURCE[#SECTION] [--in SECTION]` | `docs_link` | A Based on line |
| `docs gaps PROJECT [--json]`, `docs topics` | `docs_gaps`, `docs_topics` | What's unfilled, and every topic |
| `docs gate PROJECT` | `docs_gate` | What stops it being built |
| `docs attach PROJECT --project P [--host H]` | `docs_attach` | Attaches the project and every document |
| `docs diagram DOC --canvas ID [--section S] [--from TEXT]` | `docs_diagram` | Embeds a canvas |
| `docs export PROJECT --to DIR [--force] [--dry-run]`, `--format F [--out F]` | `docs_export` | Copies it out |
| `docs templates`, `docs template save NOTE [--name N]` | none | The user's templates |
| `docs skill [NAME]`, `docs skills install [--agent A] [--force]` | `docs_skill`, none | The skills |
| `docs plan`, `docs progress`, `docs verify` | `docs_plan`, `docs_progress`, `docs_verify` | With GOD of Agents |

Sections are read and written with the notes commands: `notes outline`, `notes show --section`, `notes write --section`, `notes check`.

## Mixed versions

Nothing in the protocol is needed for a document to exist: a daemon or app of before keeps the first line and shows an ordinary note, and syncs it as one. It can't show statuses or guard writes, which `ChangedSinceApproval` catches afterwards. The `docs` commands need a command line of this version on the host that runs them, as the notes they read are on any daemon.
