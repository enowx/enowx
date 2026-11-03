package proto

import "time"

// Files: the files of a session's folder, for the side panel's Files tool.
// Paths are relative to the session's folder ("" is the folder itself),
// and never reach out of it.
const (
	MethodFileList = "file.list" // FileRef → FileListing
	MethodFileRead = "file.read" // FileRef → FileContent
	// MethodFileUpload puts a file of the app's machine on the session's
	// host, in parts: FileUpload → FileUploaded. Files dropped on a
	// server's session go there, so its agent can read them.
	MethodFileUpload = "file.upload"
	// MethodFileBytes reads a part of a file as it is, whatever it holds,
	// for the pages the app shows from a session's folder (an HTML file,
	// its styles, scripts, images and fonts): FileBytesParams → FileBytes.
	MethodFileBytes = "file.bytes"
	// MethodFileFind finds files of a session's folder by a few letters of
	// their path, for quick open: FileFindParams → FileFound.
	MethodFileFind = "file.find"
)

// FileFindParams are the letters to find files by, and how many at most.
type FileFindParams struct {
	Session string `json:"session"`
	Query   string `json:"query"`
	Limit   int    `json:"limit,omitempty"`
}

// FileFound are the files found, best first, relative to the folder.
type FileFound struct {
	Paths []string `json:"paths"`
}

// BytesPart is the most a FileBytes carries at once.
const BytesPart = 4 << 20

// FileBytesParams name the file as a FileRef does (Abs included), and
// where in it to read from.
type FileBytesParams struct {
	Session string `json:"session"`
	Path    string `json:"path"`
	Abs     bool   `json:"abs,omitempty"`
	Offset  int64  `json:"offset,omitempty"`
}

// FileBytes is up to BytesPart of a file from the offset asked, and how
// large the file is.
type FileBytes struct {
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified,omitzero"`
	Data     []byte    `json:"data,omitempty"`
}

// UploadPart is the most a FileUpload carries at once.
const UploadPart = 4 << 20

// FileUpload is a part of a file sent to the session's host. The first
// part has no Path; the daemon answers with where the file goes, which
// the next parts give.
type FileUpload struct {
	Session string `json:"session"`
	Name    string `json:"name"` // the file's name, without folders
	Path    string `json:"path,omitempty"`
	Data    []byte `json:"data"`
	// Into puts the file in this folder of the session's folder (made if
	// missing; "." is the folder itself) rather than with the session's
	// uploads, under a free name: added to the project from the Files
	// tool. Older daemons put it with the uploads.
	Into string `json:"into,omitempty"`
}

// FileUploaded is where an uploaded file is, absolute on the host.
type FileUploaded struct {
	Path string `json:"path"`
	// Placed says the file went into FileUpload.Into; older daemons, which
	// put it with the uploads, don't say so.
	Placed bool `json:"placed,omitempty"`
}

// FileRef names a file or folder in a session's folder. Unless, when set,
// asks file.read for nothing when the file last changed then: Unchanged.
type FileRef struct {
	Session string    `json:"session"`
	Path    string    `json:"path"`
	Unless  time.Time `json:"unless,omitzero"`
	// Abs reads Path as an absolute path on the host, wherever it is: a
	// file a program printed (a screenshot in /tmp, say). file.read only.
	// Older daemons read it in the session's folder.
	Abs bool `json:"abs,omitempty"`
	// Whole reads a text file up to MaxEdit, to edit it, rather than its first part. Older
	// daemons read that: Truncated says so, and it can't be edited.
	Whole bool `json:"whole,omitempty"`
}

// MaxEdit is the longest file that can be read whole and edited.
const MaxEdit = 16 << 20

// FileWrite saves a file edited: atomically, keeping its mode, and only if it is as it was
// at Unless (when set) unless Force says to write over what's there.
type FileWrite struct {
	Session string    `json:"session"`
	Path    string    `json:"path"`
	Abs     bool      `json:"abs,omitempty"`
	Text    string    `json:"text"`
	Unless  time.Time `json:"unless,omitzero"`
	Force   bool      `json:"force,omitempty"`
}

// FileWritten is how a save went. Changed: the file changed since Unless, and nothing was
// written; Modified is when it last changed either way.
type FileWritten struct {
	Modified time.Time `json:"modified,omitzero"`
	Changed  bool      `json:"changed,omitempty"`
}

// FileListing is a folder's entries: folders first, then files, by name.
type FileListing struct {
	Path      string      `json:"path"`
	Root      string      `json:"root"` // the session's folder, absolute
	Entries   []FileEntry `json:"entries"`
	Truncated bool        `json:"truncated,omitempty"` // more entries than shown
}

// FileEntry is a file or folder. Status is git's short status of a file
// ("M", "A", "D", "R", "?" for untracked), or "M" for a folder holding
// changes; Ignored marks what git ignores.
type FileEntry struct {
	Name    string `json:"name"`
	Dir     bool   `json:"dir,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Status  string `json:"status,omitempty"`
	Ignored bool   `json:"ignored,omitempty"`
}

// FileContent is a file's text, an image's bytes, or why it shows none.
type FileContent struct {
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	Modified  time.Time `json:"modified,omitzero"`
	Unchanged bool      `json:"unchanged,omitempty"` // since FileRef.Unless: nothing else is sent
	Text      string    `json:"text,omitempty"`
	Binary    bool      `json:"binary,omitempty"`
	Truncated bool      `json:"truncated,omitempty"` // longer than shown
	ReadOnly  bool      `json:"readOnly,omitempty"`  // it can't be written
	// Image is an image's format ("png", "svg", …) and Data its bytes,
	// for images up to MaxImage.
	Image string `json:"image,omitempty"`
	Data  []byte `json:"data,omitempty"`
}

// MaxImage is the largest image file.read sends.
const MaxImage = 8 << 20

// Changing the files of a session's folder from the Files tool, and
// what a file is.
const (
	MethodFileChange = "file.change" // FileChange → FileChanged
	MethodFileWrite  = "file.write"  // FileWrite → FileWritten
	MethodFileStat   = "file.stat"   // FileRef → FileStat
)

// File changes.
const (
	FileNewFile   = "newFile"   // a new empty file Name in folder Path
	FileNewFolder = "newFolder" // a new folder Name in folder Path
	FileRename    = "rename"    // Path renamed Name, in its folder
	FileDuplicate = "duplicate" // a copy of Path beside it
	FileDelete    = "delete"    // Path, a folder with all it holds
)

// FileChange is a change to Path of the session's folder.
type FileChange struct {
	Session string `json:"session"`
	Change  string `json:"change"`
	Path    string `json:"path"`
	Name    string `json:"name,omitempty"`
}

// FileChanged is the path the change made or renamed to ("" for a
// delete), relative to the session's folder.
type FileChanged struct {
	Path string `json:"path"`
}

// FileStat is what a file or folder of the session's folder is.
type FileStat struct {
	Path     string    `json:"path"` // relative to the session's folder
	Abs      string    `json:"abs"`
	Dir      bool      `json:"dir,omitempty"`
	Link     string    `json:"link,omitempty"` // what a symbolic link points to
	Size     int64     `json:"size"`           // a folder's: what it holds, as far as counted
	Files    int       `json:"files,omitempty"`
	Folders  int       `json:"folders,omitempty"`
	Partial  bool      `json:"partial,omitempty"` // a folder too big to count whole
	Modified time.Time `json:"modified,omitzero"`
	Mode     string    `json:"mode"`            // as ls shows it: -rw-r--r--
	Owner    string    `json:"owner,omitempty"` // user:group, where the system has them
	Status   string    `json:"status,omitempty"`
	Ignored  bool      `json:"ignored,omitempty"`
}
