package ui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Import and export in the Notes page. An import's files are this
// device's: the app sends them to the host (a folder or several files as
// one .zip), where its daemon reads them, shows what it would make, then
// makes it. An export is written on the host and saved here.

// noteImportForm is the Import notes dialog. It belongs to the main
// thread.
type noteImportForm struct {
	open      bool
	host      string // the host it imports into
	paths     []string
	notionAPI bool
	token     string // a Notion integration's, sent for this import only
	cred      string // the vault's Notion credential used instead, its ID
	keep      bool   // keep the typed token in the vault
	every     int    // how often to import again (notionEvery)
	group     string
	project   string // a project's folder on this host the notes are for, "" none
	mode      int
	rows      bool
	busy      string // what it does before the host answers
	staged    string // the files, on the host
	job       *proto.NoteImportStatus
	seen      map[string]proto.NoteImportStatus // the imports' last events
	err       string
}

var importModes = []string{proto.ImportSkip, proto.ImportOverwrite, proto.ImportDuplicate}

// openNoteImport opens the dialog, with files dropped on the page when
// there are.
func (a *App) openNoteImport(paths []string) {
	if a.client == nil {
		return
	}
	a.noteImport = &noteImportForm{open: true, host: a.host.ID, paths: paths}
}

// noteImportEvent follows the import the dialog started.
func (a *App) noteImportEvent(data json.RawMessage) func() {
	var st proto.NoteImportStatus
	if json.Unmarshal(data, &st) != nil {
		return nil
	}
	return func() {
		f := a.noteImport
		if f == nil {
			return
		}
		if f.seen == nil {
			f.seen = map[string]proto.NoteImportStatus{}
		}
		f.seen[st.ID] = st
		if f.job != nil && f.job.ID == st.ID {
			f.job = &st
			if st.State == proto.ImportFailed {
				f.job, f.err = nil, st.Error
			}
		}
	}
}

func (f *noteImportForm) source() string {
	switch {
	case f.notionAPI:
		return "Notion"
	case len(f.paths) == 1:
		return filepath.Base(f.paths[0])
	case len(f.paths) > 1:
		return fmt.Sprintf("%d files", len(f.paths))
	}
	return ""
}

func (a *App) noteImportDialog(c *ui.Context) {
	f := a.noteImport
	if f == nil {
		return
	}
	if !f.open || f.host != a.host.ID {
		if j := f.job; j != nil && j.State == proto.ImportRunning && !j.DryRun && a.client != nil {
			cl, id := a.client, j.ID
			go cl.CancelNoteImport(id) // closing stops it
		}
		a.noteImport = nil
		return
	}
	j := f.job
	done := j != nil && j.State == proto.ImportDone && !j.DryRun
	previewed := j != nil && j.State == proto.ImportDone && j.DryRun && j.Preview != nil
	running := f.busy != "" || j != nil && j.State == proto.ImportRunning
	sheet := widget.FormSheet
	if done {
		sheet = widget.FinishedSheet // the import is over: Done alone
	}
	sheet(c, &f.open, 560, "Import notes",
		"From Notion, Obsidian or any folder of Markdown, Evernote, Google Keep, Apple Notes (HTML), or an enowX export, into the notes on "+a.host.Name+".", func() {
			switch {
			case done:
				a.importReport(c, *j)
			case previewed:
				importPreview(c, *j.Preview)
			default:
				a.importSource(c, f, running)
			}
			if running {
				importProgress(c, f)
			}
			if f.err != "" {
				ui.Text(c, f.err).FontSize(theme.TextLabel).TextColor(theme.Error)
			}
		}, func() {
			switch {
			case done:
				if widget.PrimaryButton(c, nil, "Done", "").Clicked() {
					if r := j.Report; r != nil && len(r.Notes) > 0 {
						a.notes.Select(a.notesOptions(), r.Notes[0])
					}
					f.open = false
				}
			case running:
				if j != nil && !j.DryRun && widget.Button(c, widget.Secondary, icon.Stop, "Stop", "").Clicked() {
					cl, id := a.client, j.ID
					go cl.CancelNoteImport(id)
				}
			case previewed:
				if widget.Button(c, widget.Secondary, nil, "Back", "").Clicked() {
					f.job = nil
				}
				label := "Import " + textfmt.Plural(importCount(*j.Preview), "a note", fmt.Sprintf("%d notes", importCount(*j.Preview)))
				b := widget.PrimaryButton(c, nil, label, "")
				if importCount(*j.Preview) == 0 {
					b.Disabled(true).Opacity(0.45)
				} else if b.Clicked() {
					a.runNoteImport(f, false)
				}
			default:
				b := widget.PrimaryButton(c, nil, "Preview", "")
				if len(f.paths) == 0 && !(f.notionAPI && a.notionToken(f) != "") {
					b.Disabled(true).Opacity(0.45)
				} else if b.Clicked() {
					a.runNoteImport(f, true)
				}
			}
		})
}

// importCount is how many notes an import makes or writes again.
func importCount(pv proto.NoteImportPreview) int {
	n := 0
	for _, e := range pv.Notes {
		if e.Action != proto.ImportSkip {
			n++
		}
	}
	return n
}

// importSource is the dialog's first step: what to import, and where to.
func (a *App) importSource(c *ui.Context, f *noteImportForm, running bool) {
	if i := widget.Segmented(c, []string{"Files on this device", "Notion (API)"}, map[bool]int{false: 0, true: 1}[f.notionAPI], nil); i >= 0 && !running {
		f.notionAPI = i == 1
	}
	if f.notionAPI {
		a.notionTokenField(c, f)
	} else {
		widget.Field(c, "Files", func() {
			ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
				label, col := "A Notion export (.zip), a vault or folder, .md, .enex, Keep's Takeout, .html, a backup", theme.TextSoft
				if len(f.paths) > 0 {
					label, col = f.source(), theme.Text
				}
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, label, widget.RunStyle{Size: theme.TextLabel, Color: col})
				})
				if widget.Button(c, widget.Secondary, icon.Folder, "Choose…", "").Clicked() && !running {
					a.localPick.Open("Import notes", "Pick an export, a folder, or files: they are sent to "+a.host.Name+" and read there.", func(paths []string) {
						if g := a.noteImport; g == f {
							f.paths, f.err = paths, ""
						}
					})
				}
			})
		})
	}
	widget.Field(c, "Into the group", func() {
		widget.Input(c, &f.group, "None: the groups their folders say").FillWidth()
	})
	if len(a.projects) > 0 {
		widget.Field(c, "For a project (optional)", func() {
			ui.Row(c).FillWidth().Gap(theme.S2).Wrap().Children(func() {
				for _, p := range append([]proto.ProjectInfo{{Name: "None"}}, a.projects...) {
					b := widget.Button(c.Key("p:"+p.Path), widget.Secondary, nil, p.Name, "").MaxWidth(240)
					if f.project == p.Path {
						b.Background(theme.Selected)
					}
					if b.Clicked() {
						f.project = p.Path
					}
				}
			})
		})
	}
	widget.Field(c, "Notes imported before from the same source", func() {
		if i := widget.Segmented(c, []string{"Skip them", "Write them again", "Make copies"}, f.mode, nil); i >= 0 {
			f.mode = i
		}
	})
	switchRow(c, &f.rows, "Notion databases as a page per row", "Rather than a table, each row a page under the database's.")
}

func importProgress(c *ui.Context, f *noteImportForm) {
	label, fraction := f.busy, 0.0
	if j := f.job; j != nil && f.busy == "" {
		label = j.Phase + "…"
		if j.Total > 0 && !j.DryRun {
			label = fmt.Sprintf("%s %d of %d", j.Phase, j.Done, j.Total)
			fraction = float64(j.Done) / float64(j.Total)
		}
	}
	ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
		widget.Running(c, label, widget.RunStyle{Size: theme.TextLabel})
		widget.Meter(c, widget.Segment{Fraction: fraction, Color: theme.Now})
	})
}

// importPreview lists what an import would make.
func importPreview(c *ui.Context, pv proto.NoteImportPreview) {
	n := importCount(pv)
	summary := textfmt.Plural(n, "A note", fmt.Sprintf("%d notes", n))
	if pv.Assets > 0 {
		summary += " and " + textfmt.Plural(pv.Assets, "a file", fmt.Sprintf("%d files", pv.Assets))
	}
	ui.Text(c, summary+" to import, read as "+importFormat(pv.From)+".").FontSize(theme.TextUI).TextColor(theme.Text)
	if skipped := len(pv.Notes) - n; skipped > 0 {
		ui.Text(c, textfmt.Plural(skipped, "One was", fmt.Sprintf("%d were", skipped))+" imported before and stays as it is.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	}
	if len(pv.Conflicts) > 0 {
		ui.Text(c, "Titles there already, kept both: "+strings.Join(pv.Conflicts, ", ")).FontSize(theme.TextLabel).TextColor(theme.Warning)
	}
	ui.Scroll(c).FillWidth().MaxHeight(300).Children(func() {
		ui.Column(c).FillWidth().Gap(1).Children(func() {
			for i, e := range pv.Notes {
				if i == 300 {
					ui.Text(c, fmt.Sprintf("and %d more", len(pv.Notes)-i)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Padding(theme.S1, theme.S3)
					break
				}
				ui.Row(c.Key(i)).FillWidth().Height(theme.ControlHeight).Shrink(0).Gap(theme.S2).Padding(0, theme.S3, 0, theme.S3+min(float32(len(e.Path))*theme.S3, 120)).Children(func() {
					ui.Icon(c, icon.File).FontSize(12).TextColor(theme.TextSoft)
					ui.Row(c).Grow(1).MinWidth(0).Children(func() {
						widget.Running(c, e.Title, widget.RunStyle{})
					})
					if e.Group != "" {
						ui.Row(c).MinWidth(0).Shrink(100).MaxWidth(120).Children(func() {
							widget.Running(c.Key("group"), e.Group, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
						})
					}
					if e.Action != "new" {
						widget.Tag(c, e.Action, theme.TextSoft)
					}
				})
			}
		})
	})
}

func importFormat(from string) string {
	switch from {
	case proto.ImportNotion:
		return "a Notion export"
	case proto.ImportNotionAPI:
		return "Notion's pages"
	case proto.ImportObsidian:
		return "a vault of Markdown"
	case proto.ImportMarkdown:
		return "Markdown"
	case proto.ImportEnex:
		return "Evernote notes"
	case proto.ImportKeep:
		return "Google Keep notes"
	case proto.ImportHTML:
		return "HTML"
	case proto.ImportBackup:
		return "an enowX export"
	}
	return from
}

// importReport says what an import made.
func (a *App) importReport(c *ui.Context, st proto.NoteImportStatus) {
	r := st.Report
	if r == nil {
		return
	}
	widget.Notice(c, icon.Check, theme.Done, "Imported "+textfmt.Plural(r.Imported, "a note", fmt.Sprintf("%d notes", r.Imported))+" from "+st.Source, "", nil)
	if r.Overwritten > 0 {
		ui.Text(c, fmt.Sprintf("%d imported before written again.", r.Overwritten)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	}
	issues := func(title string, list []proto.NoteImportIssue) {
		if len(list) == 0 {
			return
		}
		ui.Text(c, title).FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft)
		ui.Scroll(c).FillWidth().MaxHeight(140).Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				for i, it := range list {
					ui.Text(c.Key(i), it.Where+": "+it.What).FontSize(theme.TextUI).TextColor(theme.Text)
				}
			})
		})
	}
	issues("Skipped", r.Skipped)
	issues("Kept as text or left out", r.Unconverted)
}

// runNoteImport sends the files to the host, if not yet, and starts the
// import: a dry run to preview, or the import itself.
func (a *App) runNoteImport(f *noteImportForm, dry bool) {
	cl := a.client
	if cl == nil {
		return
	}
	f.err = ""
	p := proto.NoteImportParams{Group: strings.TrimSpace(f.group), Project: f.project, Mode: importModes[f.mode], DryRun: dry, RowsAsNotes: f.rows}
	if f.notionAPI {
		if f.keep {
			if err := a.keepNotionToken(f); err != nil {
				f.err = err.Error()
				return
			}
		}
		p.From, p.Token = proto.ImportNotionAPI, a.notionToken(f)
		if !dry {
			a.scheduleNotion(f)
		}
	}
	paths, staged := f.paths, f.staged
	if !f.notionAPI && staged == "" {
		f.busy = "Sending to " + a.host.Name + "…"
	} else {
		f.busy = "Reading…"
	}
	go func() {
		var err error
		if p.From != proto.ImportNotionAPI && staged == "" {
			staged, err = stageLocal(cl.StageNotes, paths)
		}
		var st proto.NoteImportStatus
		if err == nil {
			p.Path = staged
			st, err = cl.ImportNotes(p)
		}
		a.onMain(func() {
			if a.noteImport != f {
				return
			}
			f.busy = ""
			if err != nil {
				f.err = err.Error()
				return
			}
			f.staged = staged
			if !dry {
				f.staged = "" // the import uses it up
			}
			// Its events may have come before its answer.
			if later, ok := f.seen[st.ID]; ok {
				st = later
			}
			f.job = &st
			if st.State == proto.ImportFailed {
				f.job, f.err = nil, st.Error
			}
		})
	}()
}
