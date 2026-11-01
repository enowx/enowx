package ui

import (
	"fmt"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/notesview"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// noteExportForm is the Export dialog.
type noteExportForm struct {
	open   bool
	pick   notesview.ExportPick
	format int
	busy   bool
	err    string
}

var exportFormats = []struct{ label, format, about string }{
	{"Markdown", proto.ExportMarkdown, "A .md file, with a header saying what it is; several notes as a .zip of them."},
	{"Folders", proto.ExportZip, "A .zip of folders of Markdown with their images: Obsidian opens it as a vault, Notion imports it, and Import brings it back as it was."},
	{"HTML", proto.ExportHTML, "One page to open in any browser, its images in it."},
	{"PDF", proto.ExportPDF, "To print or send."},
	{"Backup", proto.ExportJSON, "Everything in one .json, images included, for Import to bring back on any host."},
}

func (a *App) openNoteExport(p notesview.ExportPick) {
	if a.client == nil {
		return
	}
	format := 0
	if p.All || p.Group != "" {
		format = 1
	}
	a.noteExport = &noteExportForm{open: true, pick: p, format: format}
}

func (a *App) noteExportDialog(c *ui.Context) {
	f := a.noteExport
	if f == nil {
		return
	}
	if !f.open {
		a.noteExport = nil
		return
	}
	widget.FormSheet(c, &f.open, 520, "Export “"+f.pick.Title+"”", "Written on "+a.host.Name+", then saved on this device where you pick.", func() {
		labels := make([]string, len(exportFormats))
		for i, ef := range exportFormats {
			labels[i] = ef.label
		}
		if i := widget.Segmented(c, labels, f.format, nil); i >= 0 && !f.busy {
			f.format = i
		}
		ui.Text(c, exportFormats[f.format].about).FontSize(theme.TextLabel).TextColor(theme.Muted)
		if f.busy {
			ui.Text(c, "Exporting…").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
		if f.err != "" {
			ui.Text(c, f.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
	}, func() {
		b := widget.PrimaryButton(c, icon.ArrowUp, "Export…", "")
		if f.busy {
			b.Disabled(true).Opacity(0.45)
		} else if b.Clicked() {
			a.runNoteExport(f)
		}
	})
}

// runNoteExport has the host write the export, then saves it where the
// user picks.
func (a *App) runNoteExport(f *noteExportForm) {
	cl := a.client
	if cl == nil {
		return
	}
	f.busy, f.err = true, ""
	p := proto.NoteExportParams{Group: f.pick.Group, All: f.pick.All, Format: exportFormats[f.format].format}
	if f.pick.ID != "" {
		p.IDs = []string{f.pick.ID}
	}
	saveAs := a.saveAs
	if saveAs == nil {
		saveAs = func(name string) (string, error) {
			return mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: "Save the export", DefaultPath: name})
		}
	}
	go func() {
		ex, err := cl.ExportNotes(p)
		file := ""
		if err == nil {
			file, err = saveAs(ex.Name)
		}
		if err == nil && file != "" {
			var out *os.File
			if out, err = os.Create(file); err == nil {
				err = cl.ReadExport(ex.Path, out)
				if cerr := out.Close(); err == nil {
					err = cerr
				}
			}
		}
		a.onMain(func() {
			f.busy = false
			switch {
			case err != nil:
				f.err = err.Error()
			case file != "":
				if a.noteExport == f {
					a.noteExport = nil
				}
				a.notify("Exported " + textfmt.Plural(ex.Notes, "a note", fmt.Sprintf("%d notes", ex.Notes)) + " to " + file)
			}
		})
	}()
}
