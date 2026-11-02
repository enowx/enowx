package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/core/cacheclean"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/proto"
)

// cacheEnv is what cacheclean needs of this host. It looks a little more each time:
// what's running and who uses what change between looking and cleaning.
func (s *Server) cacheEnv(olderDays int) cacheclean.Env {
	return cacheclean.Env{
		Dir: s.dataDir, Now: time.Now(), OlderDays: olderDays,
		InUse:          s.cacheInUse,
		Running:        cacheRunning(),
		Worktree:       worktreeState,
		RemoveWorktree: removeWorktree,
		Canvases:       s.cacheCanvases,
		ThinCanvas:     s.cacheThinCanvas,
	}
}

// cacheInUse names a session that isn't ended whose folder is at or under path.
func (s *Server) cacheInUse(path string) string {
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		if !info.Exited && (info.Dir == path || strings.HasPrefix(info.Dir, path+string(filepath.Separator))) {
			return info.ID
		}
	}
	return ""
}

// cacheRunning says a process runs from a path, or was given it. The whole list is
// read once, as arguments are read a process at a time.
func cacheRunning() func(path string) bool {
	var lines []string
	if exe, err := os.Executable(); err == nil {
		lines = append(lines, exe)
	}
	if list, err := procstat.Snapshot(); err == nil {
		for _, p := range list {
			if args := procstat.Args(p.PID); len(args) > 0 {
				lines = append(lines, strings.Join(args, " "))
			}
		}
	}
	return func(path string) bool {
		for _, line := range lines {
			if strings.Contains(line, path) {
				return true
			}
		}
		return false
	}
}

// worktreeState says what's in a worktree that isn't anywhere else.
func worktreeState(dir string) string {
	status, err := git.Run(dir, "status", "--porcelain")
	if err != nil {
		return "Not a working git folder"
	}
	var parts []string
	if n := len(strings.Fields(strings.ReplaceAll(status, "\n", " "))) / 2; strings.TrimSpace(status) != "" {
		parts = append(parts, fmt.Sprintf("about %d uncommitted", max(n, 1)))
	}
	if ahead, err := git.Run(dir, "rev-list", "--count", "HEAD", "--not", "--remotes"); err == nil {
		if n := strings.TrimSpace(ahead); n != "" && n != "0" {
			parts = append(parts, n+" not pushed")
		}
	}
	if len(parts) == 0 {
		return "Clean, all pushed"
	}
	return strings.Join(parts, ", ")
}

// removeWorktree removes a worktree as git keeps track of it; its branch stays.
func removeWorktree(dir string) error {
	_, err := git.Run(dir, "worktree", "remove", "--force", dir)
	return err
}

// cacheCanvases are the folders of canvases' versions here, under the data folder.
func (s *Server) cacheCanvases() []string {
	var out []string
	for _, pattern := range []string{filepath.Join(s.dataDir, "canvas", "*", "history"), filepath.Join(s.dataDir, "teams", "*", "canvas", "*", "history")} {
		found, _ := filepath.Glob(pattern)
		for _, dir := range found {
			if rel, err := filepath.Rel(s.dataDir, dir); err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
	}
	return out
}

// cacheThinCanvas thins the canvas whose versions are in folder (canvas/ID/history or
// teams/T/canvas/ID/history): its own store, for changes to wait their turn.
func (s *Server) cacheThinCanvas(folder string, keep int) (int64, error) {
	parts := strings.Split(folder, "/")
	var st *canvas.Store
	var id string
	switch {
	case len(parts) == 3 && parts[0] == "canvas" && parts[2] == "history":
		st, id = s.canvases, parts[1]
	case len(parts) == 5 && parts[0] == "teams" && parts[2] == "canvas" && parts[4] == "history":
		var err error
		if st, err = s.canvasesIn(parts[1]); err != nil {
			return 0, err
		}
		id = parts[3]
	default:
		return 0, fmt.Errorf("cache: %s isn't a canvas's versions", folder)
	}
	return st.ThinHistory(id, keep)
}

func (s *Server) cacheList(p proto.CacheListParams) proto.CacheReport {
	return cacheclean.List(s.cacheEnv(p.OlderDays))
}

func (s *Server) cacheClean(p proto.CacheCleanParams) proto.CacheCleanResult {
	res := cacheclean.Clean(s.cacheEnv(p.OlderDays), p)
	s.logger().Info("cache: cleaned", "items", res.Done, "freedMB", res.Freed>>20, "failed", len(res.Failed))
	return res
}
