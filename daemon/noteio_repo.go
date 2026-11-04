package daemon

import (
	"errors"

	"github.com/enowx/enowx/core/notesio"
	"github.com/enowx/enowx/proto"
)

// exportRepo writes one project's documents into a folder on this host.
func (s *Server) exportRepo(p proto.NoteExportParams) (proto.NoteExported, error) {
	if len(p.IDs) != 1 || p.Group != "" || p.All {
		return proto.NoteExported{}, errors.New("export: a repo export takes one project")
	}
	if p.Out == "" {
		return proto.NoteExported{}, errors.New("export: a repo export needs a folder")
	}
	out, err := hostPath(p.Out)
	if err != nil {
		return proto.NoteExported{}, err
	}
	res, err := notesio.ExportRepo(s.notesFor(p.IDs[0]), p.IDs[0], out, notesio.RepoOptions{Force: p.Force, DryRun: p.DryRun})
	if err != nil {
		return proto.NoteExported{}, err
	}
	return proto.NoteExported{Path: out, Size: res.Size, Notes: res.Notes, Files: res.Files}, nil
}
