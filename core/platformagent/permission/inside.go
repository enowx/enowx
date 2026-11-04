package permission

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/core/platformagent/tools"
)

// programFolders are what, once written, runs as a program or changes what the agent
// may do: an edit there is more than an edit, so acceptEdits still asks, inside the
// working folder or not.
var programFolders = map[string]bool{
	".git": true, ".claude": true, ".codex": true, ".opencode": true, ".enowx": true, ".mcp.json": true,
}

// editInside says the edit in input lands in dir, whatever links and .. lead through.
func editInside(dir string, input json.RawMessage) bool {
	var in struct {
		FilePath string `json:"file_path"`
	}
	if dir == "" || json.Unmarshal(input, &in) != nil || in.FilePath == "" {
		return false
	}
	root, err := filepath.EvalSymlinks(tools.ResolvePath(dir, "."))
	if err != nil {
		return false
	}
	// The path the tool will use, so the two can't disagree over a "link/..".
	real, ok := realPath(tools.ResolvePath(dir, in.FilePath))
	if !ok {
		return false
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		// Lower case, as macOS's and Windows's disks take .GIT for .git.
		if programFolders[strings.ToLower(part)] {
			return false
		}
	}
	return true
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
