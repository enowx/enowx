package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// What the docs commands share: finding a project and a document, and saving
// by the write guard.

// docsFlags is a flag set for a docs command, its errors left to the caller.
func docsFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// docsParse reads flags and positional arguments in any order.
func docsParse(fs *flag.FlagSet, args []string, usage string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, errors.New("usage: enowx-app docs " + usage)
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos, args = append(pos, fs.Arg(0)), fs.Args()[1:]
	}
}

// docsMayMake refuses what can't be a proposal, for the Notes page's agent.
func docsMayMake() error {
	if os.Getenv(proto.EnvNotesPropose) != "" {
		return errors.New("docs: here notes are only proposed: notes write or new, which the user accepts")
	}
	return nil
}

// docsOwnerOnly refuses in an agent's session.
func docsOwnerOnly(what string) error {
	if os.Getenv(EnvSession) != "" || os.Getenv(proto.EnvNotesPropose) != "" {
		return errors.New("docs: only the owner " + what + ", in the Notes page")
	}
	return nil
}

// docsProject loads the project of the note ref names, itself or under it.
func (r runner) docsProject(ref string) (*projectdocs.Project, []proto.NoteInfo, error) {
	n, err := r.findNote(ref)
	if err != nil {
		return nil, nil, err
	}
	list, err := r.notesSeen()
	if err != nil {
		return nil, nil, err
	}
	p, err := projectdocs.Load(list, n.ID, r.c.Note)
	if err != nil {
		return nil, nil, err
	}
	return p, list, nil
}

// docsDoc is the document ref names: its ID or title, or with inProject, its
// kind or catalog key too.
func (r runner) docsDoc(ref, inProject string) (*projectdocs.Project, *projectdocs.Doc, error) {
	if inProject != "" {
		p, _, err := r.docsProject(inProject)
		if err != nil {
			return nil, nil, err
		}
		d := p.Doc(ref)
		if d == nil {
			return nil, nil, fmt.Errorf("docs: %q has no document %q", p.Note.Title, ref)
		}
		return p, d, nil
	}
	n, err := r.findNote(ref)
	if err != nil {
		return nil, nil, fmt.Errorf("%w (by its kind, it needs --in-project)", err)
	}
	p, _, err := r.docsProject(n.ID)
	if err != nil {
		return nil, nil, err
	}
	d := p.Doc(n.ID)
	if d == nil {
		return nil, nil, fmt.Errorf("docs: %q isn't a document of a project", n.Title)
	}
	return p, d, nil
}

// docsSave saves n, changed from before, as the guard says; false when it
// went as a proposal, which has said so.
func (r runner) docsSave(before string, n proto.Note) (bool, error) {
	if os.Getenv(proto.EnvNotesPropose) != "" {
		return false, r.proposeNote(n)
	}
	if proposed, err := r.guardNoteWrite(n.Title, before, n); proposed || err != nil {
		return false, err
	}
	_, err := r.c.SaveNote(n)
	return err == nil, err
}

// docsTeam is the space a new project goes in, as notes new decides.
func (r runner) docsTeam(name string) (string, error) {
	team, err := r.findSpace(name)
	if err != nil {
		return "", err
	}
	if here, _ := sessionTeam(); here != "" {
		if name != "" && team != here {
			return "", errors.New("docs: this session is in a team's project: its notes go to that team")
		}
		return here, nil
	}
	return team, nil
}

// docsDescendants are the notes under id.
func docsDescendants(list []proto.NoteInfo, id string) []string {
	var out []string
	for pending := []string{id}; len(pending) > 0; pending = pending[1:] {
		for _, n := range list {
			if n.Parent == pending[0] {
				out = append(out, n.ID)
				pending = append(pending, n.ID)
			}
		}
	}
	return out
}

// statusNamed is the status spelled name, in any case.
func statusNamed(name string) (string, error) {
	for _, s := range projectdocs.Statuses {
		if strings.EqualFold(s, name) {
			return s, nil
		}
	}
	return "", fmt.Errorf("docs: no status %q: %s", name, strings.Join(projectdocs.Statuses, ", "))
}
