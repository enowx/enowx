package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/enowx/enowx/proto"
)

// A project taken out of Notes: into a repository, or as a file.

func init() {
	docsCommands["export"] = docsCommand{"export PROJECT --to DIR [--force] [--dry-run] | --format md|zip|html|pdf|json [--out F]   into a repository as Markdown, or as a file", runner.docsExport}
}

func (r runner) docsExport(args []string) error {
	const usage = "export PROJECT --to DIR [--force] [--dry-run] | export PROJECT --format md|zip|html|pdf|json [--out F]"
	fs := docsFlags("export")
	to := fs.String("to", "", "")
	force := fs.Bool("force", false, "")
	dry := fs.Bool("dry-run", false, "")
	format := fs.String("format", "", "")
	out := fs.String("out", "", "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	bad := errors.New("usage: enowx-app docs " + usage)
	repo, file := *to != "", *format != ""
	if len(pos) != 1 || repo == file || repo && *out != "" || file && (*force || *dry) {
		return bad
	}
	if file && !slices.Contains([]string{proto.ExportMarkdown, proto.ExportZip, proto.ExportHTML, proto.ExportPDF, proto.ExportJSON}, *format) {
		return bad
	}
	p, _, err := r.docsProject(pos[0])
	if err != nil {
		return err
	}
	if file {
		if *out == "" {
			*out = "."
		}
		abs, err := filepath.Abs(*out)
		if err != nil {
			return err
		}
		ex, err := r.c.ExportNotes(proto.NoteExportParams{IDs: []string{p.Note.ID}, Format: *format, Out: abs})
		if err != nil {
			return err
		}
		fmt.Fprintf(r.out, "Exported %d notes to %s (%d bytes).\n", ex.Notes, ex.Path, ex.Size)
		return nil
	}
	abs, err := filepath.Abs(*to)
	if err != nil {
		return err
	}
	ex, err := r.c.ExportNotes(proto.NoteExportParams{IDs: []string{p.Note.ID}, Format: proto.ExportRepo, Out: abs, Force: *force, DryRun: *dry})
	if err != nil {
		return err
	}
	if *dry {
		fmt.Fprintf(r.out, "Would write %d files to %s:\n", len(ex.Files), ex.Path)
	} else {
		fmt.Fprintf(r.out, "Wrote %d files to %s:\n", len(ex.Files), ex.Path)
	}
	for _, f := range ex.Files {
		fmt.Fprintf(r.out, "  %s\n", f)
	}
	return nil
}
