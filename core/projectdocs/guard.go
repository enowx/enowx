package projectdocs

import "fmt"

// A write from an agent's session to a note, as the owner set: approved
// documents change through proposals, and the owner alone approves.

// WriteAction is what becomes of an agent's write.
type WriteAction int

const (
	WriteDirect  WriteAction = iota // as any note
	WritePropose                    // as a proposal the user accepts
)

// AgentWrite says what becomes of an agent changing a note from before to
// after, and the body to write or propose: an approved document's new content
// stays approved once accepted, as accepting it is the owner's approval, at
// the next version. Why says why it's a proposal.
func AgentWrite(before, after string) (action WriteAction, body, why string) {
	old, wasDoc := Parse(before)
	cur, isDoc := Parse(after)
	// What the owner alone may decide, even in a note that wasn't approved.
	switch {
	case wasDoc && old.IsProject() && (!isDoc || cur.Approval != old.Approval):
		return WritePropose, after, "only the owner changes whether a project needs approval"
	case isDoc && cur.Status == StatusApproved && (!wasDoc || old.Status != StatusApproved):
		return WritePropose, after, "only the owner approves a document"
	}
	if !wasDoc || old.Status != StatusApproved {
		return WriteDirect, after, ""
	}
	if before == after {
		return WriteDirect, after, ""
	}
	// Ticking its to-dos changes no content: acceptance criteria are ticked directly.
	if isDoc && cur == old && ContentHash(before) == ContentHash(after) {
		return WriteDirect, after, ""
	}
	why = fmt.Sprintf("%s is approved: changes to it are proposed", old.Kind)
	if !isDoc {
		return WritePropose, after, why
	}
	body, err := Approve(WithMeta(after, Meta{Kind: cur.Kind, Status: StatusApproved, Version: old.Version, Hash: old.Hash}))
	if err != nil {
		return WritePropose, after, why
	}
	return WritePropose, body, why
}

// AgentMayRemove says whether an agent may delete or move a note with body:
// not an approved document, nor a project note.
func AgentMayRemove(body string) error {
	m, ok := Parse(body)
	switch {
	case ok && m.IsProject():
		return fmt.Errorf("projectdocs: only the owner removes or moves a project")
	case ok && m.Status == StatusApproved:
		return fmt.Errorf("projectdocs: %s is approved: only the owner removes or moves it", m.Kind)
	}
	return nil
}
