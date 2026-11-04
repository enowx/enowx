package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// The write guard: what an agent's session may do to Project Docs. The owner
// approves, so a session's changes to approved documents become proposals,
// and it can't approve one by any way round. Outside a session, and for
// ordinary notes, nothing changes.

// agentSession says a session's agent is writing notes directly, not the
// Notes page's agent, whose every change is a proposal already.
func agentSession() bool {
	return os.Getenv(EnvSession) != "" && os.Getenv(proto.EnvNotesPropose) == ""
}

// guardNewNote refuses a session's new note that says it's approved.
func guardNewNote(body string) error {
	if m, ok := projectdocs.Parse(body); agentSession() && ok && m.Status == projectdocs.StatusApproved {
		return errors.New("notes: only the owner approves a document: make it without Approved")
	}
	return nil
}

// guardNoteWrite proposes n instead of saving it when the guard says so; true
// means it did, and nothing's left to save.
func (r runner) guardNoteWrite(before string, n proto.Note) (bool, error) {
	if !agentSession() {
		return false, nil
	}
	action, body, why := projectdocs.AgentWrite(before, n.Body)
	if action != projectdocs.WritePropose {
		return false, nil
	}
	fmt.Fprintln(r.out, why)
	n.Body = body
	return true, r.proposeNote(n)
}

// guardNoteRemove refuses a session's removing or moving a project, an
// approved document, or a note with one under it: deleting an area page takes
// the documents in it along.
func (r runner) guardNoteRemove(id string) error {
	if !agentSession() {
		return nil
	}
	list, err := r.notesSeen()
	if err != nil {
		return err
	}
	for pending := []string{id}; len(pending) > 0; {
		cur := pending[0]
		pending = pending[1:]
		n, err := r.c.Note(cur)
		if err != nil {
			return err
		}
		if err := projectdocs.AgentMayRemove(n.Body); err != nil {
			return err
		}
		for _, c := range list {
			if c.Parent == cur {
				pending = append(pending, c.ID)
			}
		}
	}
	return nil
}

// guardNoteRestore proposes a session's bringing back a version when writing
// it would be proposed: an old version could undo an approved document, or
// be an approved one.
func (r runner) guardNoteRestore(id, version string) (bool, error) {
	if !agentSession() {
		return false, nil
	}
	cur, err := r.c.Note(id)
	if err != nil {
		return false, err
	}
	v, err := r.c.NoteVersion(id, version)
	if err != nil {
		return false, err
	}
	before := cur.Body
	cur.Body = v.Body
	return r.guardNoteWrite(before, cur)
}

// guardNoteAccept refuses a session's accepting a proposal the guard would
// have made: accepting it is the owner's approval, so proposing and then
// accepting would be approving.
func (r runner) guardNoteAccept(id string) error {
	if !agentSession() {
		return nil
	}
	list, err := r.c.NoteProposals()
	if err != nil {
		return err
	}
	for _, p := range list {
		if p.ID != id {
			continue
		}
		if action, _, _ := projectdocs.AgentWrite(p.Before, p.Note.Body); action == projectdocs.WritePropose {
			return errors.New("notes: only the owner accepts this change to Project Docs, in the Notes page")
		}
	}
	return nil
}
