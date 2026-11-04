package permission

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/platformagent/tools"
)

// programFolders are what, once written, runs as a program, changes what the agent may do
// or holds what lets others in: an edit there is more than an edit, so acceptEdits and a
// kept rule still ask, inside the working folder or not.
var programFolders = map[string]bool{
	".git": true, ".claude": true, ".codex": true, ".opencode": true, ".mcp.json": true,
	".ssh": true, ".config": true, ".husky": true, ".github": true, ".gnupg": true, ".aws": true,
}

// guardedName says a path's part is one of programFolders. Lower case, as macOS's and
// Windows's disks take .GIT for .git. Every .enowx*: .enowx, .enowx-app, .enowx-dev.
func guardedName(part string) bool {
	part = strings.ToLower(part)
	return programFolders[part] || strings.HasPrefix(part, ".enowx")
}

// editInside says the edit in input lands in dir, whatever links and .. lead through, and
// outside what programFolders guard.
func editInside(dir string, input json.RawMessage) bool {
	var in struct {
		FilePath string `json:"file_path"`
	}
	if dir == "" || json.Unmarshal(input, &in) != nil || in.FilePath == "" {
		return false
	}
	// The path the tool will use, so the two can't disagree over a "link/..".
	rel, inside := relInside(dir, tools.ResolvePath(dir, in.FilePath))
	if !inside || rel == "." {
		return false
	}
	return !slices.ContainsFunc(strings.Split(rel, string(filepath.Separator)), guardedName)
}

// relInside is absolute path p from folder dir, its links followed, and whether that is
// in dir: the folder itself is.
func relInside(dir, p string) (string, bool) {
	root, err := filepath.EvalSymlinks(tools.ResolvePath(dir, "."))
	if err != nil {
		return "", false
	}
	real, ok := realPath(p)
	if !ok {
		return "", false
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// realPath is p with its links followed, what doesn't exist yet kept as named. A link
// that leads nowhere has no answer: writing through it could land anywhere.
func realPath(p string) (string, bool) {
	real, err := filepath.EvalSymlinks(p)
	if err == nil {
		return real, true
	}
	if _, lerr := os.Lstat(p); !errors.Is(lerr, os.ErrNotExist) {
		return "", false
	}
	parent := filepath.Dir(p)
	if parent == p {
		return "", false
	}
	real, ok := realPath(parent)
	if !ok {
		return "", false
	}
	return filepath.Join(real, filepath.Base(p)), true
}
