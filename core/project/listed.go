package project

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/proto"
)

// The daemon lists its projects on several timers, and each listing asked git about
// every folder: enough processes to show in its CPU with a dozen projects. All List
// says of a folder from git is whether it is a repository and its checked out branch,
// and a stat tells when those can have changed: .git appearing, or HEAD rewritten by a
// checkout. A commit leaves HEAD as it is.

// listRefresh is how long List trusts what it knows of a folder whose .git looks the
// same, for what a stat can't see: git's own config, a reftable repository's HEAD.
// Tests shorten it.
var listRefresh = 5 * time.Minute

// listed is what List last learned of a folder, and what its .git looked like then.
type listed struct {
	info proto.ProjectInfo
	look gitLook
	at   time.Time
}

// gitLook is what can be seen of a folder's repository without running git. The
// .git folder's own time isn't part of it: every commit changes that.
type gitLook struct {
	file fileStamp // .git, when it is a file: a worktree's, naming its own folder
	head fileStamp
}

// fileStamp tells a file from what it was; zero when there is no such file.
type fileStamp struct {
	size, modTime int64
}

func stampOf(path string) fileStamp {
	st, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{size: st.Size(), modTime: st.ModTime().UnixNano()}
}

// gitLookOf looks at dir's repository. Only dir's own .git decides: a folder inside
// another's repository isn't a repository to Inspect, whatever happens to that one.
func gitLookOf(dir string) gitLook {
	dotGit := filepath.Join(dir, ".git")
	st, err := os.Stat(dotGit)
	if err != nil {
		return gitLook{}
	}
	if st.IsDir() {
		return gitLook{head: stampOf(filepath.Join(dotGit, "HEAD"))}
	}
	look := gitLook{file: stampOf(dotGit)}
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return look
	}
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
	if !ok {
		return look
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	look.head = stampOf(filepath.Join(gitDir, "HEAD"))
	return look
}

// describe is what List says of folder path: what it knows, while that can't have
// changed. It looks before asking git, so a checkout meanwhile shows at the next look.
// Two callers at once may both ask git: s.mu is never held over it.
func (s *Store) describe(path string) proto.ProjectInfo {
	look := gitLookOf(path)
	now := time.Now()
	s.mu.Lock()
	known, ok := s.listed[path]
	s.mu.Unlock()
	if ok && known.look == look && now.Sub(known.at) < listRefresh {
		return known.info
	}
	info := inspectInfo(path)
	s.mu.Lock()
	if s.listed == nil {
		s.listed = map[string]listed{}
	}
	s.listed[path] = listed{info: info, look: look, at: now}
	s.mu.Unlock()
	return info
}

// inspectInfo is the part of Inspect that lists show.
func inspectInfo(path string) proto.ProjectInfo {
	info := proto.ProjectInfo{Path: path, Name: filepath.Base(path)}
	top, err := git.TopLevel(path)
	if err != nil || top != filepath.Clean(path) {
		return info
	}
	info.IsGit = true
	info.Branch = git.CurrentBranch(path)
	return info
}
