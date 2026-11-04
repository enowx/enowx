package projectdocsview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// exportFilesHeight is the frame of the list of files, however many.
const exportFilesHeight = 170

// exportForm is the dialog exporting a project to a repository's folder.
type exportForm struct {
	open    bool
	note    proto.NoteInfo
	folders []exportFolder
	path    string
	files   []string
	busy    bool
	refused bool // the host refused: it may be asked to overwrite
	err     string
	wrote   string
}

// exportFolder is a folder of an attached project on this host.
type exportFolder struct{ name, path string }

func (v *View) openExport(o Options, note proto.NoteInfo, m *model) {
	f := &exportForm{open: true, note: note, files: repoFiles(m)}
	f.folders = foldersOf(note.Projects, o.Folder)
	if len(f.folders) > 0 {
		f.path = f.folders[0].path
	}
	v.export = f
}

// foldersOf are the projects found on this host.
func foldersOf(projects []proto.NoteProject, folder func(proto.NoteProject) string) []exportFolder {
	if folder == nil {
		return nil
	}
	var out []exportFolder
	for _, p := range projects {
		if at := folder(p); at != "" {
			out = append(out, exportFolder{p.Name, at})
		}
	}
	return out
}

func (v *View) exportDialog(c *ui.Context, o Options) {
	f := v.export
	if f == nil {
		return
	}
	if !f.open && !f.busy {
		v.export = nil
		return
	}
	f.open = true // the export under way says how it went
	widget.FormSheet(c.Key("export-docs"), &f.open, 560, "Export to repo", "Written on "+o.Host+". The notes stay the source.", func() {
		if len(f.folders) > 0 {
			widget.Field(c, "A folder of an attached project", func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					for _, d := range f.folders {
						folderRow(c.Key(d.path), f, d)
					}
				})
			})
		}
		widget.Field(c, "Folder on "+o.Host, func() {
			widget.Input(c, &f.path, "/path/to/the/repo/docs").FillWidth()
		})
		widget.Field(c, "It writes "+plural(len(f.files), "file", "files"), func() {
			ui.Scroll(c).FillWidth().Height(exportFilesHeight).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
				ui.Column(c).FillWidth().Padding(6, 10).Children(func() {
					for i, name := range f.files {
						ui.Row(c.Key(i)).FillWidth().Height(22).Gap(7).AlignItems(ui.Center).MinWidth(0).Children(func() {
							ui.Icon(c, icon.File).FontSize(11).TextColor(theme.Muted).Shrink(0)
							running(c, name, runStyle{Size: 11.5, Color: theme.TextSoft}).Grow(1)
						})
					}
				})
			})
		})
		ui.Row(c).FillWidth().Height(lineHeight).MinWidth(0).Label("Export result").Children(func() {
			switch {
			case f.err != "":
				line(c, icon.Alert, f.err, theme.Error)
			case f.wrote != "":
				line(c, icon.Check, "Written to "+f.wrote, theme.Done)
			case f.busy:
				line(c, nil, "Writing…", theme.TextSoft)
			}
		})
	}, func() {
		folder := strings.TrimSpace(f.path)
		blocked := f.busy || folder == ""
		if f.refused {
			b := widget.Button(c, widget.Danger, nil, "Overwrite…", "")
			if blocked {
				b.Disabled(true).Opacity(0.45)
			} else if b.Clicked() {
				v.ask("Overwrite these files?",
					"Files in "+folder+" that weren't exported from Notes are replaced, and what they hold is lost unless the repository keeps it.",
					"Overwrite", true, func() { v.runExport(o, f, folder, true) })
			}
			return
		}
		b := widget.Button(c, widget.Primary, icon.ArrowUp, "Export…", "")
		if blocked {
			b.Disabled(true).Opacity(0.45)
		} else if b.Clicked() {
			v.ask("Export "+plural(len(f.files), "file", "files")+"?",
				"They're written in "+folder+". Files exported from Notes before are replaced. Any other file is left alone: the host refuses, and you can decide then.",
				"Export", false, func() { v.runExport(o, f, folder, false) })
		}
	})
	if !f.open && f.busy {
		f.open = true
	}
}

func folderRow(c *ui.Context, f *exportForm, d exportFolder) {
	b := ui.ButtonBase(c).FillWidth().Height(rowHeight).PaddingX(8).Gap(8).Radius(6).Cursor(ui.CursorPointer).MinWidth(0).AlignItems(ui.Center).Label(d.name)
	picked := strings.TrimSpace(f.path) == d.path
	if picked {
		b.Background(theme.Selected)
	} else if b.Hovered() {
		b.Background(theme.Hover)
	}
	b.Children(func() {
		ui.Icon(c, icon.Folder).FontSize(12).TextColor(theme.TextSoft).Shrink(0)
		running(c, d.name+" · "+d.path, runStyle{Size: 12, Color: theme.Text}).Grow(1)
	})
	if b.Clicked() && !f.busy {
		f.path, f.refused, f.err, f.wrote = d.path, false, "", ""
	}
}

func (v *View) runExport(o Options, f *exportForm, folder string, force bool) {
	f.busy, f.err, f.wrote = true, "", ""
	o.ExportRepo(f.note.ID, folder, force, func(written string, err error) {
		f.busy = false
		if err != nil {
			f.err, f.refused = err.Error(), !force
			return
		}
		f.refused, f.wrote = false, written
	})
}
