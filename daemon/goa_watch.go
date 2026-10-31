package daemon

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A GOA whose agent can't be held to enowX's tools is watched: the
// programs it runs itself (its processes, by the session tag in their
// environment) and the files that change in the host's git projects while
// no other session works there are flagged to the user. A file that was
// unchanged, or changed only in tracked files, when GOA started can be put
// back as it was (git stash create keeps that state without touching the
// folder). Best effort: a change GOA makes while a worker works in the
// same folder passes as the worker's.

// goaWatchEvery is how often a watched GOA is looked at.
const goaWatchEvery = 5 * time.Second

// goaWatch is a watched GOA session's: the projects as they were when it
// started, what was flagged. Its goroutine owns it.
type goaWatch struct {
	stop    chan struct{}
	base    map[string]projectBase // by project folder
	flagged map[string]bool        // project + file, or a process's ID
}

// projectBase is a project as GOA found it: the files changed then, their
// content's hash, and a commit keeping their tracked changes.
type projectBase struct {
	stash string
	dirty map[string]string // file → status ("??" untracked) + hash
}

// goaWatchSeen starts watching GOA session info when its agent is only
// watched, and stops once it ends.
func (s *Server) goaWatchSeen(info proto.SessionInfo) {
	if level, _ := goa.LevelOf(agent.Base(info.Agent)); level != proto.GOAWatched {
		return
	}
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	w := s.goa.watch[info.ID]
	switch {
	case info.Exited && w != nil:
		close(w.stop)
		delete(s.goa.watch, info.ID)
	case !info.Exited && w == nil:
		w = &goaWatch{stop: make(chan struct{}), flagged: map[string]bool{}}
		s.goa.watch[info.ID] = w
		go s.runGOAWatch(info, w)
	}
}

func (s *Server) runGOAWatch(info proto.SessionInfo, w *goaWatch) {
	bases := map[string]projectBase{}
	for _, p := range s.projects.List() {
		if base, ok := baseOf(p.Path); ok {
			bases[p.Path] = base
		}
	}
	s.goa.mu.Lock()
	w.base = bases // read-only from now on; goaRevert reads it under mu
	s.goa.mu.Unlock()
	tick := time.NewTicker(goaWatchEvery)
	defer tick.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-s.done:
			return
		case <-tick.C:
			s.goaLook(info, w)
		}
	}
}

// baseOf is git project dir as it is now.
func baseOf(dir string) (projectBase, bool) {
	files, ok := gitChanged(dir)
	if !ok {
		return projectBase{}, false
	}
	b := projectBase{dirty: map[string]string{}}
	for f, st := range files {
		b.dirty[f] = st + " " + fileHash(filepath.Join(dir, f))
	}
	b.stash, _ = git.Run(dir, "stash", "create")
	return b, true
}

// goaLook flags what GOA session info did since the last look.
func (s *Server) goaLook(info proto.SessionInfo, w *goaWatch) {
	preset, _ := agent.Find(info.Agent)
	for _, pid := range procstat.Tagged(session.EnvSession, info.ID) {
		key := fmt.Sprint("pid:", pid)
		args := procstat.Args(pid)
		if w.flagged[key] || len(args) == 0 || s.goaOwnProcess(args, preset.Command) {
			continue
		}
		w.flagged[key] = true
		s.goaFlag(proto.GOAFlag{Session: info.ID, Command: clip(strings.Join(args, " "), 300)})
	}
	for dir, base := range w.base {
		if s.othersWorkIn(dir) {
			continue
		}
		now, ok := gitChanged(dir)
		if !ok {
			continue
		}
		for f, st := range now {
			key := dir + "\x00" + f
			was, dirty := base.dirty[f]
			if w.flagged[key] || dirty && was == st+" "+fileHash(filepath.Join(dir, f)) {
				continue
			}
			w.flagged[key] = true
			change := "modified"
			switch {
			case strings.Contains(st, "?") || strings.Contains(st, "A"):
				change = "added"
			case strings.Contains(st, "D"):
				change = "deleted"
			}
			s.goaFlag(proto.GOAFlag{Session: info.ID, Project: dir, File: f, Change: change,
				Revertable: !dirty || !strings.HasPrefix(was, "??") && base.stash != ""})
		}
	}
}

// goaOwnProcess says whether a process (args) is GOA's agent itself (cmd,
// and the shell wrapping it), enowX's program, or the shell a session
// keeps once its agent ends.
func (s *Server) goaOwnProcess(args []string, cmd string) bool {
	line := strings.Join(args, " ")
	if cmd != "" && strings.Contains(line, cmd) || s.HookBin != "" && strings.Contains(line, s.HookBin) {
		return true
	}
	exe, _ := os.Executable()
	if exe != "" && strings.Contains(line, exe) {
		return true
	}
	name := strings.TrimPrefix(filepath.Base(args[0]), "-")
	return len(args) == 1 && slices.Contains([]string{"sh", "bash", "zsh", "fish", "dash"}, name)
}

// othersWorkIn says whether a session other than GOA works in folder dir.
func (s *Server) othersWorkIn(dir string) bool {
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		if !info.GOA && !info.Exited && (info.Project == dir || info.Dir == dir) {
			return true
		}
	}
	return false
}

// goaFlag keeps flag f and tells the apps and the user.
func (s *Server) goaFlag(f proto.GOAFlag) {
	id := make([]byte, 4)
	_, _ = rand.Read(id)
	f.ID, f.Time = "f"+hex.EncodeToString(id), time.Now()
	s.goa.mu.Lock()
	s.goa.flags = append(s.goa.flags, f)
	if n := len(s.goa.flags); n > 200 {
		s.goa.flags = slices.Clone(s.goa.flags[n-200:])
	}
	s.goa.mu.Unlock()
	what := "ran " + f.Command
	if f.File != "" {
		what = f.Change + " " + f.File + " in " + filepath.Base(f.Project)
	}
	s.broadcast(proto.EventGOAFlag, f)
	s.broadcast(proto.EventNotifySound, proto.NotifySoundParams{Kind: proto.NotifyInput, Session: f.Session,
		Text: "GOD of Agents " + what + " itself, not through a session. See its Sessions panel."})
}

// goaFlags are the flags kept, the newest last.
func (s *Server) goaFlags() []proto.GOAFlag {
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	return append([]proto.GOAFlag{}, s.goa.flags...)
}

// goaRevert puts flag id's file back as it was when GOA started.
func (s *Server) goaRevert(id string) (proto.GOAFlag, error) {
	s.goa.mu.Lock()
	i := slices.IndexFunc(s.goa.flags, func(f proto.GOAFlag) bool { return f.ID == id })
	var f proto.GOAFlag
	var base projectBase
	if i >= 0 {
		f = s.goa.flags[i]
		if w := s.goa.watch[f.Session]; w != nil {
			base = w.base[f.Project]
		}
	}
	s.goa.mu.Unlock()
	switch {
	case i < 0:
		return f, fmt.Errorf("goa: no flag %q", id)
	case f.File == "" || !f.Revertable || f.Reverted:
		return f, fmt.Errorf("goa: %s can't be put back", nonEmpty(f.File, "a program run"))
	case base.dirty == nil:
		return f, fmt.Errorf("goa: GOA's watch of %s ended: its state from before is gone", f.Project)
	}
	var err error
	_, wasDirty := base.dirty[f.File]
	switch {
	case wasDirty:
		_, err = git.Run(f.Project, "checkout", base.stash, "--", f.File)
	case fileInHead(f.Project, f.File):
		_, err = git.Run(f.Project, "checkout", "HEAD", "--", f.File)
	default:
		err = os.Remove(filepath.Join(f.Project, f.File)) // it added it
	}
	if err != nil {
		return f, fmt.Errorf("goa: revert %s: %w", f.File, err)
	}
	s.goa.mu.Lock()
	if i < len(s.goa.flags) && s.goa.flags[i].ID == id {
		s.goa.flags[i].Reverted = true
		f = s.goa.flags[i]
	}
	s.goa.mu.Unlock()
	s.broadcast(proto.EventGOAFlag, f)
	return f, nil
}

// gitChanged are the files changed in git project dir, with their status
// (git status --porcelain's XY); false when dir isn't one.
func gitChanged(dir string) (map[string]string, bool) {
	cmd := git.Command(dir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	var out bytes.Buffer
	cmd.Stdout = &out
	if cmd.Run() != nil {
		return nil, false
	}
	files := map[string]string{}
	entries := strings.Split(out.String(), "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		st, path := e[:2], e[3:]
		if st[0] == 'R' || st[0] == 'C' {
			i++ // the name it had follows
		}
		files[path] = st
	}
	return files, true
}

func fileInHead(dir, file string) bool {
	_, err := git.Run(dir, "cat-file", "-e", "HEAD:"+filepath.ToSlash(file))
	return err == nil
}
