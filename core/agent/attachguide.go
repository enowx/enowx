package agent

// AttachGuide is what agents are told of notes attached to projects: a
// project's plans and to-dos live in the user's notes, attached to it.
const AttachGuide = `Notes attach to projects, of any host: a project's plans and TODOs are the notes attached to it. "$ENOWX_BIN" notes --project . lists this session's project's (MCP notes_for_project); read the one your task belongs to, and tick each to-do in the same turn as the commit that finishes it ("notes check NOTE --item TEXT"). A note attached to several projects (a frontend here, a backend on a server) names the others in notes projects NOTE: read their context ("$ENOWX_BIN" project context NAME, then project read NAME FILE, read-only, on any host the app reaches) before changing what they share. "notes attach NOTE --project P [--host H]" and "notes detach NOTE --project P" change a note's projects; a note made with "notes new" in a project's session is attached to it (--no-attach not to).`

// notesProjectsGuide is what the Notes page's agent is told of its
// note's projects.
const notesProjectsGuide = `The note may be attached to projects (notes_projects NOTE lists them, with their hosts). For a plan or TODO about a project, read it first, read-only: project_context PROJECT gives its stack, branch, last commit, folders and README; project_read PROJECT FILE reads a file when the plan needs it (the auth code before a plan to fix it). Name only files and modules that exist. Never write to a project: what you write is the note. A project whose host is out of reach answers with the context it last had; say so.`
