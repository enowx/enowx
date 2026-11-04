package agent

// ProjectDocsGuide is what a session's agent is told of Project Docs.
const ProjectDocsGuide = `Project Docs: a project documented as ordinary notes, one project note with its documents under it (PRD, Technical Spec, ADR, ERD...).
    "$ENOWX_BIN" docs list                     # the projects; docs summary PROJECT: statuses, progress, open questions, stale, gate
    "$ENOWX_BIN" docs new TITLE --size core|full --project .
    "$ENOWX_BIN" docs catalog | gaps PROJECT | add PROJECT KEY | skip PROJECT KEY --why TEXT
    "$ENOWX_BIN" docs link DOC --to SOURCE#SECTION --in SECTION   # "based on": the summary says when the source moves on
    "$ENOWX_BIN" docs gate PROJECT | status DOC [STATUS] | diagram DOC --canvas ID | export PROJECT --to DIR
    "$ENOWX_BIN" docs skill [NAME]             # how to: project-brainstorm, write-prd, write-spec, plan-work, build-from-spec, review-against-prd, draw-diagram
Read and write them a section at a time (notes outline, notes show --section, notes write --section). When a project has documents:
- Before changing the project, read the ones that bear on it (docs list, notes --project .).
- Run docs gate before building. While it blocks, stop and tell the owner what to approve: only the owner approves, in the Notes page.
- A change you write to an approved document becomes a proposal the owner accepts.
- Update the documents in the same turn as the commit, record decisions in the Architecture Decision Records document, and log changes in Change history.
- Never invent facts: mark what you suppose {{Assumption}} and what you couldn't check {{Unverified}}.`

// ProjectDocsNotesGuide is what the Notes page's agent is told of it.
const ProjectDocsNotesGuide = `A note whose first line starts "Project Docs:" belongs to a project documented as notes, with its documents under the project note. Keep that first line as it is, and write a section at a time. docs_summary, docs_gaps, docs_topics and docs_catalog read how the project stands; docs_skill says how to go about it. To brainstorm a project, follow docs_skill project-brainstorm: one topic at a time from docs_gaps, each answer written into its section as a proposal, not left in chat; "up to you" gets your suggestion with its reason, marked {{Assumption}}. Only the owner approves a document: never write a status yourself.`

// NotesDocsTools are the Project Docs tools the Notes page's agent gets: those
// that only read, as anything else it does must go through proposals.
var NotesDocsTools = []string{"docs_summary", "docs_gaps", "docs_topics", "docs_catalog", "docs_skill"}

// notesDocsAllowed is NotesDocsTools as an agent's allowlist names them.
func notesDocsAllowed() []string {
	out := make([]string, len(NotesDocsTools))
	for i, t := range NotesDocsTools {
		out[i] = "mcp__enowx__" + t
	}
	return out
}
