package proto

import "time"

// Notes brought in from other apps (Notion, Obsidian, Markdown, Evernote,
// Google Keep, Apple Notes, HTML, an enowX backup) and taken out of
// enowX. An import runs on the host as a job the app and agents follow;
// files of the app's machine are staged on the host first.
const (
	// MethodNoteImport starts an import: NoteImportParams → NoteImportStatus.
	MethodNoteImport       = "note.import"
	MethodNoteImportStatus = "note.import.status" // NoteImportRef → NoteImportStatus
	MethodNoteImportCancel = "note.import.cancel" // NoteImportRef
	MethodNoteImports      = "note.imports"       // → []NoteImportStatus, oldest first
	// MethodNoteStage puts a file of the app's machine on the host, in
	// parts, for an import: NoteStage → NoteStaged.
	MethodNoteStage = "note.stage"
	// MethodNoteExport writes notes as a file on the host: NoteExportParams
	// → NoteExported.
	MethodNoteExport = "note.export"
	// MethodNoteExportBytes reads a part of an export made without Out,
	// for the app to save on its machine: NoteExportBytesParams → FileBytes.
	MethodNoteExportBytes = "note.export.bytes"
)

// EventNoteImport carries a NoteImportStatus as an import goes.
const EventNoteImport = "notes.import"

// Import sources (NoteImportParams.From); "" finds out from the files.
const (
	ImportNotion    = "notion" // a Notion export: Markdown & CSV, or HTML (.zip or folder)
	ImportNotionAPI = "notion-api"
	ImportObsidian  = "obsidian" // a vault, or any folder of Markdown
	ImportMarkdown  = "md"       // a .md or .txt file
	ImportEnex      = "enex"     // Evernote
	ImportKeep      = "keep"     // Google Keep, from Google Takeout
	ImportHTML      = "html"     // HTML files, Apple Notes' exported among them
	ImportBackup    = "enowx"    // an enowX export (.zip) or backup (.json)
)

// What an import does with a note it made before from the same source.
const (
	ImportSkip      = "skip"      // leave it (the default)
	ImportOverwrite = "overwrite" // write it again
	ImportDuplicate = "duplicate" // make another
)

// NoteImportParams start an import of Path, a file or folder on the host
// (a staged file's among them).
type NoteImportParams struct {
	Path string `json:"path"`
	From string `json:"from,omitempty"`
	// Group puts the notes at the top in this group, made if missing ("" for
	// the groups their source's folders give, or none).
	Group string `json:"group,omitempty"`
	// Project is the project the notes are for, kept with the import.
	Project string `json:"project,omitempty"`
	Mode    string `json:"mode,omitempty"`
	// DryRun only reads the source: the status's Preview says what would
	// be made.
	DryRun bool `json:"dryRun,omitempty"`
	// RowsAsNotes makes a Notion database a note per row under a page, not
	// a table.
	RowsAsNotes bool `json:"rowsAsNotes,omitempty"`
	// Token is a Notion integration's, for ImportNotionAPI; never kept.
	Token string `json:"token,omitempty"`
}

// NoteImportRef names an import.
type NoteImportRef struct {
	ID string `json:"id"`
}

// Import states.
const (
	ImportRunning  = "running"
	ImportDone     = "done"
	ImportFailed   = "failed"
	ImportCanceled = "canceled"
)

// NoteImportStatus is how an import goes, and what it made once done.
type NoteImportStatus struct {
	ID      string    `json:"id"`
	Source  string    `json:"source"` // the file or folder's name
	From    string    `json:"from,omitempty"`
	DryRun  bool      `json:"dryRun,omitempty"`
	State   string    `json:"state"`
	Phase   string    `json:"phase,omitempty"`
	Done    int       `json:"done"`
	Total   int       `json:"total"`
	Error   string    `json:"error,omitempty"`
	Started time.Time `json:"started"`
	// Preview is what the import makes (a dry run's) or made.
	Preview *NoteImportPreview `json:"preview,omitempty"`
	Report  *NoteImportReport  `json:"report,omitempty"`
}

// NoteImportPreview is what an import would make.
type NoteImportPreview struct {
	From   string            `json:"from"`
	Notes  []NoteImportEntry `json:"notes"`
	Assets int               `json:"assets"`
	// Conflicts are the titles of notes there already, in the same place.
	Conflicts []string `json:"conflicts,omitempty"`
}

// NoteImportEntry is a note an import makes: its title, the titles of
// the notes it goes under, its group, and what becomes of it.
type NoteImportEntry struct {
	Title string   `json:"title"`
	Path  []string `json:"path,omitempty"`
	Group string   `json:"group,omitempty"`
	// Action is "new", or for a note made before from the same source the
	// mode's: "skip", "overwrite" or "duplicate".
	Action string `json:"action"`
}

// NoteImportReport is what an import did.
type NoteImportReport struct {
	Imported    int               `json:"imported"`
	Overwritten int               `json:"overwritten,omitempty"`
	Skipped     []NoteImportIssue `json:"skipped,omitempty"`
	// Unconverted are parts of notes enowX has no form for (an embed, an
	// equation), kept as text or left out.
	Unconverted []NoteImportIssue `json:"unconverted,omitempty"`
	Notes       []string          `json:"notes,omitempty"` // the IDs made, top first
}

// NoteImportIssue is a file or a part of one an import couldn't take, and
// why.
type NoteImportIssue struct {
	Where string `json:"where"`
	What  string `json:"what"`
}

// NoteStage is a part of a file staged for an import. The first part has
// no Path; the daemon answers with where the file goes, which the next
// parts give.
type NoteStage struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
	Data []byte `json:"data"`
}

// NoteStaged is where a staged file is on the host.
type NoteStaged struct {
	Path string `json:"path"`
}

// Export formats (NoteExportParams.Format).
const (
	ExportMarkdown = "md"   // a note's .md, or a .zip of several
	ExportZip      = "zip"  // folders of .md with their files, Obsidian's and Notion's way
	ExportHTML     = "html" // one page
	ExportPDF      = "pdf"
	ExportJSON     = "json" // a full backup, files included
	ExportRepo     = "repo" // a Project Docs project's documents, into the folder Out
)

// NoteExportParams pick the notes to export: IDs (with the notes under
// them), a group's, or all; and the format and file (on the host; "" for
// one the app reads with note.export.bytes).
type NoteExportParams struct {
	IDs    []string `json:"ids,omitempty"`
	Group  string   `json:"group,omitempty"`
	All    bool     `json:"all,omitempty"`
	Format string   `json:"format,omitempty"`
	Out    string   `json:"out,omitempty"`
	// Force and DryRun are ExportRepo's: write over files that aren't an
	// export's, and only say what would be written.
	Force  bool `json:"force,omitempty"`
	DryRun bool `json:"dryRun,omitempty"`
}

// NoteExported is the export written.
type NoteExported struct {
	Path  string `json:"path"`
	Name  string `json:"name"` // a file name for it
	Size  int64  `json:"size"`
	Notes int    `json:"notes"`
	// Files are what ExportRepo wrote or would write, under Path.
	Files []string `json:"files,omitempty"`
}

// NoteExportBytesParams read an export made without Out, from Offset.
type NoteExportBytesParams struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset,omitempty"`
}
