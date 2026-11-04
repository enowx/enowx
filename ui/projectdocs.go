package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/notes"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/notesview"
	"github.com/enowx/enowx/ui/projectdocsview"
)

// projectDocsPanel is what the Notes page shows above a page of Project Docs.
func (a *App) projectDocsPanel(c *ui.Context, p notesview.DocsPage) {
	a.projectDocs.Panel(c, a.projectDocsOptions(), projectdocsview.Page{
		Note: p.Note, Unsaved: p.Unsaved, Flush: p.Flush, Rewrite: p.Rewrite, Proposals: p.Proposals,
	})
}

// projectDocsOptions gives Project Docs the calls the Notes page has, and those
// it adds.
func (a *App) projectDocsOptions() projectdocsview.Options {
	n := a.notesOptions()
	cl := a.client
	return projectdocsview.Options{
		Host: n.Host, Team: n.Team, Notes: n.Notes,
		Load: n.Load, Save: n.Save, Delete: n.Delete,
		Read:           func(id string) (proto.Note, error) { return cl.Note(id) },
		Main:           a.onMain,
		Open:           func(id string) { a.notes.Select(a.notesOptions(), id) },
		AttachProjects: a.openAttach,
		Attach: func(id string, projects []proto.NoteProject, done func(proto.NoteInfo, error)) {
			go func() {
				info, err := cl.AttachNote(id, projects)
				a.onMain(func() { done(info, err) })
			}()
		},
		Folder: a.projectFolderHere,
	}
}

// projectFolderHere is where a note's project p is on the connected host, ""
// when it isn't there.
func (a *App) projectFolderHere(p proto.NoteProject) string {
	for _, cat := range a.catalogs.list {
		if cat.Host != a.daemonInfo.ID {
			continue
		}
		for _, cp := range cat.Projects {
			if notes.IsProject(p, cat.Host, cp) {
				return cp.Path
			}
		}
	}
	return ""
}
